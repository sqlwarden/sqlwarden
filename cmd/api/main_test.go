package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/database"
)

func TestRunMigrateAppliesSchemaAndExits(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "migrate.db")
	if err := run([]string{
		"migrate",
		"--db-driver", "sqlite",
		"--db-dsn", dsn,
		"--db-migration-timeout", "30s",
	}); err != nil {
		t.Fatal(err)
	}

	db, err := database.New("sqlite", dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("migrate command left migration history empty")
	}
}
