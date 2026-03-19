package sqlite

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/driver"
)

func TestSQLiteDriver(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()

	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	// Create a test table.
	_, err := d.Execute(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER)`)
	if err != nil {
		t.Fatalf("Create table: %v", err)
	}

	// Insert rows.
	_, err = d.Execute(ctx, `INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30), (2, 'Bob', 25)`)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Query rows.
	rs, err := d.Query(ctx, `SELECT id, name, age FROM users ORDER BY id`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rs.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(rs.Columns))
	}
	if len(rs.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rs.Rows))
	}

	// Test Tables.
	tables, err := d.Tables(ctx, "", "")
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(tables) != 1 || tables[0].Name != "users" {
		t.Errorf("expected [users], got %v", tables)
	}

	// Test Columns.
	cols, err := d.Columns(ctx, "", "", "users")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(cols) != 3 {
		t.Errorf("expected 3 columns, got %d", len(cols))
	}

	// Test Dialect.
	if d.Dialect() != driver.DialectSQLite {
		t.Errorf("expected dialect sqlite, got %s", d.Dialect())
	}
}
