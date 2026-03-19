package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/sqlwarden/internal/driver"
)

func TestPostgresDriver(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set")
	}

	d := &postgresDriver{}
	ctx := context.Background()

	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: dsn, Driver: "postgres"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT 1 AS n, 'hello' AS s")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rs.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(rs.Columns))
	}
	if len(rs.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rs.Rows))
	}
}
