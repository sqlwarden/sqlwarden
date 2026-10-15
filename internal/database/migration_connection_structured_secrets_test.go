package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/sqlwarden/assets"
)

func TestMigration46SQLitePreservesDataAndSeedsRevealPermission(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migration-46.db")
	migrateSQLiteToVersion(t, path, 45)

	db, err := New("sqlite", path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO accounts (id, email, name) VALUES (1, 'owner@example.com', 'Owner');
		INSERT INTO organizations (id, slug, name, schema_snapshots_enabled, mask_connection_credentials_on_edit)
		VALUES (1, 'migration-46', 'Migration 46', 1, 1);
		INSERT INTO org_members (org_id, account_id) VALUES (1, 1);
		INSERT INTO workspaces (id, org_id, owner_type, owner_id, name, description)
		VALUES (10, 1, 'org', 1, 'Main', '');
		INSERT INTO environments (id, workspace_id, name, description) VALUES (20, 10, 'Default', '');
		INSERT INTO connections (
			id, workspace_id, environment_id, name, driver, dsn_encrypted,
			tls_config_encrypted, ssh_config_encrypted, access_mode,
			schema_snapshot_policy, default_scope, show_system_schemas,
			show_all_databases
		) VALUES (30, 10, 20, 'Primary', 'postgres', 'legacy-dsn', NULL, NULL, 'open', 'inherit', '', 0, 0);

		INSERT INTO roles (id, org_id, workspace_id, name, description, scope_type, is_builtin) VALUES
			(40, 1, NULL, 'Owner', 'Owner', 'org', 1),
			(41, 1, NULL, 'Administrator', 'Administrator', 'org', 1),
			(42, 1, NULL, 'Baseline Access', 'Baseline', 'org', 1),
			(43, 1, 10, 'Workspace Admin', 'Workspace admin', 'workspace', 1),
			(44, 1, NULL, 'Custom', 'Custom', 'org', 0);
		INSERT INTO role_permissions (role_id, permission) VALUES
			(40, 'conn:update'), (41, 'conn:update'), (42, 'org:read'),
			(43, 'conn:update'), (44, 'conn:update');
		INSERT INTO role_bindings (
			id, org_id, role_id, subject_type, subject_id, resource_type, resource_id, created_by
		) VALUES
			(50, 1, 40, 'account', 1, 'org', 1, 1),
			(51, 1, 44, 'account', 1, 'org', 1, 1);

		INSERT INTO resource_hierarchy (child_type, child_id, parent_type, parent_id, owner_type, owner_id) VALUES
			('workspace', 10, 'org', 1, 'org', 1),
			('environment', 20, 'workspace', 10, 'org', 1),
			('connection', 30, 'environment', 20, 'org', 1);
		INSERT INTO schema_snapshots (
			id, connection_id, org_id, dialect, database_name, status, is_active, directory_data, generated_at
		) VALUES ('snapshot-1', 30, 1, 'postgres', 'app', 'ready', 1, X'7B7D', CURRENT_TIMESTAMP);
		INSERT INTO schema_nodes (connection_id, parent_path, folder, children_data, fetched_at)
		VALUES (30, '', 'databases', X'5B5D', CURRENT_TIMESTAMP);
		INSERT INTO query_history (connection_id, account_id, sql_text, status)
		VALUES (30, 1, 'SELECT 1', 'ok');
		INSERT INTO query_favorites (workspace_id, account_id, connection_id, name, sql_text)
		VALUES (10, 1, 30, 'Favorite', 'SELECT 1');
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}

	if err := db.MigrateUp(); err != nil {
		db.Close()
		t.Fatal(err)
	}

	assertMigration46Column(t, db, "connections", "params", true, "'{}'")
	assertMigration46Column(t, db, "organizations", "allow_connection_secret_reveal", true, "0")
	assertMigration46ColumnMissing(t, db, "organizations", "mask_connection_credentials_on_edit")

	conn, found, err := db.GetConnection(ctx, 30)
	if err != nil || !found {
		db.Close()
		t.Fatalf("GetConnection: found=%v err=%v", found, err)
	}
	if string(conn.Params) != `{}` {
		db.Close()
		t.Fatalf("legacy params = %s, want {}", conn.Params)
	}

	for _, table := range []string{"organizations", "workspaces", "connections", "role_bindings", "schema_snapshots", "schema_nodes", "query_history", "query_favorites"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if count == 0 {
			db.Close()
			t.Fatalf("%s rows were not preserved", table)
		}
	}

	wantPermission := map[int64]bool{40: true, 41: true, 42: false, 43: false, 44: false}
	for roleID, want := range wantPermission {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM role_permissions WHERE role_id = ? AND permission = 'conn:reveal_secret'", roleID).Scan(&count); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if got := count == 1; got != want {
			db.Close()
			t.Fatalf("role %d reveal permission = %v, want %v", roleID, got, want)
		}
	}

	if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
		ConnectionID:   30,
		Name:           "password",
		Source:         "stored",
		ValueEncrypted: "ciphertext",
		KeyID:          "key-1",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	var foreignKeyViolations int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&foreignKeyViolations); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if foreignKeyViolations != 0 {
		db.Close()
		t.Fatalf("foreign key violations after migration: %d", foreignKeyViolations)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	migrateSQLiteSteps(t, path, -1)

	db, err = New("sqlite", path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertMigration46Column(t, db, "organizations", "mask_connection_credentials_on_edit", true, "0")
	assertMigration46ColumnMissing(t, db, "organizations", "allow_connection_secret_reveal")
	assertMigration46ColumnMissing(t, db, "connections", "params")
	assertMigration46ColumnMissing(t, db, "connections", "tls_config")
	assertMigration46ColumnMissing(t, db, "connections", "ssh_config")
	assertMigration46Column(t, db, "connections", "dsn_encrypted", true, "")
	for _, table := range []string{"connections", "schema_snapshots", "schema_nodes", "query_history", "query_favorites"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s rows after migration down = %d, want 1", table, count)
		}
	}
}

