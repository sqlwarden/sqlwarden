package yugabyte

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestNavigatorMarksSystemPlatformDatabase(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	folder, ok := d.Tree().Folder("", "databases")
	if !ok {
		t.Fatal("databases folder not declared")
	}
	q, err := d.Querier(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := folder.List(ctx, q, []metadata.ScopePath{""})
	if err != nil {
		t.Fatal(err)
	}
	system := map[string]bool{}
	for _, c := range out[""] {
		system[c.Name] = c.System
	}
	if !system["system_platform"] {
		t.Error("system_platform must be system")
	}
	if system["yugabyte"] {
		t.Error("yugabyte must not be system")
	}
}

func TestYugabyteNavigatorContract(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS nav_yb`,
		`CREATE TABLE IF NOT EXISTS nav_yb.users (id int PRIMARY KEY, email text UNIQUE)`,
		`CREATE TABLE IF NOT EXISTS nav_yb.orders (id int PRIMARY KEY, user_id int REFERENCES nav_yb.users(id))`,
	} {
		if _, err := d.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP SCHEMA IF EXISTS nav_yb CASCADE`) })
	schema := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "yugabyte"}, metadata.ScopeSegment{Kind: "schema", Name: "nav_yb"})
	tables := []metadata.ScopePath{
		schema.Child(metadata.ScopeSegment{Kind: "table", Name: "users"}),
		schema.Child(metadata.ScopeSegment{Kind: "table", Name: "orders"}),
	}
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "schema", Folder: "tables", Parents: []metadata.ScopePath{schema}},
	}
	for _, folder := range []string{"columns", "constraints", "foreign_keys", "indexes", "references"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: folder, Parents: tables})
	}
	enginetest.RunNavigatorContract(t, d, "", cases)
}
