package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

const (
	navTestEngine  = "navtest"
	navFlatEngine  = "navflat"
	navPlainEngine = "navplain"
)

func init() {
	engine.Register(engine.Registration{
		ID: navTestEngine, DisplayName: "Navigator Test", Dialect: engine.DialectSQLite,
		New: func() engine.Driver {
			d := newNavTestDriver()
			d.q.set("", "databases", metadata.Child{Kind: "database", Name: "app", Current: true}, metadata.Child{Kind: "database", Name: "reports"})
			return d
		},
	})
	engine.Register(engine.Registration{
		ID: navFlatEngine, DisplayName: "Navigator Flat Test", Dialect: engine.DialectSQLite,
		New: func() engine.Driver { return navFlatDriver{newNavTestDriver()} },
	})
	engine.Register(engine.Registration{
		ID: navPlainEngine, DisplayName: "Navigator Plain Test", Dialect: engine.DialectSQLite,
		New: func() engine.Driver { return navPlainDriver{} },
	})
}

// navFlatDriver has a navigator without a database level.
type navFlatDriver struct{ *navTestDriver }

func (navFlatDriver) Tree() metadata.Tree {
	return metadata.Tree{
		Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
			{Kind: "tables", Label: "Tables", Child: "table", List: navTestLoader("tables")},
		}},
		Nodes: map[string]metadata.Node{"table": {Label: "Table", Icon: "table", Leaf: true}},
	}
}

// navPlainDriver has no navigator.
type navPlainDriver struct{}

func (navPlainDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (navPlainDriver) Ping(context.Context) error                             { return nil }
func (navPlainDriver) Close() error                                           { return nil }
func (navPlainDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (navPlainDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (navPlainDriver) Dialect() engine.Dialect { return engine.DialectSQLite }

// navTestQuerier carries per-session catalog state so parallel tests sharing
// the registered engine's static loaders stay isolated.
type navTestQuerier struct {
	mu       sync.Mutex
	children map[string][]metadata.Child
	calls    int
}

func (*navTestQuerier) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("navtest: no sql")
}
func (*navTestQuerier) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func (q *navTestQuerier) set(parent metadata.ScopePath, folder string, kids ...metadata.Child) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.children[string(parent)+"#"+folder] = kids
}

func navTestLoader(folder string) metadata.Loader {
	return func(_ context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
		fake := q.(*navTestQuerier)
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.calls++
		out := map[metadata.ScopePath][]metadata.Child{}
		for _, p := range parents {
			if kids, ok := fake.children[string(p)+"#"+folder]; ok {
				out[p] = kids
			}
		}
		return out, nil
	}
}

type navTestDriver struct{ q *navTestQuerier }

func newNavTestDriver() *navTestDriver {
	q := &navTestQuerier{children: map[string][]metadata.Child{}}
	q.set("", "databases", metadata.Child{Kind: "database", Name: "app"}, metadata.Child{Kind: "database", Name: "reports"})
	q.set(navDB("app"), "schemas", metadata.Child{Kind: "schema", Name: "public", Current: true})
	return &navTestDriver{q: q}
}

func (*navTestDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (*navTestDriver) Ping(context.Context) error                             { return nil }
func (*navTestDriver) Close() error                                           { return nil }
func (*navTestDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (*navTestDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (*navTestDriver) Dialect() engine.Dialect { return engine.DialectSQLite }

func (*navTestDriver) Tree() metadata.Tree {
	return metadata.Tree{
		SystemObjects: true,
		Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
			{Kind: "databases", Label: "Databases", Child: "database", List: navTestLoader("databases")},
		}},
		Nodes: map[string]metadata.Node{
			"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, Folders: []metadata.Folder{
				{Kind: "schemas", Label: "Schemas", Child: "schema", List: navTestLoader("schemas")},
			}},
			"schema": {Label: "Schema", Icon: "schema", Scope: true, Folders: []metadata.Folder{
				{Kind: "tables", Label: "Tables", Child: "table", List: navTestLoader("tables")},
			}},
			"table": {Label: "Table", Icon: "table", Relational: true, SupportsDiagram: true, Folders: []metadata.Folder{
				{Kind: "columns", Label: "Columns", Child: "column", List: navTestLoader("columns")},
			}},
			"column": {Label: "Column", Icon: "column", Leaf: true, Column: true},
		},
	}
}

func (d *navTestDriver) Querier(context.Context, string) (metadata.Querier, error) {
	if d.q == nil {
		return nil, errors.New("navtest: not connected")
	}
	return d.q, nil
}

