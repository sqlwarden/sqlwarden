package metadata

import (
	"slices"
	"testing"
)

func viewTestTree() Tree {
	return Tree{
		Root: Node{Folders: []Folder{{Kind: "databases", Child: "database"}}},
		Nodes: map[string]Node{
			"database": {Scope: true, ShowAllDatabases: true, Folders: []Folder{{Kind: "schemas", Child: "schema"}}},
			"schema": {Scope: true, Folders: []Folder{
				{Kind: "tables", Child: "table"},
				{Kind: "views", Child: "view", MixedKinds: []string{"materialized_view"}},
				{Kind: "functions", Child: "function"},
			}},
			"table":             {Relational: true, Folders: []Folder{{Kind: "columns", Child: "column"}}},
			"view":              {Relational: true, Folders: []Folder{{Kind: "columns", Child: "column"}}},
			"materialized_view": {Relational: true, Folders: []Folder{{Kind: "columns", Child: "column"}}},
			"column":            {Leaf: true, Column: true},
			"function":          {Leaf: true},
		},
	}
}

var (
	vApp    = NewScopePath(ScopeSegment{Kind: "database", Name: "app"})
	vPublic = vApp.Child(ScopeSegment{Kind: "schema", Name: "public"})
)

func TestCompletionViewScopesReportLoadedState(t *testing.T) {
	view := NewCompletionView(viewTestTree(), vPublic, "", map[ListingKey][]Child{
		{Parent: "", Folder: "databases"}: {{Kind: "database", Name: "app"}},
	}, nil)
	roots, loaded := view.Scopes("")
	if !loaded || !slices.Equal(roots, []ScopePath{vApp}) {
		t.Fatalf("roots = %v loaded %v", roots, loaded)
	}
	schemas, loaded := view.Scopes(vApp)
	if loaded || len(schemas) != 0 {
		t.Fatalf("schemas = %v loaded %v, want unloaded", schemas, loaded)
	}
	if got := view.ScopeDemands(vApp); !slices.Equal(got, []Demand{{Parent: vApp, Folder: "schemas"}}) {
		t.Fatalf("demands = %v", got)
	}
	if got := view.ScopeDemands(""); len(got) != 0 {
		t.Fatalf("root demands = %v, want none", got)
	}
}

func TestCompletionViewObjectsMergeListingsAndDetail(t *testing.T) {
	orders := ObjectRef{Scope: vPublic, Kind: "table", Name: "orders"}
	daily := ObjectRef{Scope: vPublic, Kind: "materialized_view", Name: "daily"}
	view := NewCompletionView(viewTestTree(), vPublic, "", map[ListingKey][]Child{
		{Parent: vPublic, Folder: "tables"}: {{Kind: "table", Name: "orders"}},
	}, map[ObjectRef]Object{daily: {Ref: daily}})
	refs, loaded := view.Objects(vPublic, "table", "materialized_view")
	if loaded {
		t.Fatal("views folder is unlisted, want loaded=false")
	}
	if !slices.Equal(refs, []ObjectRef{daily, orders}) {
		t.Fatalf("refs = %v", refs)
	}
	if got := view.ObjectDemands(vPublic, "table", "materialized_view"); !slices.Equal(got, []Demand{{Parent: vPublic, Folder: "views"}}) {
		t.Fatalf("demands = %v", got)
	}
	if _, loaded := view.Objects(vPublic, "procedure"); !loaded {
		t.Fatal("no folder holds procedure, want loaded=true")
	}
}

func TestCompletionViewListedEmptyFolderIsLoaded(t *testing.T) {
	view := NewCompletionView(viewTestTree(), vPublic, "", map[ListingKey][]Child{
		{Parent: vPublic, Folder: "tables"}: {},
	}, nil)
	refs, loaded := view.Objects(vPublic, "table")
	if !loaded || len(refs) != 0 {
		t.Fatalf("refs = %v loaded %v", refs, loaded)
	}
	if got := view.ObjectDemands(vPublic, "table"); len(got) != 0 {
		t.Fatalf("listed folder produced demands %v", got)
	}
}

