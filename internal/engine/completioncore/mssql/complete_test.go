package mssql

import (
	"context"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/engine/completioncore"
	"github.com/sqlwarden/internal/engine/completioncore/completiontest"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

var (
	app   = metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	sales = app.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
	dbo   = app.Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
)

func testTree() metadata.Tree {
	columns := []metadata.Folder{{Kind: "columns", Child: "column"}}
	return metadata.Tree{
		FallbackScopes: []string{"dbo"},
		Root:           metadata.Node{Folders: []metadata.Folder{{Kind: "databases", Child: "database"}}},
		Nodes: map[string]metadata.Node{
			"database": {Scope: true, ShowAllDatabases: true, Folders: []metadata.Folder{{Kind: "schemas", Child: "schema"}}},
			"schema": {Scope: true, Folders: []metadata.Folder{
				{Kind: "tables", Child: "table"},
				{Kind: "external_tables", Child: "external_table"},
				{Kind: "views", Child: "view"},
				{Kind: "procedures", Child: "procedure", MixedKinds: []string{"function"}},
				{Kind: "synonyms", Child: "synonym"},
			}},
			"table":          {Relational: true, Folders: columns},
			"external_table": {Relational: true, Folders: columns},
			"view":           {Relational: true, Folders: columns},
			"column":         {Leaf: true, Column: true},
			"procedure":      {Leaf: true},
			"function":       {Leaf: true},
			"synonym":        {Leaf: true},
		},
	}
}

func key(parent metadata.ScopePath, folder string) metadata.ListingKey {
	return metadata.ListingKey{Parent: parent, Folder: folder}
}

func columnsKey(scope metadata.ScopePath, kind, name string) metadata.ListingKey {
	return key(metadata.ObjectRef{Scope: scope, Kind: kind, Name: name}.Path(), "columns")
}

func col(name, dataType string, ordinal int) metadata.Child {
	return metadata.Child{Kind: "column", Name: name, Attributes: map[string]any{"data_type": dataType, "ordinal": ordinal}}
}

func worldListings() map[metadata.ListingKey][]metadata.Child {
	return map[metadata.ListingKey][]metadata.Child{
		key("", "databases"): {{Kind: "database", Name: "app"}, {Kind: "database", Name: "other"}},
		key(app, "schemas"):  {{Kind: "schema", Name: "sales"}, {Kind: "schema", Name: "dbo"}},

		key(sales, "tables"):          {{Kind: "table", Name: "orders"}},
		key(sales, "external_tables"): {},
		key(sales, "views"):           {{Kind: "view", Name: "order_summary"}},
		key(sales, "procedures"): {
			{Kind: "procedure", Name: "place_order"},
			{Kind: "function", Name: "tax"},
			{Kind: "function", Name: "open_orders", Attributes: map[string]any{ReturnsTableAttribute: true}},
		},
		key(sales, "synonyms"):                     {{Kind: "synonym", Name: "ord", Attributes: map[string]any{"target": "[dbo].[orders]"}}},
		columnsKey(sales, "table", "orders"):       {col("id", "int", 1), col("total", "decimal", 2), col("customer_id", "int", 3)},
		columnsKey(sales, "view", "order_summary"): {col("id", "int", 1)},

		key(dbo, "tables"):          {{Kind: "table", Name: "orders"}, {Kind: "table", Name: "customers"}, {Kind: "table", Name: "order details"}},
		key(dbo, "external_tables"): {{Kind: "external_table", Name: "ext_sales"}},
		key(dbo, "views"):           {},
		key(dbo, "procedures"): {
			{Kind: "procedure", Name: "cleanup"},
			{Kind: "function", Name: "tax"},
			{Kind: "function", Name: "fmt"},
		},
		key(dbo, "synonyms"):                           {},
		columnsKey(dbo, "table", "orders"):             {col("legacy_id", "int", 1)},
		columnsKey(dbo, "table", "customers"):          {col("id", "int", 1), col("name", "nvarchar", 2)},
		columnsKey(dbo, "table", "order details"):      {col("line", "int", 1)},
		columnsKey(dbo, "external_table", "ext_sales"): {col("amount", "money", 1)},
	}
}

func resolverFor(listings map[metadata.ListingKey][]metadata.Child) *completioncore.SchemaResolver {
	return completioncore.NewSchemaResolver(metadata.NewCompletionView(testTree(), sales, "", listings, nil), "sales")
}

func run(t *testing.T, marked string, meta *completioncore.SchemaResolver) ([]completioncore.Candidate, string) {
	t.Helper()
	sql, cursor := completiontest.Caret(t, marked)
	out, cctx, err := Complete(context.Background(), sql, cursor, meta)
	if err != nil {
		t.Fatal(err)
	}
	return out, cctx.Position
}

func texts(out []completioncore.Candidate, typ completioncore.CandidateType) []string {
	var names []string
	for _, c := range out {
		if c.Type == typ {
			names = append(names, c.Text)
		}
	}
	slices.Sort(names)
	return names
}

func find(t *testing.T, out []completioncore.Candidate, typ completioncore.CandidateType, text string) completioncore.Candidate {
	t.Helper()
	for _, c := range out {
		if c.Type == typ && c.Text == text {
			return c
		}
	}
	t.Fatalf("no %s candidate %q in %+v", typ, text, out)
	return completioncore.Candidate{}
}

func TestCompleteUnqualifiedRelations(t *testing.T) {
	out, position := run(t, "SELECT * FROM |", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateTable); !slices.Equal(got, []string{"customers", "order details", "orders"}) {
		t.Fatalf("tables = %v", got)
	}
	if orders := find(t, out, completioncore.CandidateTable, "orders"); orders.Definition != "sales · table" {
		t.Fatalf("orders resolved from %q, want the default schema", orders.Definition)
	}
	find(t, out, completioncore.CandidateView, "order_summary")
	find(t, out, completioncore.CandidateForeignTable, "ext_sales")
	if syn := find(t, out, completioncore.CandidateSynonym, "ord"); syn.Definition != "[dbo].[orders]" {
		t.Fatalf("synonym = %+v", syn)
	}
	if got := texts(out, completioncore.CandidateFunction); !slices.Equal(got, []string{"open_orders"}) {
		t.Fatalf("functions = %v, want only the table-valued function", got)
	}
	if got := texts(out, completioncore.CandidateSchema); !slices.Equal(got, []string{"dbo", "sales"}) {
		t.Fatalf("schemas = %v", got)
	}
	if position != completioncore.PositionRelation {
		t.Fatalf("position = %q", position)
	}
}

func TestCompleteSchemaQualifiedRelations(t *testing.T) {
	out, _ := run(t, "SELECT * FROM dbo.|", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateTable); !slices.Equal(got, []string{"customers", "order details", "orders"}) {
		t.Fatalf("tables = %v", got)
	}
	if orders := find(t, out, completioncore.CandidateTable, "orders"); orders.Definition != "dbo · table" {
		t.Fatalf("orders = %+v", orders)
	}
	out, _ = run(t, "SELECT * FROM sales.|", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateTable); !slices.Equal(got, []string{"orders"}) {
		t.Fatalf("sales tables = %v", got)
	}
}

func TestCompleteDatabaseQualifiedRelations(t *testing.T) {
	out, _ := run(t, "SELECT * FROM app.dbo.|", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateTable); !slices.Equal(got, []string{"customers", "order details", "orders"}) {
		t.Fatalf("tables = %v", got)
	}
	out, _ = run(t, "SELECT * FROM app.|", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateSchema); !slices.Equal(got, []string{"dbo", "sales"}) {
		t.Fatalf("schemas of app = %v", got)
	}
}

func TestCompleteAliasColumns(t *testing.T) {
	out, position := run(t, "SELECT o.| FROM orders o", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"customer_id", "id", "total"}) {
		t.Fatalf("columns = %v", got)
	}
	if got := texts(out, completioncore.CandidateTable); len(got) != 0 {
		t.Fatalf("alias qualifier offered tables %v", got)
	}
	if position != completioncore.PositionColumn {
		t.Fatalf("position = %q", position)
	}
	out, _ = run(t, "SELECT c.| FROM dbo.customers c", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"id", "name"}) {
		t.Fatalf("customer columns = %v", got)
	}
}

