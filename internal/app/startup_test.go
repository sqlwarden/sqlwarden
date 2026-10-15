package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
)

func seedLegacyConnections(t *testing.T, cfg config.Config, count int) {
	t.Helper()
	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO accounts (id, email, name) VALUES (1, 'owner@example.com', 'Owner');
		INSERT INTO organizations (id, slug, name) VALUES (1, 'startup', 'Startup');
		INSERT INTO workspaces (id, org_id, owner_type, owner_id, name, description)
		VALUES (10, 1, 'org', 1, 'Main', '');
		INSERT INTO environments (id, workspace_id, name, description) VALUES (20, 10, 'Default', '');
	`); err != nil {
		t.Fatal(err)
	}
	for i := range count {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO connections (id, workspace_id, environment_id, name, driver, dsn_encrypted)
			VALUES (?, 10, 20, ?, 'postgres', 'legacy-dsn')`, 30+i, "legacy"+string(rune('a'+i)),
		); err != nil {
			t.Fatal(err)
		}
	}
}

func buildOnce(t *testing.T, cfg config.Config, cmd Command) (*Application, error) {
	t.Helper()
	return Build(context.Background(), Options{Config: cfg, Logger: discardLogger(), Command: cmd, Edition: testEdition()})
}

func TestStartupRefusesLegacyConnections(t *testing.T) {
	cfg := testConfig(t)
	first, err := buildOnce(t, cfg, CommandMigrate)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close(context.Background())
	seedLegacyConnections(t, cfg, 2)

	var logs bytes.Buffer
	_, err = Build(context.Background(), Options{
		Config: cfg, Logger: slog.New(slog.NewTextHandler(&logs, nil)), Command: CommandServe, Edition: testEdition(),
	})
	var legacy *credentials.LegacyConnectionsError
	if !errors.As(err, &legacy) || legacy.Count != 2 {
		t.Fatalf("Build error = %v, want 2 legacy connections", err)
	}
	if want := "2 connections use the legacy format. Run sqlwarden rotate-keys."; err.Error() != want {
		t.Fatalf("message = %q, want %q", err.Error(), want)
	}
	if !strings.Contains(logs.String(), err.Error()) || !strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("startup error was not logged at error level: %s", logs.String())
	}
	if openDatabases.Load() != 0 {
		t.Fatalf("%d databases left open after startup check failed", openDatabases.Load())
	}
}

func TestStartupCheckSkippedByMigrateAndRotateKeys(t *testing.T) {
	cfg := testConfig(t)
	first, err := buildOnce(t, cfg, CommandMigrate)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close(context.Background())
	seedLegacyConnections(t, cfg, 1)

	for _, cmd := range []Command{CommandMigrate, CommandRotateKeys} {
		built, err := buildOnce(t, cfg, cmd)
		if err != nil {
			t.Fatalf("command %d: %v", cmd, err)
		}
		_ = built.Close(context.Background())
	}
}

func TestStartupAcceptsCleanDatabase(t *testing.T) {
	built, err := buildOnce(t, testConfig(t), CommandServe)
	if err != nil {
		t.Fatal(err)
	}
	_ = built.Close(context.Background())
}

func TestStartupRequiresMigrationWhenSchemaIsOutdated(t *testing.T) {
	cfg := testConfig(t)
	cfg.DB.Automigrate = false
	_, err := buildOnce(t, cfg, CommandServe)
	if err == nil || !strings.Contains(err.Error(), "run migrate first") {
		t.Fatalf("Build error = %v, want run migrate first", err)
	}
}
