package web

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/filestore"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/orgs"
)

func newTestDependencies(t *testing.T) Dependencies {
	t.Helper()
	db := newTestDB(t)
	enforcer, err := access.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Files.StorageBackends = map[string]config.FileStorageBackend{
		config.DefaultFilesActiveBackend: {Type: config.FilesStorageBackendFilesystem, RootDir: t.TempDir()},
	}
	if err := InitializeInstanceBaseURL(context.Background(), db, cfg.BootstrapBaseURL); err != nil {
		t.Fatal(err)
	}
	stores, err := newTestFileStores(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sessions := connection.New(30 * time.Minute)
	cursors := connection.NewQueryCursorManager(30 * time.Minute)
	t.Cleanup(func() {
		cursors.Close()
		sessions.Close()
	})
	return Dependencies{
		Config:     cfg,
		DB:         db,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Keyring:    keyring,
		Enforcer:   enforcer,
		FileStores: stores,
		Sessions:   sessions,
		Cursors:    cursors,

		Setup:       identity.FormSetup,
		Invitations: orgs.InvitationsEnabled,
	}
}

func TestNewApplicationStartsNoGoroutines(t *testing.T) {
	deps := newTestDependencies(t)
	before := runtime.NumGoroutine()
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("NewApplication started %d goroutines", after-before)
	}
}

func TestStartRuntimeThenCloseStopsIt(t *testing.T) {
	deps := newTestDependencies(t)
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	before := runtime.NumGoroutine()
	app.StartRuntime(true)
	app.Close()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("%d goroutines still running after Close", after-before)
	}
}

func TestStartRuntimeWithoutJobsAppliesSettings(t *testing.T) {
	deps := newTestDependencies(t)
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.StartRuntime(false)
	settings := app.initialSettings
	settings.AccessLogsEnabled = !settings.AccessLogsEnabled
	app.queueRuntimeOperations(settings)
	deadline := time.Now().Add(2 * time.Second)
	for app.accessLogsEnabled.Load() != settings.AccessLogsEnabled && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if app.accessLogsEnabled.Load() != settings.AccessLogsEnabled {
		t.Fatal("runtime settings change was not applied without jobs")
	}
	if app.fileReaperCancel != nil {
		t.Fatal("file reaper started without jobs")
	}
}

func TestStartRuntimeRunsJobsOnlyWhenSelected(t *testing.T) {
	for _, runJobs := range []bool{true, false} {
		deps := newTestDependencies(t)
		app, err := NewApplication(deps)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		const key = "runtime-jobs-selection"
		if _, _, err := app.jobStore.EnqueueSingleton(ctx, jobs.EnqueueInput{
			Type:         "noop",
			SingletonKey: key,
			Visibility:   jobs.VisibilityInternal,
			MaxAttempts:  1,
		}); err != nil {
			t.Fatal(err)
		}
		app.StartRuntime(runJobs)
		active := true
		deadline := time.Now().Add(500 * time.Millisecond)
		if runJobs {
			deadline = time.Now().Add(5 * time.Second)
		}
		for time.Now().Before(deadline) {
			_, active, err = app.jobStore.ActiveBySingletonKey(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if !active {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		app.Close()
		if active == runJobs {
			t.Fatalf("runJobs=%v: queued job still active = %v", runJobs, active)
		}
	}
}

func TestStartRuntimeTwiceStartsOnce(t *testing.T) {
	deps := newTestDependencies(t)
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.StartRuntime(true)
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()
	app.StartRuntime(true)
	time.Sleep(100 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("second StartRuntime started %d goroutines", after-before)
	}
}

func TestCloseLeavesDatabaseOpen(t *testing.T) {
	deps := newTestDependencies(t)
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	app.Close()
	if err := deps.DB.PingContext(context.Background()); err != nil {
		t.Fatalf("database closed by web: %v", err)
	}
}

type testFileStores struct {
	activeBackendID string
	stores          map[string]filestore.Store
}

func (r *testFileStores) ActiveBackendID() string {
	return r.activeBackendID
}

func (r *testFileStores) Store(_ context.Context, backendID string) (filestore.Store, error) {
	if backendID == "" {
		backendID = database.DefaultFileStorageBackendID
	}
	store, ok := r.stores[backendID]
	if !ok {
		return nil, files.ErrStorageBackendUnavailable
	}
	return store, nil
}

func newTestFileStores(cfg config.Config) (*testFileStores, error) {
	activeBackendID := cfg.Files.ActiveStorageBackend
	if cfg.Files.StorageMode == config.FilesStorageModeFile || strings.TrimSpace(activeBackendID) == "" {
		activeBackendID = database.DefaultFileStorageBackendID
	}
	registry := &testFileStores{
		activeBackendID: activeBackendID,
		stores:          make(map[string]filestore.Store, len(cfg.Files.StorageBackends)),
	}
	for id, backend := range cfg.Files.StorageBackends {
		store, err := filestore.NewFilesystem(backend.RootDir)
		if err != nil {
			return nil, fmt.Errorf("backend %q: %w", id, err)
		}
		registry.stores[id] = store
	}
	return registry, nil
}
