package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/config"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.HTTPPort = 0
	cfg.DB.DSN = filepath.Join(t.TempDir(), "app.db")
	cfg.Files.StorageBackends = map[string]config.FileStorageBackend{
		config.DefaultFilesActiveBackend: {Type: config.FilesStorageBackendFilesystem, RootDir: t.TempDir()},
	}
	return cfg
}

func TestBuildAllBuildsRuntimeAndHTTP(t *testing.T) {
	built, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	if got := built.ProcessKindNames(); !slices.Equal(got, []string{"runtime", "http"}) {
		t.Fatalf("ProcessKindNames() = %v", got)
	}
}

func TestBuildCommandsBuildNoProcessKinds(t *testing.T) {
	for _, cmd := range []Command{CommandMigrate, CommandRotateKeys} {
		built, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger(), Command: cmd})
		if err != nil {
			t.Fatal(err)
		}
		if got := built.ProcessKindNames(); len(got) != 0 {
			t.Fatalf("command %d built process kinds %v", cmd, got)
		}
		_ = built.Close(context.Background())
	}
}

func TestBuildRejectsInvalidConfiguration(t *testing.T) {
	cfg := testConfig(t)
	cfg.ProcessKinds = []string{config.ProcessKindRealtime}
	if _, err := Build(context.Background(), Options{Config: cfg, Logger: discardLogger()}); err == nil {
		t.Fatal("Build accepted an unimplemented process kind")
	}
}

func TestBuildClosesDatabaseWhenWebConstructionFails(t *testing.T) {
	cfg := testConfig(t)
	failWebConstruction = errors.New("injected")
	t.Cleanup(func() { failWebConstruction = nil })
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	if _, err := Build(context.Background(), Options{Config: cfg, Logger: logger}); err == nil {
		t.Fatal("Build did not return the injected error")
	}
	if openDatabases.Load() != 0 {
		t.Fatalf("%d databases left open", openDatabases.Load())
	}
	var closed []string
	for line := range strings.Lines(logs.String()) {
		var entry struct {
			Msg      string `json:"msg"`
			Resource string `json:"resource"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Msg == "resource closed" {
			closed = append(closed, entry.Resource)
		}
	}
	if want := []string{"cursors", "sessions", "database"}; !slices.Equal(closed, want) {
		t.Fatalf("closed resources = %v, want %v", closed, want)
	}
}

func TestHealthRoutesOnAllProcess(t *testing.T) {
	built, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	handler := built.httpHandler()
	for _, path := range []string{"/livez", "/api/setup/status"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
	}
}

func TestBuildAcquiresResourcesInDependencyOrder(t *testing.T) {
	built, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	want := []string{"database", "sessions", "cursors", "web"}
	if got := built.ResourceOrder(); !slices.Equal(got, want) {
		t.Fatalf("ResourceOrder() = %v, want %v", got, want)
	}
}

func TestBuildMigrateCommandOpensOnlyTheDatabase(t *testing.T) {
	cfg := testConfig(t)
	cfg.DB.Automigrate = false
	built, err := Build(context.Background(), Options{Config: cfg, Logger: discardLogger(), Command: CommandMigrate})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	if got := built.ResourceOrder(); !slices.Equal(got, []string{"database"}) {
		t.Fatalf("ResourceOrder() = %v, want only the database", got)
	}
	if _, err := built.RotateEncryptionKeys(context.Background()); err == nil {
		t.Fatal("RotateEncryptionKeys succeeded on a migrate-only application")
	}
}

func TestBuildLeavesNoDatabaseOpenAfterClose(t *testing.T) {
	built, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if openDatabases.Load() != 0 {
		t.Fatalf("%d databases left open", openDatabases.Load())
	}
}

func TestReadyzReportsStartState(t *testing.T) {
	built, err := Build(context.Background(), Options{Config: testConfig(t), Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	probe := func() int {
		rec := httptest.NewRecorder()
		built.httpHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rec.Code
	}
	if code := probe(); code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz before Start = %d, want 503", code)
	}
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code := probe(); code != http.StatusOK {
		t.Fatalf("GET /readyz after Start = %d, want 200", code)
	}
}

func TestBuildStartCloseDoesNotLeakGoroutines(t *testing.T) {
	cfg := testConfig(t)
	cycle := func() {
		built, err := Build(context.Background(), Options{Config: cfg, Logger: discardLogger()})
		if err != nil {
			t.Fatal(err)
		}
		if err := built.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := built.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	cycle()
	// database.MigrateUp leaves its migrate instance open, so later cycles
	// reuse the migrated database to measure only what Build owns.
	cfg.DB.Automigrate = false
	baseline := settledGoroutines()
	for range 3 {
		cycle()
	}
	if got := settledGoroutines(); got > baseline {
		t.Fatalf("goroutines after repeated build/start/close = %d, want at most %d", got, baseline)
	}
}

func TestEnsureSQLiteParentDir(t *testing.T) {
	cfg := testConfig(t)
	cfg.DB.DSN = filepath.Join(t.TempDir(), "nested", "sqlwarden.db")
	built, err := Build(context.Background(), Options{Config: cfg, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("build against a nested sqlite path: %v", err)
	}
	_ = built.Close(context.Background())
}
