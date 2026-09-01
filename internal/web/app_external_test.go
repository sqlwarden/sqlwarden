package web_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/web"
)

func TestDesktopDatabaseBackupCreatesSQLiteSnapshot(t *testing.T) {
	directory := t.TempDir()
	cfg := web.DefaultConfig()
	cfg.Mode = web.ModeDesktop
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = filepath.Join(directory, "sqlwarden.db")
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = web.FileStorageBackend{
		Type:    web.FilesStorageBackendFilesystem,
		RootDir: filepath.Join(directory, "files"),
	}

	app, err := web.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	destination := filepath.Join(directory, "snapshot.db")
	if err := app.BackupDesktopDatabase(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("desktop database snapshot is empty")
	}
}

func TestAppCanBeConstructedFromExternalPackage(t *testing.T) {
	cfg := web.DefaultConfig()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = t.TempDir() + "/sqlwarden.db"
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = web.FileStorageBackend{
		Type:    web.FilesStorageBackendFilesystem,
		RootDir: t.TempDir() + "/files",
	}

	app, err := web.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	var _ http.Handler = app.Handler()
}

func TestBaseURLIsBootstrappedOnceAndThenDatabaseOwned(t *testing.T) {
	dbPath := t.TempDir() + "/sqlwarden.db"
	cfg := web.DefaultConfig()
	cfg.BootstrapBaseURL = "https://first.example.com"
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = dbPath
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = web.FileStorageBackend{
		Type:    web.FilesStorageBackendFilesystem,
		RootDir: t.TempDir() + "/files",
	}

	app, err := web.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	app.Close()

	cfg.BootstrapBaseURL = "https://second.example.com"
	app, err = web.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	app.Close()

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
	cfg := web.DefaultConfig()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = dbPath
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends["local"] = web.FileStorageBackend{
		Type:    web.FilesStorageBackendFilesystem,
		RootDir: t.TempDir() + "/files",
	}

	setup, err := web.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	setup.Close()

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

	app, err := web.New(cfg, slog.Default())
	if err == nil {
		app.Close()
		t.Fatal("expected missing storage backend to fail startup")
	}
	if !strings.Contains(err.Error(), `file storage backend "retired"`) {
		t.Fatalf("error = %v, want missing retired backend", err)
	}
}
