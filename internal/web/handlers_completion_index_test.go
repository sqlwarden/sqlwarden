package web

import (
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
)

func completionIndexURL(org string, wsID, envID, connID int64) string {
	return orgConnectionURL(org, wsID, envID, strconv.FormatInt(connID, 10)) + "/schema/completion-index"
}

func completionIndexObjects(body map[string]any) []map[string]any {
	raw, _ := body["objects"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func completionIndexColumns(body map[string]any) []map[string]any {
	raw, _ := body["columns"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func hasIndexObject(objects []map[string]any, schema, name, kind string) bool {
	for _, o := range objects {
		if o["schema"] == schema && o["name"] == name && o["kind"] == kind {
			return true
		}
	}
	return false
}

func hasIndexColumn(columns []map[string]any, schema, table, name, dataType string, nullable bool) bool {
	for _, c := range columns {
		if c["schema"] == schema && c["table"] == table && c["name"] == name &&
			c["type"] == dataType && c["nullable"] == nullable {
			return true
		}
	}
	return false
}

func hasString(values any, want string) bool {
	list, _ := values.([]any)
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestCompletionIndexRejectsForeignSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-index-scope"), "Index", "Index Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Index WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	target := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Target", "open")
	other := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Other", "open")
	disableSchemaSnapshots(t, app, target.ID)
	session := openSchemaSession(t, app, owner.ID, other.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet, completionIndexURL(org.Slug, ws.ID, envID, target.ID), nil, token)
	req.Header.Set("X-Warden-Session", session.ID)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestProjectCompletionIndexFromView(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	public := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	users := metadata.ObjectRef{Scope: public, Kind: "table", Name: "users"}
	view := metadatatest.Build(metadatatest.Tree(true), metadatatest.Fixture{
		DefaultScope: public,
		Objects:      []metadata.Object{{Ref: users, Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "integer"}}}}},
	})
	out, err := projectCompletionIndex(view)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultSchema != "public" || len(out.Schemas) != 1 || out.Schemas[0] != "public" {
		t.Fatalf("schemas = %+v default %q", out.Schemas, out.DefaultSchema)
	}
	if len(out.Objects) != 1 || out.Objects[0] != (completionIndexObject{Schema: "public", Name: "users", Kind: "table", Score: completer.KindScore("table")}) {
		t.Fatalf("objects = %+v", out.Objects)
	}
	if len(out.Columns) != 1 || out.Columns[0].Name != "id" || out.Columns[0].Type != "integer" {
		t.Fatalf("columns = %+v", out.Columns)
	}
	again, _ := projectCompletionIndex(view)
	if out.Version == "" || out.Version != again.Version {
		t.Fatalf("version = %q / %q", out.Version, again.Version)
	}
}

func TestProjectCompletionIndexReportsDefaultScopeRelationCoverage(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	public := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	tree := metadatatest.Tree(true)

	listed := metadatatest.Build(tree, metadatatest.Fixture{DefaultScope: public})
	out, err := projectCompletionIndex(listed)
	if err != nil {
		t.Fatal(err)
	}
	if !out.DefaultScopeRelationsListed {
		t.Fatal("default scope with every folder listed must report relations listed")
	}
	if out.ColumnScore != completer.KindScore("column") {
		t.Fatalf("column score = %d", out.ColumnScore)
	}

	partial := metadata.NewCompletionView(tree, public, "", map[metadata.ListingKey][]metadata.Child{
		{Parent: public, Folder: "tables"}: {{Kind: "table", Name: "orders"}},
	}, nil)
	out, err = projectCompletionIndex(partial)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScopeRelationsListed {
		t.Fatal("unlisted view folders must not report relations listed")
	}

	noDefault := metadatatest.Build(tree, metadatatest.Fixture{Scopes: []metadata.ScopePath{public}})
	out, err = projectCompletionIndex(noDefault)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScopeRelationsListed {
		t.Fatal("no default scope must not report relations listed")
	}

	database := metadatatest.Build(tree, metadatatest.Fixture{DefaultScope: app})
	out, err = projectCompletionIndex(database)
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultScopeRelationsListed {
		t.Fatal("a default scope without relation folders must not report relations listed")
	}
}

func completionFallbackTree() metadata.Tree {
	tree := metadatatest.Tree(true)
	tree.FallbackScopes = []string{"dbo"}
	return tree
}

func relationFolderListings(scope metadata.ScopePath) map[metadata.ListingKey][]metadata.Child {
	return map[metadata.ListingKey][]metadata.Child{
		{Parent: scope, Folder: "tables"}:             {{Kind: "table", Name: "orders"}},
		{Parent: scope, Folder: "views"}:              {},
		{Parent: scope, Folder: "materialized_views"}: {},
		{Parent: scope, Folder: "foreign_tables"}:     {},
	}
}

func TestProjectCompletionIndexSearchSchemas(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	sales := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
	dbo := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	schemas := func(names ...string) []metadata.Child {
		out := []metadata.Child{}
		for _, name := range names {
			out = append(out, metadata.Child{Kind: "schema", Name: name})
		}
		return out
	}
	merge := func(parts ...map[metadata.ListingKey][]metadata.Child) map[metadata.ListingKey][]metadata.Child {
		out := map[metadata.ListingKey][]metadata.Child{}
		for _, part := range parts {
			for k, v := range part {
				out[k] = v
			}
		}
		return out
	}
	for _, tc := range []struct {
		name     string
		def      metadata.ScopePath
		listings map[metadata.ListingKey][]metadata.Child
		search   []string
		listed   bool
	}{
		{"default and dbo listed", sales, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales", "dbo")},
			relationFolderListings(sales), relationFolderListings(dbo),
		), []string{"sales", "dbo"}, true},
		{"dbo unlisted", sales, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales", "dbo")},
			relationFolderListings(sales),
		), []string{"sales", "dbo"}, false},
		{"dbo absent", sales, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales")},
			relationFolderListings(sales),
		), []string{"sales"}, true},
		{"database default searches dbo", app, merge(
			map[metadata.ListingKey][]metadata.Child{{Parent: app, Folder: "schemas"}: schemas("sales", "dbo")},
			relationFolderListings(dbo),
		), []string{"dbo"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := projectCompletionIndex(metadata.NewCompletionView(completionFallbackTree(), tc.def, "", tc.listings, nil))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(out.SearchSchemas, tc.search) || out.DefaultScopeRelationsListed != tc.listed {
				t.Fatalf("search = %q listed %v, want %q %v", out.SearchSchemas, out.DefaultScopeRelationsListed, tc.search, tc.listed)
			}
		})
	}
}

func TestProjectCompletionIndexSearchSchemasFlatTree(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	out, err := projectCompletionIndex(metadatatest.Build(metadatatest.Tree(false), metadatatest.Fixture{DefaultScope: app}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.SearchSchemas, []string{""}) || !out.DefaultScopeRelationsListed {
		t.Fatalf("search = %q listed %v", out.SearchSchemas, out.DefaultScopeRelationsListed)
	}
}

func TestProjectCompletionIndexSearchSchemasEmptyWithoutDefault(t *testing.T) {
	out, err := projectCompletionIndex(metadata.NewCompletionView(metadata.Tree{}, "", "", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if out.SearchSchemas == nil || len(out.SearchSchemas) != 0 {
		t.Fatalf("search = %#v, want empty non-nil", out.SearchSchemas)
	}
}
