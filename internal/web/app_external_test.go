package web_test

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/web"
)

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

	db, err := database.New("sqlite", dbPath, slog.Default(), false)
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
