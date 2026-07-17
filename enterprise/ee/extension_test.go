// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise License. See enterprise/LICENSE.

package ee

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/license"
)

func TestExtensionName(t *testing.T) {
	if got := NewModule().Name; got != "ee" {
		t.Fatalf("module name = %q, want ee", got)
	}
}

func TestMigrationsExistPerDriver(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		src, ok := NewModule().Migrations(driver)
		if !ok {
			t.Fatalf("expected migrations for %s", driver)
		}
		entries, err := fs.ReadDir(src, ".")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Fatalf("no migration files for %s", driver)
		}
	}
	if _, ok := NewModule().Migrations("unknown"); ok {
		t.Fatal("unknown database drivers must not receive PostgreSQL migrations")
	}
}

func TestMigrationsApplyToSQLite(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "ee.db")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.New("sqlite", dsn, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}

	src, _ := NewModule().Migrations("sqlite")
	if err := db.MigrateExtensionUp(src, "schema_migrations_ee"); err != nil {
		t.Fatal(err)
	}

	var count int
	err = db.NewSelect().
		Table("sqlite_master").
		ColumnExpr("count(*)").
		Where("type = 'table' AND name = 'ee_stub'").
		Scan(context.Background(), &count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("expected ee_stub table")
	}
}

func TestPlaceholderLicenseService(t *testing.T) {
	svc, err := newLicenseService(context.Background(), extension.BootstrapDeps{})
	if err != nil {
		t.Fatal(err)
	}
	if svc.Edition() != "enterprise" {
		t.Fatalf("Edition() = %q, want enterprise", svc.Edition())
	}
	if svc.IsLicensed("stub") {
		t.Fatal("placeholder must not license features")
	}
	if !errors.Is(svc.Require("stub"), license.ErrNotLicensed) {
		t.Fatal("Require must wrap ErrNotLicensed")
	}
}
