package schema

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func TestCompletionViewProjectsCachedListings(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"})
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	cat.set(tablePathOf("app", "public", "users"), "columns",
		metadata.Child{Kind: "column", Name: "name", Attributes: map[string]any{"data_type": "text", "ordinal": 2}},
		metadata.Child{Kind: "column", Name: "id", Attributes: map[string]any{"data_type": "integer", "ordinal": float64(1)}},
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
		{tablePathOf("app", "public", "users"), "columns"},
	} {
		if _, err := n.Children(ctx, conn, tree, cat, step.parent, step.folder); err != nil {
			t.Fatal(err)
		}
	}
	users := metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "users"}
	for name, nav := range map[string]*Navigator{"memory": n, "store": newTestNavigator(store)} {
		t.Run(name, func(t *testing.T) {
			view, err := nav.CompletionView(ctx, conn, tree, nil)
			if err != nil {
				t.Fatal(err)
			}
			if view.DefaultScope() != conn.DefaultScope {
				t.Fatalf("default = %q", view.DefaultScope())
			}
			refs, loaded := view.Objects(schemaPathOf("app", "public"), "table")
			if !loaded || len(refs) != 1 || refs[0] != users {
				t.Fatalf("refs = %v loaded %v", refs, loaded)
			}
			cols, loaded := view.Columns(users)
			if !loaded || len(cols) != 2 || cols[0].Name != "id" {
				t.Fatalf("columns = %+v", cols)
			}
			if _, loaded := view.Objects(schemaPathOf("app", "public"), "function"); loaded {
				t.Fatal("functions folder was never listed")
			}
		})
	}
}

func TestCompletionViewHidesSystemSubtrees(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("app"), "schemas",
		metadata.Child{Kind: "schema", Name: "public"},
		metadata.Child{Kind: "schema", Name: "pg_catalog", System: true})
	cat.set(schemaPathOf("app", "pg_catalog"), "tables", metadata.Child{Kind: "table", Name: "pg_class"})
	n := newTestNavigator(nil)
	ctx := context.Background()
	tree := cat.Tree()
	shown := Connection{ID: 1, ShowSystem: true}
	for _, step := range []struct {
		parent metadata.ScopePath
		folder string
	}{{dbPath("app"), "schemas"}, {schemaPathOf("app", "pg_catalog"), "tables"}} {
		if _, err := n.Children(ctx, shown, tree, cat, step.parent, step.folder); err != nil {
			t.Fatal(err)
		}
	}
	hidden := Connection{ID: 1}
	view, err := n.CompletionView(ctx, hidden, tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	scopes, _ := view.Scopes(dbPath("app"))
	if len(scopes) != 1 || scopes[0] != schemaPathOf("app", "public") {
		t.Fatalf("scopes = %v", scopes)
	}
	for _, ref := range view.Refs() {
		if ref.Scope == schemaPathOf("app", "pg_catalog") {
			t.Fatalf("system object leaked: %v", ref)
		}
	}
	visible, _ := n.CompletionView(ctx, shown, tree, nil)
	if refs, _ := visible.Objects(schemaPathOf("app", "pg_catalog"), "table"); len(refs) != 1 {
		t.Fatalf("show-system refs = %v", refs)
	}
}