func (*navTestDriver) InspectObjects(_ context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	out := make([]metadata.Object, 0, len(refs))
	for _, ref := range refs {
		out = append(out, metadata.Object{Ref: ref, Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "integer", Ordinal: 1}}}})
	}
	return out, nil
}

// Complete suggests the tables of app.public from the cached view and demands
// the listing when it has not been loaded.
func (*navTestDriver) Complete(_ context.Context, req completer.Request) (completer.Result, error) {
	if req.Metadata == nil {
		return completer.Result{}, nil
	}
	scope := navSchema("app", "public")
	refs, loaded := req.Metadata.Objects(scope, "table")
	result := completer.Result{}
	for _, ref := range refs {
		result.Suggestions = append(result.Suggestions, completer.Suggestion{Label: ref.Name, Kind: ref.Kind, InsertText: ref.Name})
	}
	if !loaded {
		result.Demands = req.Metadata.ObjectDemands(scope, "table")
	}
	return result, nil
}

func (q *navTestQuerier) loadCalls() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.calls
}

func navDB(name string) metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name})
}

type navFixture struct {
	app   *application
	tok   string
	base  string
	conn  database.Connection
	acct  database.Account
	orgID int64
	wsID  int64
}

func newNavFixture(t *testing.T, driver string) navFixture {
	t.Helper()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "nav"), "Nav Owner", "Nav Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Nav WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, driver, "Nav Conn", "open")
	return navFixture{app: app, tok: tok, conn: conn, acct: owner, orgID: org.ID, wsID: ws.ID, base: orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))}
}

func (f navFixture) attach(t *testing.T, drv engine.Driver) string {
	t.Helper()
	return f.attachTo(t, f.conn.ID, drv)
}

func (f navFixture) attachTo(t *testing.T, connID int64, drv engine.Driver) string {
	t.Helper()
	meta := connection.SessionMetadata{OrgID: strconv.FormatInt(f.orgID, 10), WorkspaceID: strconv.FormatInt(f.wsID, 10)}
	sess, _, err := testConnManager(t, f.app).GetOrCreateWithMetadata(strconv.FormatInt(f.acct.ID, 10), strconv.FormatInt(connID, 10), meta,
		func() (engine.Driver, func(), error) { return drv, nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	return sess.ID
}

func (f navFixture) nodes(t *testing.T, sessionID string, path metadata.ScopePath, folder string) testResponse {
	t.Helper()
	target := f.base + "/schema/nodes?folder=" + url.QueryEscape(folder)
	if path != "" {
		target += "&path=" + schemaScopeParam(path)
	}
	req := newAuthRequest(t, http.MethodGet, target, nil, f.tok)
	if sessionID != "" {
		req.Header.Set("X-Warden-Session", sessionID)
	}
	return send(t, req, f.app.routes())
}

func itemNamesOf(t *testing.T, res testResponse) []string {
	t.Helper()
	items, ok := res.BodyFields["items"].([]any)
	if !ok {
		t.Fatalf("items missing: %s", res.BodyBytes)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.(map[string]any)["name"].(string))
	}
	return names
}

func TestSchemaTreeServesGrammarWithoutSession(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	res := send(t, newAuthRequest(t, http.MethodGet, f.base+"/schema/tree", nil, f.tok), f.app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	root := res.BodyFields["root"].(map[string]any)
	folders := root["folders"].([]any)
	assert.Equal(t, folders[0].(map[string]any)["kind"], "databases")
	nodes := res.BodyFields["nodes"].(map[string]any)
	assert.Equal(t, nodes["table"].(map[string]any)["supports_diagram"], true)
	_, hasEditor := res.BodyFields["editor"]
	assert.Equal(t, hasEditor, true)
}

func TestSchemaTreeUnsupportedDriver(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navPlainEngine)
	res := send(t, newAuthRequest(t, http.MethodGet, f.base+"/schema/tree", nil, f.tok), f.app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
}

func TestSchemaNodesWithoutSessionNeverConnects(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	res := f.nodes(t, "", "", "databases")
	assert.Equal(t, res.StatusCode, http.StatusConflict)
	assertAPIError(t, res, "session_required", "Connect to this database to load schema objects.")

	expired := f.nodes(t, "expired-session-id", "", "databases")
	assert.Equal(t, expired.StatusCode, http.StatusGone)
	assert.Equal(t, testConnManager(t, f.app).CountForConnection(strconv.FormatInt(f.conn.ID, 10)), 0)
}

func TestSchemaNodesLiveThenMemoryThenStore(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	sessionID := f.attach(t, newNavTestDriver())

	live := f.nodes(t, sessionID, "", "databases")
	assert.Equal(t, live.StatusCode, http.StatusOK)
	assert.Equal(t, live.BodyFields["source"], "live")
	assert.Equal(t, live.BodyFields["folder"], "databases")
	first := live.BodyFields["items"].([]any)[0].(map[string]any)
	assert.Equal(t, first["path"].([]any)[0].(map[string]any)["name"], "app")

	memory := f.nodes(t, "", "", "databases")
	assert.Equal(t, memory.BodyFields["source"], "memory")

	f.app.schemaNavigator.ForgetConnection(f.conn.ID)
	stored := f.nodes(t, "", "", "databases")
	assert.Equal(t, stored.BodyFields["source"], "store")
	assert.Equal(t, len(itemNamesOf(t, stored)), 2)
}

func TestSchemaNodesEphemeralForgetsOnLastSessionClose(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	disableSchemaSnapshots(t, f.app, f.conn.ID)
	sessionID := f.attach(t, newNavTestDriver())
	assert.Equal(t, f.nodes(t, sessionID, "", "databases").StatusCode, http.StatusOK)
	testConnManager(t, f.app).Remove(sessionID)
	assert.Equal(t, f.nodes(t, "", "", "databases").StatusCode, http.StatusConflict)
}

func TestSchemaNodesValidation(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	sessionID := f.attach(t, newNavTestDriver())
	assert.Equal(t, f.nodes(t, sessionID, "", "").StatusCode, http.StatusBadRequest)
	assert.Equal(t, f.nodes(t, sessionID, "", "tables").StatusCode, http.StatusBadRequest)
	bad := newAuthRequest(t, http.MethodGet, f.base+"/schema/nodes?folder=schemas&path=%7Bnot-json", nil, f.tok)
	bad.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, bad, f.app.routes()).StatusCode, http.StatusBadRequest)
}

