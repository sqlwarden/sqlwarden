package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sqlwarden/app"
	"github.com/sqlwarden/buildinfo"
	"github.com/sqlwarden/distribution"
)

type lifecycle struct{ started, stopped chan struct{} }

func (l lifecycle) Start(context.Context) error    { close(l.started); return nil }
func (l lifecycle) Shutdown(context.Context) error { close(l.stopped); return nil }

type failingLifecycle struct{ started, stopped chan struct{} }

func (l failingLifecycle) Start(context.Context) error {
	close(l.started)
	return errors.New("start failed")
}
func (l failingLifecycle) Shutdown(context.Context) error { close(l.stopped); return nil }

func TestDistributionCompositionUsesPublicContracts(t *testing.T) {
	cfg := app.DefaultConfig()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = t.TempDir() + "/sqlwarden.db"
	cfg.DB.Automigrate = true
	cfg.Jobs.PollInterval = 10 * time.Millisecond
	cfg.Files.StorageBackends["local"] = app.FileStorageBackend{Type: app.FilesStorageBackendFilesystem, RootDir: t.TempDir() + "/files"}

	migrations := fstest.MapFS{
		"sqlite/000001_notes.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE paid_notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL);")},
		"sqlite/000001_notes.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE paid_notes;")},
	}
	frontend := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("enterprise frontend")}}
	jobRan := make(chan struct{})
	life := lifecycle{started: make(chan struct{}), stopped: make(chan struct{})}
	var services distribution.HostServices

	application, err := app.New(cfg, slog.Default(), app.WithDistribution(func(host distribution.HostServices) (distribution.Dependencies, error) {
		services = host
		return distribution.Dependencies{
			Migrations: []distribution.MigrationSet{{FS: migrations, SQLitePath: "sqlite", PostgresPath: "sqlite", MigrationsName: "paid_schema_migrations"}},
			Jobs: []distribution.Job{{Type: "paid_contract_job", MaxAttempts: 1, Handler: distribution.JobHandlerFunc(func(context.Context, distribution.JobRuntime) (any, error) {
				close(jobRan)
				return map[string]bool{"ok": true}, nil
			})}},
			InstallRoutes: func(routes distribution.RouteMounts) {
				routes.Public.Get("/paid/ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
				routes.Account.Get("/paid/whoami", func(w http.ResponseWriter, r *http.Request) {
					account, ok := host.Request.Account(r)
					if !ok {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					_, _ = fmt.Fprint(w, account.ID)
				})
			},
			Lifecycle: life, Frontend: frontend,
			Build: distributionBuildInfo(),
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-life.started:
	default:
		t.Fatal("lifecycle did not start")
	}
	var count int
	if err := services.DB.NewSelect().TableExpr("paid_notes").ColumnExpr("COUNT(*)").Scan(context.Background(), &count); err != nil {
		t.Fatal(err)
	}
	if err := services.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'paid_schema_migrations'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("distribution migration ledger was not created")
	}
	if application.BuildInfo().Distribution != "enterprise" {
		t.Fatalf("distribution = %q", application.BuildInfo().Distribution)
	}
	account, err := services.Accounts.Provision(context.Background(), "sso@example.com", "SSO User")
	if err != nil {
		t.Fatal(err)
	}
	sessionRequest := httptest.NewRequest(http.MethodPost, "/sso/callback", nil)
	sessionResponse := httptest.NewRecorder()
	if err = services.Sessions.Complete(sessionResponse, sessionRequest, account.ID); err != nil {
		t.Fatal(err)
	}
	if sessionResponse.Code != http.StatusOK || !strings.Contains(sessionResponse.Body.String(), "access_token") || len(sessionResponse.Result().Cookies()) == 0 {
		t.Fatalf("session response status=%d body=%q cookies=%d", sessionResponse.Code, sessionResponse.Body.String(), len(sessionResponse.Result().Cookies()))
	}
	var session struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.Unmarshal(sessionResponse.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(application.Handler())
	response, err := server.Client().Get(server.URL + "/api/v1/paid/ping")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("route status = %d", response.StatusCode)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/paid/whoami", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != fmt.Sprint(account.ID) {
		t.Fatalf("account route status=%d body=%q", response.StatusCode, body)
	}
	response, err = server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if string(body) != "enterprise frontend" || response.StatusCode != http.StatusOK {
		t.Fatalf("frontend status = %d body = %q", response.StatusCode, body)
	}

	if _, err := services.Jobs.Enqueue(context.Background(), distribution.EnqueueJob{Type: "paid_contract_job", Visibility: "internal"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-jobRan:
	case <-time.After(3 * time.Second):
		t.Fatal("distribution job did not run")
	}

	server.Close()
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-life.stopped:
	default:
		t.Fatal("lifecycle did not stop")
	}
}

func TestDistributionLifecycleStartupFailureShutsDownInitializedResources(t *testing.T) {
	cfg := app.DefaultConfig()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = t.TempDir() + "/sqlwarden.db"
	cfg.DB.Automigrate = true
	backend := cfg.Files.StorageBackends["local"]
	backend.RootDir = t.TempDir() + "/files"
	cfg.Files.StorageBackends["local"] = backend

	life := failingLifecycle{started: make(chan struct{}), stopped: make(chan struct{})}
	application, err := app.New(cfg, slog.Default(), app.WithDistribution(func(distribution.HostServices) (distribution.Dependencies, error) {
		return distribution.Dependencies{Lifecycle: life}, nil
	}))
	if err == nil || application != nil {
		t.Fatalf("application=%v error=%v, want startup failure", application, err)
	}
	select {
	case <-life.started:
	default:
		t.Fatal("lifecycle start was not called")
	}
	select {
	case <-life.stopped:
	default:
		t.Fatal("lifecycle shutdown was not called after start failure")
	}
}

func distributionBuildInfo() buildinfo.Info {
	return buildinfo.Info{Distribution: "enterprise", DistributionVersion: "test", DistributionCommit: "test"}
}
