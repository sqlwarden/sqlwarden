package supabase

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestNavigatorMarksManagedSchemasSystem(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	created := []string{}
	for _, schema := range append(slices.Sorted(maps.Keys(managedSchemas)), "nav_app") {
		var exists bool
		if err := d.DB().QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, schema).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			continue
		}
		if _, err := d.Execute(ctx, `CREATE SCHEMA `+schema); err != nil {
			t.Fatal(err)
		}
		created = append(created, schema)
	}
	t.Cleanup(func() {
		for _, schema := range created {
			_, _ = d.Execute(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		}
	})
	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	parent := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	folder, ok := d.Tree().Folder("database", "schemas")
	if !ok {
		t.Fatal("schemas folder not declared")
	}
	q, err := d.Querier(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	out, err := folder.List(ctx, q, []metadata.ScopePath{parent})
	if err != nil {
		t.Fatal(err)
	}
	system := map[string]bool{}
	for _, c := range out[parent] {
		system[c.Name] = c.System
	}
	for schema := range managedSchemas {
		if !system[schema] {
			t.Errorf("managed schema %q must be system", schema)
		}
	}
	if system["nav_app"] || system["public"] {
		t.Error("user schemas must not be system")
	}
}

func TestSupabaseNavigatorContract(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if err := d.Tree().Validate(); err != nil {
		t.Fatal(err)
	}
	enginetest.RunNavigatorContract(t, d, "", []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "database", Folder: "schemas", Parents: []metadata.ScopePath{
			metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database}),
		}},
	})
}