func TestSchemaNodesAppliesShowAllDatabases(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	conn := f.conn
	if err := f.app.db.UpdateConnectionWithScopeAndPolicy(context.Background(), conn.ID, conn.Name, conn.DSNEncrypted, conn.AccessMode, conn.SchemaSnapshotPolicy, navDB("reports"), conn.ShowSystemSchemas, false); err != nil {
		t.Fatal(err)
	}
	sessionID := f.attach(t, newNavTestDriver())
	res := f.nodes(t, sessionID, "", "databases")
	assert.Equal(t, res.StatusCode, http.StatusOK)
	names := itemNamesOf(t, res)
	assert.Equal(t, len(names), 1)
	assert.Equal(t, names[0], "reports")
	assert.Equal(t, res.BodyFields["items"].([]any)[0].(map[string]any)["current"], true)
}

func TestSchemaRefreshRequiresSession(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	req := newAuthRequest(t, http.MethodPost, f.base+"/schema/refresh", map[string]any{"path": []any{}}, f.tok)
	res := send(t, req, f.app.routes())
	assert.Equal(t, res.StatusCode, http.StatusConflict)
	assertAPIError(t, res, "session_required", "Connect to this database to load schema objects.")
}

func TestSchemaRefreshRelistsCachedSubtree(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	drv := newNavTestDriver()
	sessionID := f.attach(t, drv)
	assert.Equal(t, f.nodes(t, sessionID, "", "databases").StatusCode, http.StatusOK)
	assert.Equal(t, f.nodes(t, sessionID, navDB("app"), "schemas").StatusCode, http.StatusOK)
	drv.q.set(navDB("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"}, metadata.Child{Kind: "schema", Name: "sales"})

	req := newAuthRequest(t, http.MethodPost, f.base+"/schema/refresh", map[string]any{
		"path": []any{map[string]any{"kind": "database", "name": "app"}},
	}, f.tok)
	req.Header.Set("X-Warden-Session", sessionID)
	res := send(t, req, f.app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, len(res.BodyFields["items"].([]any)), 2)

	after := f.nodes(t, "", navDB("app"), "schemas")
	assert.Equal(t, len(itemNamesOf(t, after)), 2)
}

func TestSchemaNodesRejectsSessionForAnotherConnection(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	other := f.attachTo(t, f.conn.ID+1_000_000, newNavTestDriver())
	res := f.nodes(t, other, "", "databases")
	assert.Equal(t, res.StatusCode, http.StatusGone)
}

