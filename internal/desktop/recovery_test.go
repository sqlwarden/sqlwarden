package desktop

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupValidationRejectsUnmanifestedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malicious.sqlwarden-backup")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	database := []byte("database")
	digest := sha256.Sum256(database)
	manifest := backupManifest{
		FormatVersion: backupFormatVersion,
		AppVersion:    "test",
		CreatedAt:     "2026-09-01T00:00:00Z",
		Files:         map[string]string{"sqlwarden.db": hex.EncodeToString(digest[:])},
	}
	manifestContents, _ := json.Marshal(manifest)
	for name, contents := range map[string][]byte{
		"sqlwarden.db":  database,
		"extra.txt":     []byte("not declared"),
		"manifest.json": manifestContents,
	} {
		writer, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := writer.Write(contents); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBackupArchive(path); err == nil {
		t.Fatal("backup with unmanifested entry was accepted")
	}
}

func TestBackupValidationAndPendingRestore(t *testing.T) {
	paths, err := ResolvePaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := createDesktopDirectories(paths); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(paths.Temp, "snapshot.db")
	if err := os.WriteFile(snapshot, []byte("backup database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Files, "query.sql"), []byte("select 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(paths.Backups, "test.sqlwarden-backup")
	if err := CreateBackupArchive(paths, snapshot, archive, "test"); err != nil {
		t.Fatal(err)
	}
	manifest, err := ValidateBackupArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.FormatVersion != backupFormatVersion || manifest.Files["sqlwarden.db"] == "" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}

	if err := os.WriteFile(paths.Database, []byte("current database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Files, "query.sql"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := QueueRestore(paths, archive); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(paths); err != nil {
		t.Fatal(err)
	}
	database, err := os.ReadFile(paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	if string(database) != "backup database" {
		t.Fatalf("restored database = %q", database)
	}
	file, err := os.ReadFile(filepath.Join(paths.Files, "query.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if string(file) != "select 1" {
		t.Fatalf("restored file = %q", file)
	}
	rollbacks, err := filepath.Glob(filepath.Join(paths.Backups, "pre-restore-*.sqlwarden-backup"))
	if err != nil || len(rollbacks) != 1 {
		t.Fatalf("rollback backups = %v, err=%v", rollbacks, err)
	}
}
