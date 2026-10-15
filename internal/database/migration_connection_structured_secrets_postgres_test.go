package database

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/sqlwarden/assets"
)

func TestMigration46PostgresPreservesDataAndSeedsRevealPermission(t *testing.T) {
	ctx := context.Background()
	dsn := newRawPostgresTestDSN(t)
	migratePostgresToVersion(t, dsn, 45)

	db, err := New("postgres", dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		INSERT INTO accounts (id, email, name) VALUES (1, 'owner@example.com', 'Owner');
		INSERT INTO organizations (id, slug, name, schema_snapshots_enabled, mask_connection_credentials_on_edit)
		VALUES (1, 'migration-46', 'Migration 46', TRUE, TRUE);
		INSERT INTO workspaces (id, org_id, owner_type, owner_id, name, description)
		VALUES (10, 1, 'org', 1, 'Main', '');
		INSERT INTO environments (id, workspace_id, name, description) VALUES (20, 10, 'Default', '');
		INSERT INTO connections (
			id, workspace_id, environment_id, name, driver, dsn_encrypted,
			tls_config_encrypted, ssh_config_encrypted, access_mode,
			schema_snapshot_policy, default_scope, show_system_schemas,
			show_all_databases
		) VALUES (30, 10, 20, 'Primary', 'postgres', 'legacy-dsn', NULL, NULL, 'open', 'inherit', '', FALSE, FALSE);
		INSERT INTO roles (id, org_id, workspace_id, name, description, scope_type, is_builtin) VALUES
			(40, 1, NULL, 'Owner', 'Owner', 'org', TRUE),
			(41, 1, NULL, 'Administrator', 'Administrator', 'org', TRUE),
			(42, 1, NULL, 'Baseline Access', 'Baseline', 'org', TRUE),
			(43, 1, 10, 'Workspace Admin', 'Workspace admin', 'workspace', TRUE),
			(44, 1, NULL, 'Custom', 'Custom', 'org', FALSE);
		INSERT INTO role_bindings (
			id, org_id, role_id, subject_type, subject_id, resource_type, resource_id, created_by
		) VALUES (50, 1, 40, 'account', 1, 'org', 1, 1);
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}

	conn, found, err := db.GetConnection(ctx, 30)
	if err != nil || !found {
		t.Fatalf("GetConnection: found=%v err=%v", found, err)
	}
	if string(conn.Params) != `{}` {
		t.Fatalf("legacy params = %s, want {}", conn.Params)
	}
	org, found, err := db.GetOrg(ctx, 1)
	if err != nil || !found {
		t.Fatalf("GetOrg: found=%v err=%v", found, err)
	}
	if org.AllowConnectionSecretReveal {
		t.Fatal("allow_connection_secret_reveal must default false")
	}

	wantPermission := map[int64]bool{40: true, 41: true, 42: false, 43: false, 44: false}
	for roleID, want := range wantPermission {
		count, err := db.NewSelect().TableExpr("role_permissions").
			Where("role_id = ?", roleID).
			Where("permission = ?", "conn:reveal_secret").
			Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got := count == 1; got != want {
			t.Fatalf("role %d reveal permission = %v, want %v", roleID, got, want)
		}
	}

	bindings, err := db.NewSelect().TableExpr("role_bindings").Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bindings != 1 {
		t.Fatalf("role bindings = %d, want 1", bindings)
	}
	if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
		ConnectionID:   30,
		Name:           "password",
		Source:         "stored",
		ValueEncrypted: "ciphertext",
		KeyID:          "key-1",
	}); err != nil {
		t.Fatal(err)
	}
}

func migratePostgresToVersion(t *testing.T, dsn string, version uint) {
	t.Helper()
	source, err := iofs.New(assets.EmbeddedFiles, "migrations_postgres")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, "postgres://"+dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(version); err != nil {
		closeTestMigrator(t, migrator)
		t.Fatal(err)
	}
	closeTestMigrator(t, migrator)
}
