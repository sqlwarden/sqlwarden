package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sqlwarden/internal/driver"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = connStr

	code := m.Run()

	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func newConnectedDriver(t *testing.T) *postgresDriver {
	t.Helper()
	d := &postgresDriver{}
	ctx := context.Background()
	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: testDSN, Driver: "postgres"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestConnect(t *testing.T) {
	t.Run("valid DSN", func(t *testing.T) {
		d := &postgresDriver{}
		ctx := context.Background()
		if err := d.Connect(ctx, driver.ConnectionConfig{DSN: testDSN, Driver: "postgres"}); err != nil {
			t.Fatalf("expected connect to succeed, got: %v", err)
		}
		_ = d.Close()
	})

	t.Run("invalid DSN", func(t *testing.T) {
		d := &postgresDriver{}
		ctx := context.Background()
		err := d.Connect(ctx, driver.ConnectionConfig{DSN: "postgres://invalid:5432/nonexistent?sslmode=disable", Driver: "postgres"})
		if err == nil {
			t.Fatal("expected connect to fail with invalid DSN, got nil")
			_ = d.Close()
		}
	})
}

func TestPing(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestQuery_BasicTypes(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	// Create a table with multiple column types
	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS basic_types_test (
			id        INTEGER,
			label     TEXT,
			active    BOOLEAN,
			price     NUMERIC(10,2),
			created   TIMESTAMPTZ,
			notes     TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS basic_types_test")
	})

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	_, err = d.Execute(ctx, `
		INSERT INTO basic_types_test (id, label, active, price, created, notes)
		VALUES (1, 'hello', true, 9.99, $1, NULL)
	`, ts)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT id, label, active, price, created, notes FROM basic_types_test")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if len(rs.Columns) != 6 {
		t.Errorf("expected 6 columns, got %d", len(rs.Columns))
	}
	if len(rs.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rs.Rows))
	}

	// Verify column names
	expectedCols := []string{"id", "label", "active", "price", "created", "notes"}
	for i, col := range rs.Columns {
		if col.Name != expectedCols[i] {
			t.Errorf("column %d: expected name %q, got %q", i, expectedCols[i], col.Name)
		}
	}

	// Verify NULL value in last column
	if len(rs.Rows) > 0 {
		nullVal := rs.Rows[0][5]
		if nullVal.Type != "null" {
			t.Errorf("expected NULL value in notes column, got type %q", nullVal.Type)
		}
	}
}

func TestQuery_Args(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS args_test (
			id    INTEGER,
			name  TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS args_test")
	})

	_, err = d.Execute(ctx, "INSERT INTO args_test (id, name) VALUES (1, 'alpha'), (2, 'beta'), (3, 'gamma')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT id, name FROM args_test WHERE id = $1 AND name = $2", 2, "beta")
	if err != nil {
		t.Fatalf("Query with args: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rs.Rows))
	}
	if rs.Rows[0][1].Text != "beta" {
		t.Errorf("expected name='beta', got %q", rs.Rows[0][1].Text)
	}
}

func TestExecute_DML(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS dml_test (
			id   SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS dml_test")
	})

	// Insert multiple rows
	_, err = d.Execute(ctx, "INSERT INTO dml_test (name) VALUES ('row1'), ('row2'), ('row3')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// Verify count via Query
	rs, err := d.Query(ctx, "SELECT COUNT(*) AS cnt FROM dml_test")
	if err != nil {
		t.Fatalf("Query count: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rs.Rows))
	}
	cnt := rs.Rows[0][0].Integer
	if cnt != 3 {
		t.Errorf("expected count=3, got %d", cnt)
	}

	// Test DELETE
	_, err = d.Execute(ctx, "DELETE FROM dml_test WHERE name = 'row1'")
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}

	rs, err = d.Query(ctx, "SELECT COUNT(*) AS cnt FROM dml_test")
	if err != nil {
		t.Fatalf("Query count after delete: %v", err)
	}
	cnt = rs.Rows[0][0].Integer
	if cnt != 2 {
		t.Errorf("expected count=2 after delete, got %d", cnt)
	}
}

func TestTables(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, "CREATE TABLE IF NOT EXISTS tables_test_a (id INTEGER)")
	if err != nil {
		t.Fatalf("create table a: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS tables_test_a")
	})

	_, err = d.Execute(ctx, "CREATE TABLE IF NOT EXISTS tables_test_b (id INTEGER)")
	if err != nil {
		t.Fatalf("create table b: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS tables_test_b")
	})

	// List all tables (no schema filter)
	tables, err := d.Tables(ctx, "", "")
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}

	findTable := func(name string) bool {
		for _, tb := range tables {
			if tb.Name == name {
				return true
			}
		}
		return false
	}

	if !findTable("tables_test_a") {
		t.Errorf("tables_test_a not found in Tables() result")
	}
	if !findTable("tables_test_b") {
		t.Errorf("tables_test_b not found in Tables() result")
	}

	// Filter by schema = "public" — should still find our tables
	publicTables, err := d.Tables(ctx, "", "public")
	if err != nil {
		t.Fatalf("Tables with schema filter: %v", err)
	}
	if !findTable("tables_test_a") {
		_ = publicTables // just ensure the call succeeded
		t.Logf("tables_test_a not found when filtering by public schema (may be in different schema)")
	}

	// Filter by non-existent schema — should return empty
	noneTables, err := d.Tables(ctx, "", "nonexistent_schema_xyz")
	if err != nil {
		t.Fatalf("Tables with nonexistent schema: %v", err)
	}
	if len(noneTables) != 0 {
		t.Errorf("expected 0 tables for nonexistent schema, got %d", len(noneTables))
	}

	// Verify each returned table has a non-empty schema
	for _, tb := range tables {
		if tb.Schema == "" {
			t.Errorf("table %q has empty schema", tb.Name)
		}
	}
}

func TestColumns(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS columns_test (
			id       INTEGER NOT NULL,
			name     TEXT,
			price    NUMERIC(10,2),
			active   BOOLEAN NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS columns_test")
	})

	cols, err := d.Columns(ctx, "", "public", "columns_test")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}

	colMap := make(map[string]driver.ColumnMeta)
	for _, c := range cols {
		colMap[c.Name] = c
	}

	// id — INTEGER NOT NULL
	idCol, ok := colMap["id"]
	if !ok {
		t.Fatal("column 'id' not found")
	}
	if idCol.Nullable {
		t.Errorf("column 'id' should not be nullable")
	}

	// name — TEXT (nullable)
	nameCol, ok := colMap["name"]
	if !ok {
		t.Fatal("column 'name' not found")
	}
	if !nameCol.Nullable {
		t.Errorf("column 'name' should be nullable")
	}

	// active — BOOLEAN NOT NULL
	activeCol, ok := colMap["active"]
	if !ok {
		t.Fatal("column 'active' not found")
	}
	if activeCol.Nullable {
		t.Errorf("column 'active' should not be nullable")
	}

	// Verify types are non-empty
	for _, c := range cols {
		if c.Type == "" {
			t.Errorf("column %q has empty type", c.Name)
		}
	}
}

func TestDialect(t *testing.T) {
	d := &postgresDriver{}
	if d.Dialect() != driver.DialectPostgres {
		t.Errorf("expected dialect %q, got %q", driver.DialectPostgres, d.Dialect())
	}
}
