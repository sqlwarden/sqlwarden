package web_test

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/community"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/web"
)

func buildApp(cfg config.Config) (*app.Application, error) {
	return app.Build(context.Background(), app.Options{Config: cfg, Logger: slog.Default(), Edition: community.New()})
}

func closeApp(t *testing.T, built *app.Application) {
	t.Helper()
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAppCanBeConstructedFromExternalPackage(t *testing.T) {
	cfg := config.Default()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = t.TempDir() + "/sqlwarden.db"
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = config.FileStorageBackend{
		Type:    config.FilesStorageBackendFilesystem,
		RootDir: t.TempDir() + "/files",
	}

	built, err := buildApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closeApp(t, built)

	if _, err := built.RotateEncryptionKeys(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNewServerAcceptsAnyHandler(t *testing.T) {
	srv := web.NewServer(0, http.NotFoundHandler(), slog.Default())
	if srv.Handler == nil {
		t.Fatal("NewServer dropped the handler")
	}
}

func TestBaseURLIsBootstrappedOnceAndThenDatabaseOwned(t *testing.T) {
	dbPath := t.TempDir() + "/sqlwarden.db"
	cfg := config.Default()
	cfg.BootstrapBaseURL = "https://first.example.com"
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = dbPath
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = config.FileStorageBackend{
		Type:    config.FilesStorageBackendFilesystem,
		RootDir: t.TempDir() + "/files",
	}

	built, err := buildApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	closeApp(t, built)

	cfg.BootstrapBaseURL = "https://second.example.com"
	built, err = buildApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	closeApp(t, built)

	db, err := database.New("sqlite", dbPath, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings, found, err := db.GetInstanceSettings(context.Background())
	if err != nil || !found {
		t.Fatalf("get instance settings: found=%v err=%v", found, err)
	}
	if settings.BaseURL != "https://first.example.com" {
		t.Fatalf("base URL = %q, want first bootstrap value", settings.BaseURL)
	}
}

func TestAppFailsWhenSavedFileStorageBackendIsNotConfigured(t *testing.T) {
	dbPath := t.TempDir() + "/sqlwarden.db"
	cfg := config.Default()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = dbPath
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = config.FileStorageBackend{
		Type:    config.FilesStorageBackendFilesystem,
		RootDir: t.TempDir() + "/files",
	}

	setup, err := buildApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	closeApp(t, setup)

	db, err := database.New("sqlite", dbPath, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
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
	db.Close()

	built, err := buildApp(cfg)
	if err == nil {
		closeApp(t, built)
		t.Fatal("expected missing storage backend to fail startup")
	}
	if !strings.Contains(err.Error(), `file storage backend "retired"`) {
		t.Fatalf("error = %v, want missing retired backend", err)
	}
}
