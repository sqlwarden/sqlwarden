package web

import (
	"context"
	"fmt"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/profile/server"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/filestore"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/services"
	"github.com/sqlwarden/internal/settings"
)

func newTestDependencies(t *testing.T) Dependencies {
	t.Helper()
	deps, _ := newTestDependenciesWithDB(t)
	return deps
}

func newTestDependenciesWithDB(t *testing.T) (Dependencies, *database.DB) {
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
	if err := settings.InitializeInstanceBaseURL(context.Background(), db, cfg.BootstrapBaseURL); err != nil {
		t.Fatal(err)
	}
	stores, err := newTestFileStores(cfg)
	if err != nil {
		t.Fatal(err)
	}
	policy := settings.NewTargetPolicy(db)
	core := credentials.NewEncryptedColumnProvider(db, credentials.NewKeyringSealer(keyring), engine.ConnectionSpecFor)
	rt := execution.NewLocal(execution.LocalConfig{
		Credentials: core,
		Policy:      policy,
		IdleTimeout: 30 * time.Minute,
	})
	t.Cleanup(rt.Shutdown)
	svc, infra, err := services.Build(services.Deps{
		Config:           cfg,
		DB:               db,
		Enforcer:         enforcer,
		Runtime:          rt,
		FileStores:       stores,
		Credentials:      core,
		CredentialWriter: core,
		RevealPolicy:     server.New(db).RevealPolicy(),
		TargetPolicy:     policy,
		SecretSealer:     keyring,
		Keyring:          keyring,
		SignIn:           identity.PasswordSignIn(db),
		Setup:            identity.FormSetup,
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dependencies{
		Config:           cfg,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Services:         svc,
		SettingsAppliers: infra.SettingsAppliers(),

		Invitations: orgs.InvitationsEnabled,
	}, db
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

func TestCloseLeavesDatabaseOpen(t *testing.T) {
	deps, db := newTestDependenciesWithDB(t)
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	app.Close()
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("database closed by web: %v", err)
	}
}

func TestSettingsAppliersApplyChangedSettings(t *testing.T) {
	deps := newTestDependencies(t)
	app, err := NewApplication(deps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	initial := app.InitialSettings()
	if app.accessLogsEnabled.Load() != initial.AccessLogsEnabled {
		t.Fatal("NewApplication did not apply the initial access log setting")
	}
	changed := initial
	changed.AccessLogsEnabled = !initial.AccessLogsEnabled
	for _, apply := range app.SettingsAppliers() {
		if err := apply(context.Background(), changed); err != nil {
			t.Fatal(err)
		}
	}
	if app.accessLogsEnabled.Load() != changed.AccessLogsEnabled {
		t.Fatal("settings appliers did not apply the access log change")
	}
	if app.InitialSettings() != initial {
		t.Fatal("InitialSettings changed after applying later settings")
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
