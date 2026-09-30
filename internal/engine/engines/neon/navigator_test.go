package neon

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func connect(t *testing.T) *driver {
	t.Helper()
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "neon"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNeonNavigatorContract(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS nav_neon`,
		`CREATE TABLE IF NOT EXISTS nav_neon.users (id int PRIMARY KEY)`,
	} {
		if _, err := d.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP SCHEMA IF EXISTS nav_neon CASCADE`) })
	schema := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database}, metadata.ScopeSegment{Kind: "schema", Name: "nav_neon"})
	enginetest.RunNavigatorContract(t, d, "", []enginetest.NavigatorCase{
		{NodeKind: "database", Folder: "schemas", Parents: []metadata.ScopePath{schema.Parent()}},
		{NodeKind: "schema", Folder: "tables", Parents: []metadata.ScopePath{schema}},
	})
}