func TestCompleteColumnsAndScalarFunctions(t *testing.T) {
	out, _ := run(t, "SELECT | FROM orders o", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"customer_id", "id", "total"}) {
		t.Fatalf("columns = %v", got)
	}
	if tax := find(t, out, completioncore.CandidateFunction, "tax"); tax.InsertText != "sales.tax" {
		t.Fatalf("tax = %+v, want the default schema's function", tax)
	}
	if fmt := find(t, out, completioncore.CandidateFunction, "fmt"); fmt.InsertText != "dbo.fmt" {
		t.Fatalf("fmt = %+v", fmt)
	}
	for _, c := range out {
		if c.Type == completioncore.CandidateFunction && c.Text == "open_orders" {
			t.Fatalf("table-valued function offered as an expression: %+v", c)
		}
	}
}

func TestCompleteProcedures(t *testing.T) {
	out, _ := run(t, "EXEC |", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateProcedure); !slices.Equal(got, []string{"cleanup", "place_order"}) {
		t.Fatalf("procedures = %v", got)
	}
	out, _ = run(t, "EXEC dbo.|", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateProcedure); !slices.Equal(got, []string{"cleanup"}) {
		t.Fatalf("dbo procedures = %v", got)
	}
}

func TestCompleteCTEAndDerivedColumns(t *testing.T) {
	out, _ := run(t, "WITH c AS (SELECT id, total AS amount FROM orders) SELECT c.| FROM c", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"amount", "id"}) {
		t.Fatalf("cte columns = %v", got)
	}
	out, _ = run(t, "WITH c AS (SELECT 1 AS a) SELECT * FROM |", resolverFor(worldListings()))
	if cte := find(t, out, completioncore.CandidateTable, "c"); cte.Definition != "Common table expression" {
		t.Fatalf("cte = %+v", cte)
	}
	out, _ = run(t, "SELECT d.| FROM (SELECT id AS x, total FROM orders) d", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"total", "x"}) {
		t.Fatalf("derived columns = %v", got)
	}
	out, _ = run(t, "SELECT d.| FROM (SELECT id FROM orders) AS d(b)", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("aliased derived columns = %v", got)
	}
}