func TestCompletionViewPrefersCachedDetail(t *testing.T) {
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
	if _, err := n.Objects(ctx, conn, cat, []metadata.ObjectRef{users}); err != nil {
		t.Fatal(err)
	}
	view, err := n.CompletionView(ctx, conn, tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cols, _ := view.Columns(users); len(cols) != 1 || cols[0].DataType != "bigint" {
		t.Fatalf("columns = %+v", cols)
	}
}

func TestCompletionViewAppliesShowAllDatabases(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"}, metadata.Child{Kind: "database", Name: "hidden"})
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"})
	cat.set(dbPath("hidden"), "schemas", metadata.Child{Kind: "schema", Name: "secret"})
	ctx := context.Background()
	tree := cat.Tree()
	rootDemand := metadata.Demand{Parent: "", Folder: "databases"}
	only := []metadata.ScopePath{dbPath("app")}

	t.Run("root unlisted", func(t *testing.T) {
		n := newTestNavigator(nil)
		_, _ = n.Children(ctx, Connection{ID: 1, ShowAllDatabases: true}, tree, cat, dbPath("hidden"), "schemas")
		off, err := n.CompletionView(ctx, Connection{ID: 1, DefaultScope: dbPath("app")}, tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		if scopes, loaded := off.Scopes(""); !loaded || !slices.Equal(scopes, only) {
			t.Fatalf("scopes = %v loaded %v", scopes, loaded)
		}
		if got := off.ScopeDemands(""); len(got) != 0 {
			t.Fatalf("demands = %v, want none", got)
		}
		if slices.Contains(off.ScopePaths(), schemaPathOf("hidden", "secret")) {
			t.Fatalf("hidden database leaked: %v", off.ScopePaths())
		}
		on, err := n.CompletionView(ctx, Connection{ID: 1, DefaultScope: dbPath("app"), ShowAllDatabases: true}, tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := on.ScopeDemands(""); !slices.Equal(got, []metadata.Demand{rootDemand}) {
			t.Fatalf("show-all demands = %v", got)
		}
	})

	t.Run("root listed", func(t *testing.T) {
		n := newTestNavigator(nil)
		showAll := Connection{ID: 1, DefaultScope: dbPath("app"), ShowAllDatabases: true}
		_, _ = n.Children(ctx, showAll, tree, cat, "", "databases")
		_, _ = n.Children(ctx, showAll, tree, cat, dbPath("hidden"), "schemas")
		off, err := n.CompletionView(ctx, Connection{ID: 1, DefaultScope: dbPath("app")}, tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		if scopes, _ := off.Scopes(""); !slices.Equal(scopes, only) {
			t.Fatalf("scopes = %v", scopes)
		}
		if slices.Contains(off.ScopePaths(), schemaPathOf("hidden", "secret")) {
			t.Fatalf("hidden database leaked: %v", off.ScopePaths())
		}
		on, err := n.CompletionView(ctx, showAll, tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		if scopes, _ := on.Scopes(""); !slices.Equal(scopes, []metadata.ScopePath{dbPath("app"), dbPath("hidden")}) {
			t.Fatalf("show-all scopes = %v", scopes)
		}
	})
}

func TestCompletionViewShowAllKeepsDatabasesMissingFromStaleRoot(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	ctx := context.Background()
	tree := cat.Tree()
	showAll := Connection{ID: 1, DefaultScope: dbPath("app"), ShowAllDatabases: true}
	n := newTestNavigator(nil)
	_, _ = n.Children(ctx, showAll, tree, cat, "", "databases")
	cat.set(dbPath("newdb"), "schemas", metadata.Child{Kind: "schema", Name: "public"})
	cat.set(schemaPathOf("newdb", "public"), "tables", metadata.Child{Kind: "table", Name: "orders"})
	_, _ = n.Children(ctx, showAll, tree, cat, dbPath("newdb"), "schemas")
	_, _ = n.Children(ctx, showAll, tree, cat, schemaPathOf("newdb", "public"), "tables")
	_, _ = n.Children(ctx, showAll, tree, cat, dbPath("typo"), "schemas")

	on, err := n.CompletionView(ctx, showAll, tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scopes, _ := on.Scopes(""); !slices.Equal(scopes, []metadata.ScopePath{dbPath("app"), dbPath("newdb")}) {
		t.Fatalf("show-all scopes = %v", scopes)
	}
	if refs, loaded := on.Objects(schemaPathOf("newdb", "public"), "table"); !loaded || len(refs) != 1 {
		t.Fatalf("show-all newdb tables = %v loaded %v", refs, loaded)
	}

	off, err := n.CompletionView(ctx, Connection{ID: 1, DefaultScope: dbPath("app")}, tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scopes, _ := off.Scopes(""); !slices.Equal(scopes, []metadata.ScopePath{dbPath("app")}) {
		t.Fatalf("scopes = %v", scopes)
	}
	if slices.Contains(off.ScopePaths(), schemaPathOf("newdb", "public")) {
		t.Fatalf("newdb leaked with show-all off: %v", off.ScopePaths())
	}
}

type scopedCatalog struct {
	*fakeCatalog
	scope metadata.ScopePath
	err   error
	calls int
}

func (c *scopedCatalog) CurrentScope(context.Context) (metadata.ScopePath, error) {
	c.calls++
	return c.scope, c.err
}

func TestCompletionViewIgnoresPersistedCurrentFlags(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app", Current: true}, metadata.Child{Kind: "database", Name: "other"})
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "sales", Current: true}, metadata.Child{Kind: "schema", Name: "public"})
	store := newFakeStore()
	ctx := context.Background()
	tree := cat.Tree()
	for _, conn := range []Connection{{ID: 1, Persistent: true}, {ID: 1, Persistent: true, DefaultScope: dbPath("app")}} {
		writer := newTestNavigator(store)
		_, _ = writer.Children(ctx, conn, tree, cat, "", "databases")
		_, _ = writer.Children(ctx, conn, tree, cat, dbPath("app"), "schemas")

		reader := newTestNavigator(store)
		view, err := reader.CompletionView(ctx, conn, tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := view.DefaultScope(); got != conn.DefaultScope {
			t.Fatalf("configured %q without session: default scope = %q", conn.DefaultScope, got)
		}
		live := &scopedCatalog{fakeCatalog: cat, scope: schemaPathOf("app", "public")}
		view, err = reader.CompletionView(ctx, conn, tree, live)
		if err != nil {
			t.Fatal(err)
		}
		if got := view.DefaultScope(); got != schemaPathOf("app", "public") {
			t.Fatalf("configured %q with session: default scope = %q", conn.DefaultScope, got)
		}
	}
}

func TestCompletionViewMemoizesSessionScope(t *testing.T) {
	cat := newFakeCatalog()
	tree := cat.Tree()
	ctx := context.Background()
	conn := Connection{ID: 1}
	now := fixedNow()
	n := newTestNavigator(nil)
	n.now = func() time.Time { return now }
	live := &scopedCatalog{fakeCatalog: cat, scope: schemaPathOf("app", "public")}
	read := func() metadata.ScopePath {
		t.Helper()
		view, err := n.CompletionView(ctx, conn, tree, live)
		if err != nil {
			t.Fatal(err)
		}
		return view.DefaultScope()
	}

	if got := read(); got != schemaPathOf("app", "public") || live.calls != 1 {
		t.Fatalf("first read = %q calls %d", got, live.calls)
	}
	live.scope = schemaPathOf("app", "sales")
	now = now.Add(29 * time.Second)
	if got := read(); got != schemaPathOf("app", "public") || live.calls != 1 {
		t.Fatalf("memoized read = %q calls %d", got, live.calls)
	}
	now = now.Add(2 * time.Second)
	if got := read(); got != schemaPathOf("app", "sales") || live.calls != 2 {
		t.Fatalf("expired read = %q calls %d", got, live.calls)
	}

	if view, _ := n.CompletionView(ctx, conn, tree, nil); view.DefaultScope() != "" {
		t.Fatalf("memoized scope without session: default scope = %q", view.DefaultScope())
	}

	live.scope = schemaPathOf("app", "public")
	n.ForgetConnection(conn.ID)
	if got := read(); got != schemaPathOf("app", "public") || live.calls != 3 {
		t.Fatalf("read after forget = %q calls %d", got, live.calls)
	}

	other := Connection{ID: 2}
	failing := &scopedCatalog{fakeCatalog: cat, scope: schemaPathOf("app", "public"), err: errors.New("boom")}
	view, err := n.CompletionView(ctx, other, tree, failing)
	if err != nil {
		t.Fatal(err)
	}
	if got := view.DefaultScope(); got != "" {
		t.Fatalf("failed session scope: default scope = %q", got)
	}
	if view, _ := n.CompletionView(ctx, other, tree, nil); view.DefaultScope() != "" {
		t.Fatalf("without session: default scope = %q", view.DefaultScope())
	}
	if view, _ := n.CompletionView(ctx, other, tree, cat); view.DefaultScope() != "" {
		t.Fatalf("live without a current scope: default scope = %q", view.DefaultScope())
	}
}

func TestCompletionViewReflectsStoreChangesBetweenReads(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"})
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	store := newFakeStore()
	conn := Connection{ID: 1, Persistent: true, ShowAllDatabases: true, DefaultScope: schemaPathOf("app", "public")}
	ctx := context.Background()
	tree := cat.Tree()
	writer := newTestNavigator(store)
	for _, step := range []struct {
		parent metadata.ScopePath
		folder string
	}{
		{dbPath("app"), "schemas"},
		{schemaPathOf("app", "public"), "tables"},
	} {
		if _, err := writer.Children(ctx, conn, tree, cat, step.parent, step.folder); err != nil {
			t.Fatal(err)
		}
	}
	reader := newTestNavigator(store)
	tables := func() []string {
		view, err := reader.CompletionView(ctx, conn, tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs, _ := view.Objects(schemaPathOf("app", "public"), "table")
		var names []string
		for _, ref := range refs {
			names = append(names, ref.Name)
		}
		return names
	}
	if got := tables(); !slices.Equal(got, []string{"users"}) {
		t.Fatalf("first read = %v", got)
	}
	if got := tables(); !slices.Equal(got, []string{"users"}) {
		t.Fatalf("repeat read = %v", got)
	}

	row, err := encodeListing(conn.ID, Listing{Parent: schemaPathOf("app", "public"), Folder: "tables", Items: []metadata.Child{{Kind: "table", Name: "users"}, {Kind: "table", Name: "orders"}}, FetchedAt: fixedNow()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSchemaListings(ctx, []database.SchemaListing{row}); err != nil {
		t.Fatal(err)
	}
	if got := tables(); !slices.Equal(got, []string{"users", "orders"}) && !slices.Equal(got, []string{"orders", "users"}) {
		t.Fatalf("read after store change = %v", got)
	}

	store.mu.Lock()
	delete(store.listings, string(schemaPathOf("app", "public"))+"#tables")
	store.mu.Unlock()
	if got := tables(); len(got) != 0 {
		t.Fatalf("read after row removal = %v", got)
	}
}