func TestCompletionViewColumnsPreferDetailThenListing(t *testing.T) {
	orders := ObjectRef{Scope: vPublic, Kind: "table", Name: "orders"}
	users := ObjectRef{Scope: vPublic, Kind: "table", Name: "users"}
	ghost := ObjectRef{Scope: vPublic, Kind: "table", Name: "ghost"}
	fn := ObjectRef{Scope: vPublic, Kind: "function", Name: "tidy"}
	view := NewCompletionView(viewTestTree(), vPublic, "", map[ListingKey][]Child{
		{Parent: users.Path(), Folder: "columns"}: {
			{Kind: "column", Name: "name", Attributes: map[string]any{"data_type": "text", "nullable": true, "ordinal": 2}},
			{Kind: "column", Name: "id", Attributes: map[string]any{"data_type": "integer", "ordinal": float64(1)}},
		},
	}, map[ObjectRef]Object{orders: {Ref: orders, Relational: &RelationalDetail{Columns: []Column{{Name: "id", DataType: "bigint"}}}}})

	if cols, loaded := view.Columns(orders); !loaded || len(cols) != 1 || cols[0].DataType != "bigint" {
		t.Fatalf("orders = %+v %v", cols, loaded)
	}
	cols, loaded := view.Columns(users)
	if !loaded || len(cols) != 2 || cols[0].Name != "id" || cols[0].Nullable || cols[1].DataType != "text" || !cols[1].Nullable {
		t.Fatalf("users = %+v %v", cols, loaded)
	}
	if _, loaded := view.Columns(ghost); loaded {
		t.Fatal("ghost columns unlisted, want loaded=false")
	}
	if got := view.ColumnDemands(ghost); !slices.Equal(got, []Demand{{Parent: ghost.Path(), Folder: "columns"}}) {
		t.Fatalf("demands = %v", got)
	}
	if _, loaded := view.Columns(fn); !loaded {
		t.Fatal("function has no column folder, want loaded=true")
	}
}

func TestCompletionViewProjections(t *testing.T) {
	orders := ObjectRef{Scope: vPublic, Kind: "table", Name: "orders"}
	view := NewCompletionView(viewTestTree(), vPublic, "", map[ListingKey][]Child{
		{Parent: "", Folder: "databases"}:          {{Kind: "database", Name: "app"}},
		{Parent: vApp, Folder: "schemas"}:          {{Kind: "schema", Name: "public"}},
		{Parent: vPublic, Folder: "tables"}:        {{Kind: "table", Name: "orders"}},
		{Parent: orders.Path(), Folder: "columns"}: {{Kind: "column", Name: "id"}},
	}, nil)
	if got := view.ScopePaths(); !slices.Equal(got, []ScopePath{vApp, vPublic}) {
		t.Fatalf("scopes = %v", got)
	}
	if got := view.Refs(); !slices.Equal(got, []ObjectRef{orders}) {
		t.Fatalf("refs = %v", got)
	}
	if NewCompletionView(viewTestTree(), "", "", nil, nil).Empty() != true || view.Empty() {
		t.Fatal("Empty mismatch")
	}
}

func TestCompletionViewScopePathsIgnoreUnlistedParents(t *testing.T) {
	sales := vApp.Child(ScopeSegment{Kind: "schema", Name: "sales"})
	phantom := vApp.Child(ScopeSegment{Kind: "schema", Name: "Sales"})
	view := NewCompletionView(viewTestTree(), "", "", map[ListingKey][]Child{
		{Parent: "", Folder: "databases"}:   {{Kind: "database", Name: "app"}},
		{Parent: vApp, Folder: "schemas"}:   {{Kind: "schema", Name: "sales"}},
		{Parent: sales, Folder: "tables"}:   {{Kind: "table", Name: "orders"}},
		{Parent: phantom, Folder: "tables"}: {},
	}, nil)
	if got := view.ScopePaths(); !slices.Equal(got, []ScopePath{vApp, sales}) {
		t.Fatalf("scopes = %v", got)
	}
}

func TestCompletionViewScopePathsIncludeDefaultScope(t *testing.T) {
	view := NewCompletionView(viewTestTree(), vPublic, "", map[ListingKey][]Child{
		{Parent: vPublic, Folder: "tables"}: {{Kind: "table", Name: "orders"}},
	}, nil)
	if got := view.ScopePaths(); !slices.Equal(got, []ScopePath{vApp, vPublic}) {
		t.Fatalf("scopes = %v", got)
	}
}

func TestCompletionViewDefaultScopeExtendsConfiguredWithSessionScope(t *testing.T) {
	for _, configured := range []ScopePath{"", vApp, vPublic} {
		view := NewCompletionView(viewTestTree(), configured, vPublic, nil, nil)
		if got := view.DefaultScope(); got != vPublic {
			t.Fatalf("configured %q: default scope = %q, want %q", configured, got, vPublic)
		}
	}
}

