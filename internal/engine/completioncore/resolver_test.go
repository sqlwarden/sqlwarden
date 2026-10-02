package completioncore

import (
	"slices"
	"testing"

	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
)

var (
	rApp    = metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	rPublic = rApp.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	rSales  = rApp.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
)

func relationDemands(scope metadata.ScopePath) []metadata.Demand {
	return []metadata.Demand{
		{Parent: scope, Folder: "tables"}, {Parent: scope, Folder: "views"},
		{Parent: scope, Folder: "materialized_views"}, {Parent: scope, Folder: "foreign_tables"},
	}
}

func partialView(listings map[metadata.ListingKey][]metadata.Child) *metadata.CompletionView {
	return metadata.NewCompletionView(metadatatest.Tree(true), rPublic, "", listings, nil)
}

func TestSchemaResolverRecordsRelationFolderDemands(t *testing.T) {
	r := NewSchemaResolver(partialView(map[metadata.ListingKey][]metadata.Child{
		{Parent: rApp, Folder: "schemas"}: {{Kind: "schema", Name: "public"}},
	}), "public")
	if got := r.Relations("", ""); len(got) != 0 {
		t.Fatalf("relations = %+v", got)
	}
	if got := r.Demands(); !slices.Equal(got, relationDemands(rPublic)) {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverRecordsColumnDemandForReferencedRelation(t *testing.T) {
	r := NewSchemaResolver(partialView(map[metadata.ListingKey][]metadata.Child{
		{Parent: rPublic, Folder: "tables"}:             {{Kind: "table", Name: "orders"}},
		{Parent: rPublic, Folder: "views"}:              {},
		{Parent: rPublic, Folder: "materialized_views"}: {},
		{Parent: rPublic, Folder: "foreign_tables"}:     {},
	}), "public")
	relation, ok := r.FindRelation("", "", "orders")
	if !ok || relation.Name != "orders" || len(relation.Columns) != 0 {
		t.Fatalf("relation = %+v %v", relation, ok)
	}
	orders := metadata.ObjectRef{Scope: rPublic, Kind: "table", Name: "orders"}
	if got := r.Demands(); !slices.Equal(got, []metadata.Demand{{Parent: orders.Path(), Folder: "columns"}}) {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverCascadesUnlistedSchema(t *testing.T) {
	r := NewSchemaResolver(partialView(nil), "public")
	_ = r.Relations("", "sales")
	want := append([]metadata.Demand{{Parent: rApp, Folder: "schemas"}}, relationDemands(rSales)...)
	if got := r.Demands(); !slices.Equal(got, want) {
		t.Fatalf("demands = %v, want %v", got, want)
	}
}

func TestSchemaResolverMatchesListedScopeCaseInsensitively(t *testing.T) {
	r := NewSchemaResolver(partialView(map[metadata.ListingKey][]metadata.Child{
		{Parent: rApp, Folder: "schemas"}:  {{Kind: "schema", Name: "sales"}},
		{Parent: rSales, Folder: "tables"}: {{Kind: "table", Name: "orders"}},
	}), "public")
	got := r.Relations("", "Sales")
	if len(got) != 1 || got[0].Name != "orders" || got[0].Schema != "sales" {
		t.Fatalf("relations = %+v", got)
	}
	for _, demand := range r.Demands() {
		if demand.Parent.Name("schema") == "Sales" {
			t.Fatalf("speculative scope used although schemas are listed: %v", demand)
		}
	}
}

func TestSchemaResolverDemandsDatabaseNames(t *testing.T) {
	r := NewSchemaResolver(partialView(nil), "public")
	if got := r.DatabaseNames(); len(got) != 0 {
		t.Fatalf("names = %v", got)
	}
	if got := r.Demands(); !slices.Equal(got, []metadata.Demand{{Parent: "", Folder: "databases"}}) {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverOrdersDemandsByPriorityAndDeduplicates(t *testing.T) {
	r := NewSchemaResolver(partialView(map[metadata.ListingKey][]metadata.Child{
		{Parent: rApp, Folder: "schemas"}: {{Kind: "schema", Name: "public"}, {Kind: "schema", Name: "sales"}},
	}), "public")
	_ = r.Relations("", "")
	_ = r.Relations("", "sales")
	_ = r.Relations("", "")
	want := append(relationDemands(rSales), relationDemands(rPublic)...)
	if got := r.Demands(); !slices.Equal(got, want) {
		t.Fatalf("demands = %v, want %v", got, want)
	}

	upgraded := NewSchemaResolver(partialView(map[metadata.ListingKey][]metadata.Child{
		{Parent: rApp, Folder: "schemas"}: {{Kind: "schema", Name: "public"}, {Kind: "schema", Name: "sales"}},
	}), "public")
	_ = upgraded.Relations("", "")
	_ = upgraded.Relations("", "sales")
	_ = upgraded.Relations("app", "public")
	want = append(relationDemands(rPublic), relationDemands(rSales)...)
	if got := upgraded.Demands(); !slices.Equal(got, want) {
		t.Fatalf("upgraded demands = %v, want %v", got, want)
	}
}

func TestSchemaResolverFullyListedFixtureRecordsNothing(t *testing.T) {
	orders := metadata.ObjectRef{Scope: rPublic, Kind: "table", Name: "orders"}
	view := metadatatest.Build(metadatatest.Tree(true), metadatatest.Fixture{
		DefaultScope: rPublic,
		Objects:      []metadata.Object{{Ref: orders, Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id"}}}}},
	})
	r := NewSchemaResolver(view, "public")
	if got := r.Relations("", ""); len(got) != 1 || len(got[0].Columns) != 1 {
		t.Fatalf("relations = %+v", got)
	}
	if _, ok := r.FindRelation("", "public", "ORDERS"); !ok {
		t.Fatal("case-insensitive relation lookup failed")
	}
	if got := r.Demands(); len(got) != 0 {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverDemandsRootScopesWithoutDefault(t *testing.T) {
	view := metadata.NewCompletionView(metadatatest.Tree(true), "", "", nil, nil)
	r := NewSchemaResolver(view, "")
	if _, ok := r.FindRelation("", "", "orders"); ok {
		t.Fatal("relation found in an empty view")
	}
	if got := r.Demands(); !slices.Equal(got, []metadata.Demand{{Parent: "", Folder: "databases"}}) {
		t.Fatalf("demands = %v", got)
	}

	listed := metadata.NewCompletionView(metadatatest.Tree(true), "", "", map[metadata.ListingKey][]metadata.Child{
		{Parent: "", Folder: "databases"}: {{Kind: "database", Name: "app"}, {Kind: "database", Name: "other"}},
	}, nil)
	r = NewSchemaResolver(listed, "")
	_, _ = r.FindRelation("", "", "orders")
	if got := r.Demands(); len(got) != 0 {
		t.Fatalf("listed root without a current database: demands = %v", got)
	}
}

func TestSchemaResolverDemandsSchemasOfDatabaseOnlyDefault(t *testing.T) {
	view := metadata.NewCompletionView(metadatatest.Tree(true), rApp, "", nil, nil)
	r := NewSchemaResolver(view, "")
	if _, ok := r.FindRelation("", "", "orders"); ok {
		t.Fatal("relation found in an empty view")
	}
	if got := r.Demands(); !slices.Equal(got, []metadata.Demand{{Parent: rApp, Folder: "schemas"}}) {
		t.Fatalf("demands = %v", got)
	}

	listed := metadata.NewCompletionView(metadatatest.Tree(true), rApp, "", map[metadata.ListingKey][]metadata.Child{
		{Parent: rApp, Folder: "schemas"}: {{Kind: "schema", Name: "public"}},
	}, nil)
	r = NewSchemaResolver(listed, "")
	_, _ = r.FindRelation("", "", "orders")
	if got := r.Demands(); len(got) != 0 {
		t.Fatalf("listed schemas without a current schema: demands = %v", got)
	}
}

func TestSchemaResolverFollowsCurrentScopesWithoutDefault(t *testing.T) {
	view := metadata.NewCompletionView(metadatatest.Tree(true), "", rPublic, map[metadata.ListingKey][]metadata.Child{
		{Parent: "", Folder: "databases"}: {{Kind: "database", Name: "app", Current: true}, {Kind: "database", Name: "other"}},
		{Parent: rApp, Folder: "schemas"}: {{Kind: "schema", Name: "public", Current: true}},
	}, nil)
	r := NewSchemaResolver(view, "public")
	_, _ = r.FindRelation("", "", "orders")
	if got := r.Demands(); !slices.Equal(got, relationDemands(rPublic)) {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverResolvesSchemaQualifierUnderCurrentDatabase(t *testing.T) {
	alpha := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "alpha"})
	beta := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "beta"})
	alphaPublic := alpha.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	betaPublic := beta.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	listings := map[metadata.ListingKey][]metadata.Child{
		{Parent: "", Folder: "databases"}:       {{Kind: "database", Name: "alpha"}, {Kind: "database", Name: "beta", Current: true}},
		{Parent: alpha, Folder: "schemas"}:      {{Kind: "schema", Name: "public"}},
		{Parent: beta, Folder: "schemas"}:       {{Kind: "schema", Name: "public", Current: true}},
		{Parent: alphaPublic, Folder: "tables"}: {{Kind: "table", Name: "alpha_only"}},
		{Parent: betaPublic, Folder: "tables"}:  {{Kind: "table", Name: "beta_only"}},
	}
	for _, scope := range []metadata.ScopePath{alphaPublic, betaPublic} {
		for _, folder := range []string{"views", "materialized_views", "foreign_tables"} {
			listings[metadata.ListingKey{Parent: scope, Folder: folder}] = []metadata.Child{}
		}
	}
	view := metadata.NewCompletionView(metadatatest.Tree(true), "", betaPublic, listings, nil)
	if got := view.DefaultScope(); got != betaPublic {
		t.Fatalf("default scope = %q, want %q", got, betaPublic)
	}

	for _, schema := range []string{"", "public"} {
		r := NewSchemaResolver(view, "public")
		got := r.Relations("", schema)
		if len(got) != 1 || got[0].Name != "beta_only" || got[0].Database != "beta" {
			t.Fatalf("relations(%q) = %+v, want beta_only in beta", schema, got)
		}
		if _, ok := r.FindRelation("", schema, "alpha_only"); ok {
			t.Fatalf("FindRelation(%q) resolved a relation from the non-current database", schema)
		}
		if relation, ok := r.FindRelation("", schema, "beta_only"); !ok || relation.Database != "beta" {
			t.Fatalf("FindRelation(%q) = %+v %v", schema, relation, ok)
		}
		betaOnly := metadata.ObjectRef{Scope: betaPublic, Kind: "table", Name: "beta_only"}
		if got := r.Demands(); !slices.Equal(got, []metadata.Demand{{Parent: betaOnly.Path(), Folder: "columns"}}) {
			t.Fatalf("demands(%q) = %v", schema, got)
		}
	}
}

func TestSchemaResolverRanksDefaultScopeListingBelowExplicitQualifiers(t *testing.T) {
	view := metadata.NewCompletionView(metadatatest.Tree(true), rApp, "", nil, nil)
	r := NewSchemaResolver(view, "")
	_ = r.Relations("app", "")
	_ = r.Relations("other", "")
	want := []metadata.Demand{{Parent: "", Folder: "databases"}, {Parent: rApp, Folder: "schemas"}}
	if got := r.Demands(); !slices.Equal(got, want) {
		t.Fatalf("demands = %v, want %v", got, want)
	}
}

var rDbo = rApp.Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})

func fallbackView(listings map[metadata.ListingKey][]metadata.Child) *metadata.CompletionView {
	tree := metadatatest.Tree(true)
	tree.FallbackScopes = []string{"dbo"}
	return metadata.NewCompletionView(tree, rSales, "", listings, nil)
}

func listings(parts ...map[metadata.ListingKey][]metadata.Child) map[metadata.ListingKey][]metadata.Child {
	out := map[metadata.ListingKey][]metadata.Child{}
	for _, part := range parts {
		for key, items := range part {
			out[key] = items
		}
	}
	return out
}

func schemasListing(names ...string) map[metadata.ListingKey][]metadata.Child {
	items := []metadata.Child{}
	for _, name := range names {
		items = append(items, metadata.Child{Kind: "schema", Name: name})
	}
	return map[metadata.ListingKey][]metadata.Child{{Parent: rApp, Folder: "schemas"}: items}
}

func relationsListing(scope metadata.ScopePath, tables ...string) map[metadata.ListingKey][]metadata.Child {
	items := []metadata.Child{}
	for _, name := range tables {
		items = append(items, metadata.Child{Kind: "table", Name: name})
	}
	return map[metadata.ListingKey][]metadata.Child{
		{Parent: scope, Folder: "tables"}:             items,
		{Parent: scope, Folder: "views"}:              {},
		{Parent: scope, Folder: "materialized_views"}: {},
		{Parent: scope, Folder: "foreign_tables"}:     {},
	}
}

func qualifiedNames(relations []Relation) []string {
	out := make([]string, 0, len(relations))
	for _, relation := range relations {
		out = append(out, relation.Schema+"."+relation.Name)
	}
	return out
}

func TestSchemaResolverSearchScopesShadowLaterScopes(t *testing.T) {
	r := NewSchemaResolver(fallbackView(listings(
		schemasListing("sales", "dbo"),
		relationsListing(rSales, "orders"),
		relationsListing(rDbo, "Orders", "customers"),
	)), "sales")
	if got := qualifiedNames(r.Relations("", "")); !slices.Equal(got, []string{"sales.orders", "dbo.customers"}) {
		t.Fatalf("relations = %v", got)
	}
	if got := r.Demands(); len(got) != 0 {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverDemandsUnlistedFallbackScope(t *testing.T) {
	r := NewSchemaResolver(fallbackView(listings(
		schemasListing("sales", "dbo"),
		relationsListing(rSales, "orders"),
	)), "sales")
	if got := qualifiedNames(r.Relations("", "")); !slices.Equal(got, []string{"sales.orders"}) {
		t.Fatalf("relations = %v", got)
	}
	if got := r.Demands(); !slices.Equal(got, relationDemands(rDbo)) {
		t.Fatalf("demands = %v, want %v", got, relationDemands(rDbo))
	}
}

func TestSchemaResolverSkipsAbsentFallbackScope(t *testing.T) {
	r := NewSchemaResolver(fallbackView(listings(
		schemasListing("sales"),
		relationsListing(rSales, "orders"),
	)), "sales")
	if got := qualifiedNames(r.Relations("", "")); !slices.Equal(got, []string{"sales.orders"}) {
		t.Fatalf("relations = %v", got)
	}
	if got := r.Demands(); len(got) != 0 {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverFindRelationWaitsForEarlierSearchScope(t *testing.T) {
	r := NewSchemaResolver(fallbackView(listings(
		schemasListing("sales", "dbo"),
		relationsListing(rDbo, "orders"),
	)), "sales")
	if relation, ok := r.FindRelation("", "", "orders"); ok {
		t.Fatalf("resolved %+v although sales may shadow it", relation)
	}
	if got := r.Demands(); !slices.Equal(got, relationDemands(rSales)) {
		t.Fatalf("demands = %v, want %v", got, relationDemands(rSales))
	}
}

func TestSchemaResolverFindRelationFallsBackToDbo(t *testing.T) {
	customers := metadata.ObjectRef{Scope: rDbo, Kind: "table", Name: "customers"}
	r := NewSchemaResolver(fallbackView(listings(
		schemasListing("sales", "dbo"),
		relationsListing(rSales, "orders"),
		relationsListing(rDbo, "customers"),
		map[metadata.ListingKey][]metadata.Child{{Parent: customers.Path(), Folder: "columns"}: {{Kind: "column", Name: "id"}}},
	)), "sales")
	relation, ok := r.FindRelation("", "", "customers")
	if !ok || relation.Schema != "dbo" || len(relation.Columns) != 1 {
		t.Fatalf("relation = %+v ok %v", relation, ok)
	}
	if got := r.Demands(); len(got) != 0 {
		t.Fatalf("demands = %v", got)
	}
}

func TestSchemaResolverCatalogObjectsShadowAcrossSearchScopes(t *testing.T) {
	r := NewSchemaResolver(fallbackView(listings(
		schemasListing("sales", "dbo"),
		map[metadata.ListingKey][]metadata.Child{
			{Parent: rSales, Folder: "functions"}: {{Kind: "function", Name: "tax"}},
			{Parent: rDbo, Folder: "functions"}:   {{Kind: "function", Name: "tax"}, {Kind: "function", Name: "fmt"}},
		},
	)), "sales")
	got := r.CatalogObjects("", "", "function")
	want := []metadata.ObjectRef{
		{Scope: rSales, Kind: "function", Name: "tax"},
		{Scope: rDbo, Kind: "function", Name: "fmt"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("objects = %v, want %v", got, want)
	}
}

func TestSchemaResolverChildExposesListingAttributes(t *testing.T) {
	r := NewSchemaResolver(fallbackView(map[metadata.ListingKey][]metadata.Child{
		{Parent: rSales, Folder: "functions"}: {{Kind: "function", Name: "open", Attributes: map[string]any{"returns_table": true}}},
	}), "sales")
	child, ok := r.Child(metadata.ObjectRef{Scope: rSales, Kind: "function", Name: "open"})
	if !ok || child.Attributes["returns_table"] != true {
		t.Fatalf("child = %+v ok %v", child, ok)
	}
}

func TestRelationFromObjectMapsExternalTable(t *testing.T) {
	relation, ok := relationFromObject(metadata.Object{Ref: metadata.ObjectRef{Scope: rSales, Kind: "external_table", Name: "ext"}})
	if !ok || relation.Kind != CandidateForeignTable || relation.Schema != "sales" {
		t.Fatalf("relation = %+v ok %v", relation, ok)
	}
}