func migrateSQLiteToVersion(t *testing.T, path string, version uint) {
	t.Helper()
	migrator := newSQLiteTestMigrator(t, path)
	if err := migrator.Migrate(version); err != nil {
		closeTestMigrator(t, migrator)
		t.Fatal(err)
	}
	closeTestMigrator(t, migrator)
}

func migrateSQLiteSteps(t *testing.T, path string, steps int) {
	t.Helper()
	migrator := newSQLiteTestMigrator(t, path)
	if err := migrator.Steps(steps); err != nil {
		closeTestMigrator(t, migrator)
		t.Fatal(err)
	}
	closeTestMigrator(t, migrator)
}

func newSQLiteTestMigrator(t *testing.T, path string) *migrate.Migrate {
	t.Helper()
	source, err := iofs.New(assets.EmbeddedFiles, "migrations_sqlite")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, "sqlite://"+path)
	if err != nil {
		t.Fatal(err)
	}
	return migrator
}

func closeTestMigrator(t *testing.T, migrator *migrate.Migrate) {
	t.Helper()
	sourceErr, databaseErr := migrator.Close()
	if sourceErr != nil {
		t.Error(sourceErr)
	}
	if databaseErr != nil {
		t.Error(databaseErr)
	}
}

func assertMigration46Column(t *testing.T, db *DB, table, column string, notNull bool, defaultValue string) {
	t.Helper()
	var gotNotNull int
	var gotDefault *string
	err := db.QueryRowContext(context.Background(),
		"SELECT `notnull`, dflt_value FROM pragma_table_info(?) WHERE name = ?", table, column,
	).Scan(&gotNotNull, &gotDefault)
	if err != nil {
		t.Fatal(err)
	}
	if (gotNotNull == 1) != notNull {
		t.Fatalf("%s.%s notnull = %d, want %v", table, column, gotNotNull, notNull)
	}
	if defaultValue == "" {
		if gotDefault != nil {
			t.Fatalf("%s.%s default = %q, want NULL", table, column, *gotDefault)
		}
		return
	}
	if gotDefault == nil || *gotDefault != defaultValue {
		t.Fatalf("%s.%s default = %v, want %q", table, column, gotDefault, defaultValue)
	}
}

func assertMigration46ColumnMissing(t *testing.T, db *DB, table, column string) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("%s.%s still exists", table, column)
	}
}