func TestCompleteUpdateTargetColumns(t *testing.T) {
	out, _ := run(t, "UPDATE orders SET |", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"customer_id", "id", "total"}) {
		t.Fatalf("update columns = %v", got)
	}
}

func TestCompleteUpdateAliasTarget(t *testing.T) {
	meta := resolverFor(worldListings())
	out, _ := run(t, "UPDATE o SET | FROM orders o", meta)
	if got := texts(out, completioncore.CandidateColumn); !slices.Equal(got, []string{"customer_id", "id", "total"}) {
		t.Fatalf("update alias columns = %v", got)
	}
	if got := meta.Demands(); len(got) != 0 {
		t.Fatalf("alias target looked up as a relation: demands %v", got)
	}
}

func TestCompleteSuppressedInCommentsAndStrings(t *testing.T) {
	for _, marked := range []string{"SELECT 1 -- FROM |", "SELECT '|", "SELECT /* ord| */ 1"} {
		if out, _ := run(t, marked, resolverFor(worldListings())); len(out) != 0 {
			t.Fatalf("%q: candidates %+v", marked, out)
		}
	}
}

func TestCompleteBracketPrefix(t *testing.T) {
	out, _ := run(t, "SELECT * FROM [ord|", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateTable); !slices.Equal(got, []string{"order details", "orders"}) {
		t.Fatalf("tables = %v", got)
	}
	find(t, out, completioncore.CandidateView, "order_summary")
	find(t, out, completioncore.CandidateSynonym, "ord")
	for _, c := range out {
		if c.Type == completioncore.CandidateTable && c.Text == "customers" {
			t.Fatal("prefix filter not applied")
		}
	}
}

func TestCompleteHiddenDatabaseYieldsNothing(t *testing.T) {
	listings := worldListings()
	listings[key("", "databases")] = []metadata.Child{{Kind: "database", Name: "app"}}
	meta := resolverFor(listings)
	out, _ := run(t, "SELECT * FROM other.dbo.|", meta)
	if got := texts(out, completioncore.CandidateTable); len(got) != 0 {
		t.Fatalf("tables = %v", got)
	}
	if got := meta.Demands(); len(got) != 0 {
		t.Fatalf("demands = %v", got)
	}
}

func TestCompleteUnlistedWorldRecordsSearchScopeDemands(t *testing.T) {
	meta := resolverFor(map[metadata.ListingKey][]metadata.Child{
		key(app, "schemas"): {{Kind: "schema", Name: "sales"}, {Kind: "schema", Name: "dbo"}},
	})
	run(t, "SELECT * FROM |", meta)
	want := map[metadata.Demand]bool{}
	for _, scope := range []metadata.ScopePath{sales, dbo} {
		for _, folder := range []string{"tables", "external_tables", "views", "procedures", "synonyms"} {
			want[metadata.Demand{Parent: scope, Folder: folder}] = true
		}
	}
	got := meta.Demands()
	for _, demand := range got {
		delete(want, demand)
	}
	if len(want) != 0 {
		t.Fatalf("missing demands %v in %v", want, got)
	}
}

func TestCompleteDatabases(t *testing.T) {
	out, _ := run(t, "USE |", resolverFor(worldListings()))
	if got := texts(out, completioncore.CandidateDatabase); !slices.Equal(got, []string{"app", "other"}) {
		t.Fatalf("databases = %v", got)
	}
}

func TestCompleteWithoutMetadataIsKeywordOnly(t *testing.T) {
	out, _ := run(t, "SELE|", nil)
	find(t, out, completioncore.CandidateKeyword, "SELECT")
	for _, c := range out {
		if c.Type != completioncore.CandidateKeyword && c.Type != completioncore.CandidateTypeName && c.Type != completioncore.CandidateFunction {
			t.Fatalf("object candidate without metadata: %+v", c)
		}
	}
}
