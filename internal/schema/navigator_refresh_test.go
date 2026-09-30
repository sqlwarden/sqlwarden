package schema

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func seedRefreshCatalog(cat *fakeCatalog) {
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"}, metadata.Child{Kind: "schema", Name: "sales"})
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	cat.set(schemaPathOf("app", "sales"), "tables", metadata.Child{Kind: "table", Name: "orders"})
	cat.set(tablePathOf("app", "sales", "orders"), "columns", metadata.Child{Kind: "column", Name: "id"})
}

func expandAll(t *testing.T, n *Navigator, conn Connection, cat *fakeCatalog) {
	t.Helper()
	ctx := context.Background()
	tree := cat.Tree()
	steps := []struct {
		parent metadata.ScopePath
		folder string
	}{
		{"", "databases"},
		{dbPath("app"), "schemas"},
		{schemaPathOf("app", "public"), "tables"},
		{schemaPathOf("app", "sales"), "tables"},
		{tablePathOf("app", "sales", "orders"), "columns"},
	}
	for _, step := range steps {
		if _, err := n.Children(ctx, conn, tree, cat, step.parent, step.folder); err != nil {
			t.Fatal(err)
		}
	}
	cat.mu.Lock()
	cat.calls = nil
	cat.mu.Unlock()
}

func TestRefreshRequiresSession(t *testing.T) {
	cat := newFakeCatalog()
	_, err := newTestNavigator(nil).Refresh(context.Background(), Connection{ID: 1}, cat.Tree(), nil, "")
	if !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshBatchesOneLoaderCallPerGroup(t *testing.T) {
	cat := newFakeCatalog()
	seedRefreshCatalog(cat)
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	expandAll(t, n, conn, cat)
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"}, metadata.Child{Kind: "table", Name: "audit"})

	listings, err := n.Refresh(context.Background(), conn, cat.Tree(), cat, dbPath("app"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"databases:1", "schemas:1", "tables:2", "columns:1"}
	if got := cat.callLog(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(listings) != 5 || listings[0].Parent != "" || listings[0].Folder != "databases" {
		t.Fatalf("listings = %+v", listings)
	}
	public, _ := n.Children(context.Background(), conn, cat.Tree(), nil, schemaPathOf("app", "public"), "tables")
	if !slices.Equal(itemNames(public), []string{"users", "audit"}) {
		t.Fatalf("refreshed tables = %v", itemNames(public))
	}
}

func TestRefreshLeafRelistsContainingFolderOnly(t *testing.T) {
	cat := newFakeCatalog()
	seedRefreshCatalog(cat)
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	expandAll(t, n, conn, cat)
	column := tablePathOf("app", "sales", "orders").Child(seg("column", "id"))
	if _, err := n.Refresh(context.Background(), conn, cat.Tree(), cat, column); err != nil {
		t.Fatal(err)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"columns:1"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestRefreshLeafDeletesVanishedObjectSubtree(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(schemaPathOf("app", "public"), "tables",
		metadata.Child{Kind: "table", Name: "t1"},
		metadata.Child{Kind: "table", Name: "t2"},
	)
	cat.set(tablePathOf("app", "public", "t2"), "columns", metadata.Child{Kind: "column", Name: "id"})
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true}
	tree := cat.Tree()
	if _, err := n.Children(context.Background(), conn, tree, cat, schemaPathOf("app", "public"), "tables"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Children(context.Background(), conn, tree, cat, tablePathOf("app", "public", "t2"), "columns"); err != nil {
		t.Fatal(err)
	}
	cat.mu.Lock()
	cat.calls = nil
	cat.mu.Unlock()
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "t1"})
	n = newTestNavigator(store)

	listings, err := n.Refresh(context.Background(), conn, tree, cat, tablePathOf("app", "public", "t2"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"tables:1"}) {
		t.Fatalf("calls = %v", got)
	}
	if len(listings) != 1 || !slices.Equal(itemNames(listings[0]), []string{"t1"}) {
		t.Fatalf("listings = %+v", listings)
	}
	if rows, _ := store.SchemaListingsWithin(context.Background(), 1, string(tablePathOf("app", "public", "t2"))); len(rows) != 0 {
		t.Fatalf("store kept %d listings under vanished table", len(rows))
	}
	if _, err := n.Children(context.Background(), conn, tree, nil, tablePathOf("app", "public", "t2"), "columns"); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("vanished table subtree still in memory: %v", err)
	}
}

func TestRefreshDeletesVanishedSubtree(t *testing.T) {
	cat := newFakeCatalog()
	seedRefreshCatalog(cat)
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true}
	expandAll(t, n, conn, cat)
	sales := schemaPathOf("app", "sales")
	orders := metadata.ObjectRef{Scope: sales, Kind: "table", Name: "orders"}
	_ = store.UpsertSchemaObjects(context.Background(), []database.SchemaObject{{ConnectionID: 1, Scope: string(sales), Kind: "table", Name: "orders", ObjectData: []byte("x")}})
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"})

	if _, err := n.Refresh(context.Background(), conn, cat.Tree(), cat, dbPath("app")); err != nil {
		t.Fatal(err)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"databases:1", "schemas:1", "tables:1"}) {
		t.Fatalf("vanished parents must not be queried: %v", got)
	}
	if rows, _ := store.SchemaListingsWithin(context.Background(), 1, string(sales)); len(rows) != 0 {
		t.Fatalf("store kept %d listings under dropped schema", len(rows))
	}
	if _, ok := store.objects[database.SchemaObjectKey{Scope: string(orders.Scope), Kind: orders.Kind, Name: orders.Name}]; ok {
		t.Fatal("store kept object detail under dropped schema")
	}
	restarted := newTestNavigator(store)
	if _, err := restarted.Children(context.Background(), conn, cat.Tree(), nil, sales, "tables"); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("dropped subtree still served: %v", err)
	}
	if _, err := n.Children(context.Background(), conn, cat.Tree(), nil, sales, "tables"); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("dropped subtree still in memory: %v", err)
	}
}

func TestRefreshInvalidatesDetailWithinAndAboveRoot(t *testing.T) {
	cat := newFakeCatalog()
	seedRefreshCatalog(cat)
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true}
	ctx := context.Background()
	public := schemaPathOf("app", "public")
	_ = store.UpsertSchemaObjects(ctx, []database.SchemaObject{
		{ConnectionID: 1, Scope: string(public), Kind: "table", Name: "users", ObjectData: []byte("x")},
		{ConnectionID: 1, Scope: string(schemaPathOf("app", "sales")), Kind: "table", Name: "orders", ObjectData: []byte("x")},
	})
	for _, scope := range []metadata.ScopePath{"", dbPath("app"), public, schemaPathOf("app", "sales")} {
		_ = store.UpsertSchemaRelationship(ctx, database.SchemaRelationship{ConnectionID: 1, Scope: string(scope), Data: []byte("g")})
	}
	if _, err := n.Refresh(ctx, conn, cat.Tree(), cat, public); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 1 {
		t.Fatalf("objects = %v", store.objects)
	}
	if _, ok := store.relationships[string(schemaPathOf("app", "sales"))]; !ok || len(store.relationships) != 1 {
		t.Fatalf("relationships = %v", store.relationships)
	}
}
