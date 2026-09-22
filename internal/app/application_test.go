package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/web"
)

func externalTestConfig(t *testing.T, dbPath string) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = dbPath
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends[config.DefaultFilesActiveBackend] = config.FileStorageBackend{
		Type:    config.FilesStorageBackendFilesystem,
		RootDir: t.TempDir(),
	}
	return cfg
}

func externalTestOptions(cfg config.Config) app.Options {
	return app.Options{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Prepare: []func(context.Context, *database.DB) error{
			web.PrepareInstanceSettings(cfg),
		},
	}
}

func TestApplicationCanBeBuiltFromExternalPackage(t *testing.T) {
	ctx := context.Background()
	built, err := app.Build(ctx, externalTestOptions(externalTestConfig(t, t.TempDir()+"/sqlwarden.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer built.Close(ctx)

	var _ http.Handler = web.NewApplication(built.Services).Handler()
}

func TestBaseURLIsBootstrappedOnceAndThenDatabaseOwned(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/sqlwarden.db"

	cfg := externalTestConfig(t, dbPath)
	cfg.BootstrapBaseURL = "https://first.example.com"
	built, err := app.Build(ctx, externalTestOptions(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := built.Close(ctx); err != nil {
		t.Fatal(err)
	}

	cfg.BootstrapBaseURL = "https://second.example.com"
	built, err = app.Build(ctx, externalTestOptions(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := built.Close(ctx); err != nil {
		t.Fatal(err)
	}

	db, err := database.New("sqlite", dbPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	settings, found, err := db.GetInstanceSettings(ctx)
	if err != nil || !found {
		t.Fatalf("get instance settings: found=%v err=%v", found, err)
	}
	if settings.BaseURL != "https://first.example.com" {
		t.Fatalf("base URL = %q, want first bootstrap value", settings.BaseURL)
	}
}

func TestBuildFailsWhenSavedFileStorageBackendIsNotConfigured(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/sqlwarden.db"
	cfg := externalTestConfig(t, dbPath)

	built, err := app.Build(ctx, externalTestOptions(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := built.Close(ctx); err != nil {
		t.Fatal(err)
	}

	seedFileOnRetiredBackend(t, dbPath)

	built, err = app.Build(ctx, externalTestOptions(cfg))
	if err == nil {
		built.Close(ctx)
		t.Fatal("expected missing storage backend to fail the build")
	}
	if !strings.Contains(err.Error(), `file storage backend "retired"`) {
		t.Fatalf("error = %v, want missing retired backend", err)
	}
}

// seedFileOnRetiredBackend saves workspace file content against a storage
// backend ID that no configuration declares, which is the state a deployment
// reaches by removing a backend that still owns saved content.
func seedFileOnRetiredBackend(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()

	db, err := database.New("sqlite", dbPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	account, err := db.InsertAccount(ctx, "backend-check@example.com", "Backend Check", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := db.InsertOrg(ctx, "backend-check", "Backend Check")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	file := database.WorkspaceFile{
		WorkspaceID:    ws.ID,
		Visibility:     database.FileVisibilityPrivate,
		OwnerAccountID: &account.ID,
		ObjectType:     database.FileObjectTypeFile,
		Name:           "orphan.sql",
		CreatedBy:      account.ID,
		UpdatedBy:      account.ID,
	}
	if err := db.InsertWorkspaceFile(ctx, &file); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveWorkspaceFileContent(ctx, file.ID, account.ID, database.WorkspaceFileContent{
		StorageBackendID: "retired",
		StorageKey:       "objects/orphan",
		ContentHash:      "hash",
		SizeBytes:        4,
	}, false); err != nil {
		t.Fatal(err)
	}
}
