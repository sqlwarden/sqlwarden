package schema

import (
	"context"
	"testing"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func TestCompletionMetadataProjectsCachedListings(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"}, metadata.Child{Kind: "schema", Name: "pg_catalog", System: true})
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	cat.set(schemaPathOf("app", "public"), "functions", metadata.Child{Kind: "procedure", Name: "tidy"})
	cat.set(tablePathOf("app", "public", "users"), "columns",
		metadata.Child{Kind: "column", Name: "name", Attributes: map[string]any{"data_type": "text", "nullable": true, "ordinal": 2}},
		metadata.Child{Kind: "column", Name: "id", Attributes: map[string]any{"data_type": "integer", "nullable": false, "ordinal": float64(1)}},
	)
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true, DefaultScope: schemaPathOf("app", "public")}
	ctx := context.Background()
	tree := cat.Tree()
	for _, step := range []struct {
		parent metadata.ScopePath
		folder string
	}{
		{dbPath("app"), "schemas"},
		{schemaPathOf("app", "public"), "tables"},
		{schemaPathOf("app", "public"), "functions"},
		{tablePathOf("app", "public", "users"), "columns"},
	} {
		if _, err := n.Children(ctx, conn, tree, cat, step.parent, step.folder); err != nil {
			t.Fatal(err)
		}
	}

	for name, nav := range map[string]*Navigator{"memory": n, "store": newTestNavigator(store)} {
		t.Run(name, func(t *testing.T) {
			set, err := nav.CompletionMetadata(ctx, conn, tree)
			if err != nil {
				t.Fatal(err)
			}
			if set.Directory.DefaultScope != conn.DefaultScope || set.Version == "" {
				t.Fatalf("directory = %+v version %q", set.Directory, set.Version)
			}
			var scopes []metadata.ScopePath
			for _, node := range set.Directory.ScopeNodes() {
				scopes = append(scopes, node.Path)
			}
			if len(scopes) != 2 || scopes[0] != dbPath("app") || scopes[1] != schemaPathOf("app", "public") {
				t.Fatalf("scopes = %v (system schema must be hidden)", scopes)
			}
			refs := set.Directory.ObjectRefs()
			if len(refs) != 2 {
				t.Fatalf("refs = %+v", refs)
			}
			if len(set.Objects) != 1 || set.Objects[0].Relational == nil {
				t.Fatalf("objects = %+v", set.Objects)
			}
			cols := set.Objects[0].Relational.Columns
			if len(cols) != 2 || cols[0].Name != "id" || cols[0].Ordinal != 1 || cols[0].Nullable || cols[1].DataType != "text" {
				t.Fatalf("columns = %+v", cols)
			}
		})
	}
}

func TestCompletionMetadataPrefersCachedDetailAndChangesVersion(t *testing.T) {
	cat := newFakeCatalog()
	users := metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "users"}
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	cat.set(users.Path(), "columns", metadata.Child{Kind: "column", Name: "id", Attributes: map[string]any{"data_type": "integer"}})
	cat.objects[users] = metadata.Object{Ref: users, Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "bigint", Ordinal: 1}}}}
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	ctx := context.Background()
	tree := cat.Tree()
	_, _ = n.Children(ctx, conn, tree, cat, schemaPathOf("app", "public"), "tables")
	_, _ = n.Children(ctx, conn, tree, cat, users.Path(), "columns")
	before, _ := n.CompletionMetadata(ctx, conn, tree)
	if _, err := n.Objects(ctx, conn, cat, []metadata.ObjectRef{users}); err != nil {
		t.Fatal(err)
	}
	after, _ := n.CompletionMetadata(ctx, conn, tree)
	if len(after.Objects) != 1 || after.Objects[0].Relational.Columns[0].DataType != "bigint" {
		t.Fatalf("objects = %+v", after.Objects)
	}
	if before.Version == after.Version {
		t.Fatal("version must change when cached metadata changes")
	}
}

func TestCompletionMetadataOmitsDetailUnderHiddenSystemScopes(t *testing.T) {
	cat := newFakeCatalog()
	catalogTable := metadata.ObjectRef{Scope: schemaPathOf("app", "pg_catalog"), Kind: "table", Name: "pg_class"}
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"}, metadata.Child{Kind: "schema", Name: "pg_catalog", System: true})
	cat.objects[catalogTable] = metadata.Object{Ref: catalogTable}
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	ctx := context.Background()
	tree := cat.Tree()
	if _, err := n.Children(ctx, conn, tree, cat, dbPath("app"), "schemas"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Objects(ctx, conn, cat, []metadata.ObjectRef{catalogTable}); err != nil {
		t.Fatal(err)
	}
	hidden, err := n.CompletionMetadata(ctx, conn, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.Objects) != 0 {
		t.Fatalf("objects = %+v", hidden.Objects)
	}
	conn.ShowSystem = true
	shown, err := n.CompletionMetadata(ctx, conn, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(shown.Objects) != 1 {
		t.Fatalf("objects = %+v", shown.Objects)
	}
}