func (*navTestDriver) InspectRelationshipsInScope(_ context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	return &metadata.RelationshipGraph{Scope: scope, Relationships: []metadata.Relationship{{Name: "orders_user_fk"}}}, nil
}

func (*navTestDriver) InspectDefinition(_ context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	return &metadata.Descriptor{Title: "DDL", Source: &metadata.Source{Language: "sql", Body: "CREATE TABLE " + ref.Name + " ()"}}, nil
}

func (*navTestDriver) DDLSpec() ddl.Spec {
	return ddl.Spec{
		Operations:               []ddl.Operation{ddl.OperationCreateTable},
		ColumnTypes:              []string{"integer"},
		CreatableTableScopeKinds: []string{"schema"},
	}
}

func (d *navTestDriver) ApplyDDL(_ context.Context, req ddl.Request) error {
	d.q.mu.Lock()
	defer d.q.mu.Unlock()
	key := string(req.Scope) + "#tables"
	d.q.children[key] = append(d.q.children[key], metadata.Child{Kind: "table", Name: req.Name})
	return nil
}

func navSchema(db, schema string) metadata.ScopePath {
	return navDB(db).Child(metadata.ScopeSegment{Kind: "schema", Name: schema})
}

func navTable(db, schema, table string) metadata.ScopePath {
	return navSchema(db, schema).Child(metadata.ScopeSegment{Kind: "table", Name: table})
}

func (f navFixture) do(t *testing.T, sessionID, method, suffix string, body map[string]any) testResponse {
	t.Helper()
	req := newAuthRequest(t, method, f.base+suffix, body, f.tok)
	if sessionID != "" {
		req.Header.Set("X-Warden-Session", sessionID)
	}
	return send(t, req, f.app.routes())
}

func usersRef() map[string]any {
	return map[string]any{
		"scope": []any{map[string]any{"kind": "database", "name": "app"}, map[string]any{"kind": "schema", "name": "public"}},
		"kind":  "table",
		"name":  "users",
	}
}

func TestSchemaObjectsServeCacheWithoutSessionAndGateMisses(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	body := map[string]any{"refs": []any{usersRef()}}

	assert.Equal(t, f.do(t, "", http.MethodPost, "/schema/objects", body).StatusCode, http.StatusConflict)

	sessionID := f.attach(t, newNavTestDriver())
	live := f.do(t, sessionID, http.MethodPost, "/schema/objects", body)
	assert.Equal(t, live.StatusCode, http.StatusOK)
	assert.Equal(t, len(live.BodyFields["objects"].([]any)), 1)

	f.app.schemaNavigator.ForgetConnection(f.conn.ID)
	stored := f.do(t, "", http.MethodPost, "/schema/objects", body)
	assert.Equal(t, stored.StatusCode, http.StatusOK)
	_, hasPending := stored.BodyFields["pending_connection"]
	assert.Equal(t, hasPending, false)
}

func TestSchemaRelationshipsSessionGatedOnMiss(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	suffix := "/schema/relationships?scope=" + schemaScopeParam(navSchema("app", "public"))

	assert.Equal(t, f.do(t, "", http.MethodGet, suffix, nil).StatusCode, http.StatusConflict)
	sessionID := f.attach(t, newNavTestDriver())
	assert.Equal(t, f.do(t, sessionID, http.MethodGet, suffix, nil).StatusCode, http.StatusOK)
	cached := f.do(t, "", http.MethodGet, suffix, nil)
	assert.Equal(t, cached.StatusCode, http.StatusOK)
	graph := cached.BodyFields["graph"].(map[string]any)
	assert.Equal(t, len(graph["relationships"].([]any)), 1)
}

func TestSchemaDefinitionRequiresSession(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	ref := metadata.ObjectRef{Scope: navSchema("app", "public"), Kind: "table", Name: "users"}
	suffix := "/schema/object/definition?scope=" + schemaScopeParam(ref.Scope) + "&kind=table&name=users"

	res := f.do(t, "", http.MethodGet, suffix, nil)
	assert.Equal(t, res.StatusCode, http.StatusConflict)
	assertAPIError(t, res, "session_required", "Connect to this database to load schema objects.")

	sessionID := f.attach(t, newNavTestDriver())
	assert.Equal(t, f.do(t, sessionID, http.MethodGet, suffix, nil).StatusCode, http.StatusOK)
}

