package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func extTestMigrations() fstest.MapFS {
	return fstest.MapFS{
		"000001_ee_stub.up.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE ee_stub (id INTEGER PRIMARY KEY, note TEXT NOT NULL);"),
		},
		"000001_ee_stub.down.sql": &fstest.MapFile{
			Data: []byte("DROP TABLE ee_stub;"),
		},
	}
}

func TestMigrateExtensionUpUsesSeparateVersionTable(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "ext.db")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := New("sqlite", dsn, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateExtensionUp(extTestMigrations(), "schema_migrations_ee"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, table := range []string{"ee_stub", "schema_migrations_ee", "schema_migrations"} {
		var count int
		err := db.NewSelect().
			Table("sqlite_master").
			ColumnExpr("count(*)").
			Where("type = 'table' AND name = ?", table).
			Scan(ctx, &count)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected table %q to exist", table)
		}
	}

	// The extension stream tracks its own versions: version 1 must be
	// recorded in schema_migrations_ee, independent of core's table.
	var extVersion int
	if err := db.NewSelect().Table("schema_migrations_ee").ColumnExpr("version").Scan(ctx, &extVersion); err != nil {
		t.Fatal(err)
	}
	if extVersion != 1 {
		t.Fatalf("extension version = %d, want 1", extVersion)
	}

	// Re-running must be a no-op, not an error.
	if err := db.MigrateExtensionUp(extTestMigrations(), "schema_migrations_ee"); err != nil {
		t.Fatalf("second MigrateExtensionUp: %v", err)
	}
}