func TestCompletionViewDefaultScopeKeepsConfiguredWhenSessionDisagrees(t *testing.T) {
	other := NewScopePath(ScopeSegment{Kind: "database", Name: "other"})
	appSales := vApp.Child(ScopeSegment{Kind: "schema", Name: "sales"})
	cases := []struct{ configured, session ScopePath }{
		{other, vPublic},
		{appSales, vPublic},
		{vPublic, vApp},
		{vApp, ""},
		{vApp, NewScopePath(ScopeSegment{Kind: "database", Name: "app"}, ScopeSegment{Kind: "table", Name: "orders"})},
	}
	for _, tc := range cases {
		if got := NewCompletionView(viewTestTree(), tc.configured, tc.session, nil, nil).DefaultScope(); got != tc.configured {
			t.Fatalf("configured %q session %q: default scope = %q", tc.configured, tc.session, got)
		}
	}
}

func TestCompletionViewDefaultScopeIgnoresCachedCurrentFlags(t *testing.T) {
	stale := map[ListingKey][]Child{
		{Parent: "", Folder: "databases"}: {{Kind: "database", Name: "other"}, {Kind: "database", Name: "app", Current: true}},
		{Parent: vApp, Folder: "schemas"}: {{Kind: "schema", Name: "sales", Current: true}, {Kind: "schema", Name: "public"}},
	}
	if got := NewCompletionView(viewTestTree(), "", "", stale, nil).DefaultScope(); got != "" {
		t.Fatalf("without session: default scope = %q, want empty", got)
	}
	if got := NewCompletionView(viewTestTree(), vApp, "", stale, nil).DefaultScope(); got != vApp {
		t.Fatalf("configured database: default scope = %q, want %q", got, vApp)
	}
	if got := NewCompletionView(viewTestTree(), vApp, vPublic, stale, nil).DefaultScope(); got != vPublic {
		t.Fatalf("with session: default scope = %q, want %q", got, vPublic)
	}
}

func fallbackTestTree() Tree {
	tree := viewTestTree()
	tree.FallbackScopes = []string{"dbo"}
	return tree
}

var (
	vSales = vApp.Child(ScopeSegment{Kind: "schema", Name: "sales"})
	vDbo   = vApp.Child(ScopeSegment{Kind: "schema", Name: "dbo"})
)

func TestCompletionViewSearchScopesAppendFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tree    Tree
		def     ScopePath
		session ScopePath
		want    []ScopePath
	}{
		{"schema default searches dbo sibling", fallbackTestTree(), vSales, "", []ScopePath{vSales, vDbo}},
		{"dbo default is not repeated", fallbackTestTree(), vDbo, "", []ScopePath{vDbo}},
		{"database default searches dbo child", fallbackTestTree(), vApp, "", []ScopePath{vApp, vDbo}},
		{"session schema extends database default", fallbackTestTree(), vApp, vPublic, []ScopePath{vPublic, vDbo}},
		{"no fallbacks keeps default only", viewTestTree(), vSales, "", []ScopePath{vSales}},
		{"no default searches nothing", fallbackTestTree(), "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewCompletionView(tc.tree, tc.def, tc.session, nil, nil).SearchScopes()
			if !slices.Equal(got, tc.want) {
				t.Fatalf("search scopes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTreeCloneKeepsFallbackScopes(t *testing.T) {
	tree := fallbackTestTree().WithNode("extra", Node{Leaf: true})
	if !slices.Equal(tree.FallbackScopes, []string{"dbo"}) {
		t.Fatalf("fallback scopes = %v", tree.FallbackScopes)
	}
}

func TestCompletionViewChildReturnsListingEntry(t *testing.T) {
	view := NewCompletionView(viewTestTree(), vSales, "", map[ListingKey][]Child{
		{Parent: vSales, Folder: "functions"}: {{Kind: "function", Name: "open_orders", Attributes: map[string]any{"returns_table": true}}},
	}, nil)
	child, ok := view.Child(ObjectRef{Scope: vSales, Kind: "function", Name: "open_orders"})
	if !ok || child.Attributes["returns_table"] != true {
		t.Fatalf("child = %+v ok %v", child, ok)
	}
	if _, ok := view.Child(ObjectRef{Scope: vSales, Kind: "function", Name: "missing"}); ok {
		t.Fatal("missing child reported present")
	}
	if _, ok := view.Child(ObjectRef{Scope: vSales, Kind: "table", Name: "open_orders"}); ok {
		t.Fatal("kind mismatch reported present")
	}
}