func TestSchemaMutationRefreshesAffectedListings(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	drv := newNavTestDriver()
	drv.q.set(navSchema("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	sessionID := f.attach(t, drv)
	assert.Equal(t, f.nodes(t, sessionID, navSchema("app", "public"), "tables").StatusCode, http.StatusOK)

	res := f.do(t, sessionID, http.MethodPost, "/schema/mutations", map[string]any{
		"operation": "create_table",
		"scope":     []any{map[string]any{"kind": "database", "name": "app"}, map[string]any{"kind": "schema", "name": "public"}},
		"name":      "events",
		"columns":   []any{map[string]any{"name": "id", "data_type": "integer", "primary_key": true}},
	})
	assert.Equal(t, res.StatusCode, http.StatusOK)
	if len(res.BodyFields["listings"].([]any)) == 0 {
		t.Fatalf("mutation returned no refreshed listings: %s", res.BodyBytes)
	}

	after := f.nodes(t, "", navSchema("app", "public"), "tables")
	assert.Equal(t, len(itemNamesOf(t, after)), 2)
}

func TestCompletionIndexBuiltFromCachedListings(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	drv := newNavTestDriver()
	drv.q.set(navSchema("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	drv.q.set(navTable("app", "public", "users"), "columns",
		metadata.Child{Kind: "column", Name: "id", Attributes: map[string]any{"data_type": "integer", "ordinal": 1}})
	sessionID := f.attach(t, drv)
	f.nodes(t, sessionID, "", "databases")
	f.nodes(t, sessionID, navDB("app"), "schemas")
	f.nodes(t, sessionID, navSchema("app", "public"), "tables")
	f.nodes(t, sessionID, navTable("app", "public", "users"), "columns")

	res := f.do(t, "", http.MethodGet, "/schema/completion-index", nil)
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, hasIndexObject(completionIndexObjects(res.BodyFields), "public", "users", "table"), true)
	assert.Equal(t, hasIndexColumn(completionIndexColumns(res.BodyFields), "public", "users", "id", "integer", false), true)
}

func TestSchemaReadPathsNeverOpenTargetConnections(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	ref := usersRef()
	reads := []struct {
		method, suffix string
		body           map[string]any
	}{
		{http.MethodGet, "/schema/tree", nil},
		{http.MethodGet, "/schema/nodes?folder=databases", nil},
		{http.MethodPost, "/schema/objects", map[string]any{"refs": []any{ref}}},
		{http.MethodGet, "/schema/relationships?scope=" + schemaScopeParam(navSchema("app", "public")), nil},
		{http.MethodGet, "/schema/object/definition?scope=" + schemaScopeParam(navSchema("app", "public")) + "&kind=table&name=users", nil},
		{http.MethodGet, "/schema/completion-index", nil},
		{http.MethodPost, "/schema/refresh", map[string]any{"path": []any{}}},
	}
	for _, read := range reads {
		res := f.do(t, "", read.method, read.suffix, read.body)
		if res.StatusCode >= 500 {
			t.Fatalf("%s %s: status %d body %s", read.method, read.suffix, res.StatusCode, res.BodyBytes)
		}
	}
	assert.Equal(t, testConnManager(t, f.app).CountForConnection(strconv.FormatInt(f.conn.ID, 10)), 0)
}

func TestCompleteConnectionSQLFetchesUnexpandedRelations(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	drv := newNavTestDriver()
	drv.q.set(navSchema("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	sessionID := f.attach(t, drv)
	res := f.do(t, sessionID, http.MethodPost, "/completion", map[string]any{"sql": "SELECT * FROM ", "cursor_offset": 14})
	assert.Equal(t, res.StatusCode, http.StatusOK)
	if !responseHasCompletionLabel(res.BodyFields, "users") {
		t.Fatalf("want users after fetch, got %s", res.BodyBytes)
	}
	assert.Equal(t, res.BodyFields["metadata_loaded"], true)
	assert.Equal(t, res.BodyFields["metadata_status"], "ready")
}

func TestCompleteConnectionSQLDoesNotFetchWithoutSession(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	drv := newNavTestDriver()
	drv.q.set(navSchema("app", "public"), "tables", metadata.Child{Kind: "table", Name: "users"})
	res := f.do(t, "", http.MethodPost, "/completion", map[string]any{"sql": "SELECT * FROM ", "cursor_offset": 14})
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["metadata_status"], "partial")
	assert.Equal(t, res.BodyFields["metadata_loaded"], false)
	if responseHasCompletionLabel(res.BodyFields, "users") {
		t.Fatal("fetched without a session")
	}
	assert.Equal(t, drv.q.loadCalls(), 0)
}
