package web

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

// schemaFakeDriver implements metadata.SchemaInspector without requiring a live
// target database, keeping schema handler tests focused on HTTP behavior.
type schemaFakeDriver struct{}

func (schemaFakeDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (schemaFakeDriver) Ping(context.Context) error                             { return nil }
func (schemaFakeDriver) Close() error                                           { return nil }
func (schemaFakeDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (schemaFakeDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (schemaFakeDriver) Dialect() engine.Dialect { return engine.DialectSQLite }

func (schemaFakeDriver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "sqlite",
		Kinds: []metadata.SchemaObjectKind{{
			Kind:            "table",
			Label:           "Table",
			PluralLabel:     "Tables",
			Order:           1,
			Relational:      true,
			SupportsDiagram: true,
			Listing:         "enumerated",
		}},
	}
}

func (schemaFakeDriver) InspectDirectory(context.Context, metadata.DirectoryOptions) (*metadata.Directory, error) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	return &metadata.Directory{
		Engine: "sqlite", DefaultScope: scope,
		Roots: []metadata.ScopeNode{{
			Path: scope,
			Groups: []metadata.ObjectGroup{{
				Kind:    "table",
				Objects: []metadata.ObjectRef{{Scope: scope, Kind: "table", Name: "widgets"}},
			}},
		}},
	}, nil
}

func (schemaFakeDriver) InspectObjects(_ context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	out := make([]metadata.Object, 0, len(refs))
	for _, ref := range refs {
		out = append(out, metadata.Object{
			Ref: ref,
			Relational: &metadata.RelationalDetail{
				Columns: []metadata.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}},
			},
		})
	}
	return out, nil
}

// schemaRelDriver is schemaFakeDriver plus the optional RelationshipInspector
// capability, used to exercise the relationships endpoint. schemaFakeDriver
// deliberately does NOT implement it, so it drives the 501 path.
type schemaRelDriver struct{ schemaFakeDriver }

func (schemaRelDriver) InspectRelationshipsInScope(_ context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	return &metadata.RelationshipGraph{
		Scope: scope,
		Relationships: []metadata.Relationship{{
			Name:              "orders_user_fk",
			Source:            metadata.ObjectRef{Scope: scope, Kind: "table", Name: "orders"},
			Columns:           []string{"user_id"},
			References:        metadata.ObjectRef{Scope: scope, Kind: "table", Name: "users"},
			ReferencedColumns: []string{"id"},
		}},
	}, nil
}

type ddlFakeDriver struct {
	schemaFakeDriver
	mu      sync.Mutex
	applied []ddl.Request
}

func (*ddlFakeDriver) DDLSpec() ddl.Spec {
	return ddl.Spec{
		Operations:               []ddl.Operation{ddl.OperationCreateTable, ddl.OperationDropObject},
		ColumnTypes:              []string{"integer", "text"},
		CreatableTableScopeKinds: []string{"database"},
		DroppableObjectKinds:     []string{"table", "view"},
	}
}

func (d *ddlFakeDriver) ApplyDDL(_ context.Context, request ddl.Request) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.applied = append(d.applied, request)
	return nil
}

func TestGetConnectionSchemaRelationships(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-rel"), "Schema Rel", "Schema Rel Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaRelDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/relationships?scope="+schemaScopeParam(metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"})), nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	graph, ok := res.BodyFields["graph"].(map[string]any)
	if !ok {
		t.Fatalf("expected graph object, got %v", res.BodyFields)
	}
	rels, ok := graph["relationships"].([]any)
	if !ok || len(rels) != 1 {
		t.Fatalf("expected one relationship, got %v", graph)
	}
	first := rels[0].(map[string]any)
	refObj := first["references"].(map[string]any)
	assert.Equal(t, refObj["name"], "users")
}

func TestGetConnectionSchemaRelationships_Unsupported(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-rel-unsup"), "Schema Rel U", "Schema Rel U Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/relationships?scope="+schemaScopeParam(metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"})), nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
}

func TestGetConnectionDirectory_RequiresSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-owner"), "Schema Owner", "Schema Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	disableSchemaSnapshots(t, app, conn.ID)

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/directory", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestGetConnectionDirectory_InspectsAndCaches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-owner2"), "Schema Owner2", "Schema Org2")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/directory", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	directoryField, ok := res.BodyFields["directory"].(map[string]any)
	if !ok {
		t.Fatalf("expected directory object, got %v", res.BodyFields)
	}
	assert.Equal(t, directoryField["engine"], "sqlite")
	roots, ok := directoryField["roots"].([]any)
	if !ok || len(roots) != 1 {
		t.Fatalf("expected one root, got %v", directoryField)
	}
	firstRoot := roots[0].(map[string]any)
	groups := firstRoot["groups"].([]any)
	firstGroup := groups[0].(map[string]any)
	objects := firstGroup["objects"].([]any)
	firstObject := objects[0].(map[string]any)
	assert.Equal(t, firstObject["name"], "widgets")
}

func TestGetConnectionSchemaSpec(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-spec"), "Schema Spec", "Schema Spec Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/spec", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	spec := res.BodyFields["spec"].(map[string]any)
	kinds := spec["kinds"].([]any)
	table := kinds[0].(map[string]any)
	assert.Equal(t, table["kind"], "table")
	assert.Equal(t, table["listing"], "enumerated")
	if _, ok := res.BodyFields["editor"].(map[string]any); !ok {
		t.Fatalf("expected schema editor spec, got %v", res.BodyFields["editor"])
	}
	if _, ok := res.BodyFields["statements"].(map[string]any); !ok {
		t.Fatalf("expected statement generator spec, got %v", res.BodyFields["statements"])
	}
}

func TestGenerateConnectionStatement(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "statement-generate"), "Statement", "Statement Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Statement WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Statement Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})
	ref := map[string]any{
		"scope": []map[string]any{{"kind": "database", "name": "main"}},
		"kind":  "table",
		"name":  "widgets",
	}
	url := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10)) + "/schema/statements"

	tests := []struct {
		operation string
		contains  string
	}{
		{operation: "select", contains: "SELECT\n  \"id\"\nFROM \"main\".\"widgets\";"},
		{operation: "insert", contains: "VALUES (\n  ?\n);"},
		{operation: "update", contains: "WHERE 1 = 0;"},
		{operation: "delete", contains: "DELETE FROM \"main\".\"widgets\"\nWHERE 1 = 0;"},
	}
	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			req := newAuthRequest(t, http.MethodPost, url, map[string]any{"operation": test.operation, "ref": ref}, tok)
			req.Header.Set("X-Warden-Session", sess.ID)
			res := send(t, req, app.routes())
			assert.Equal(t, res.StatusCode, http.StatusOK)
			generated, _ := res.BodyFields["sql"].(string)
			if !strings.Contains(generated, test.contains) {
				t.Fatalf("generated %s = %q, want substring %q", test.operation, generated, test.contains)
			}
		})
	}

	viewRef := maps.Clone(ref)
	viewRef["kind"] = "view"
	req := newAuthRequest(t, http.MethodPost, url, map[string]any{"operation": "delete", "ref": viewRef}, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assert.Equal(t, res.BodyFields["error"].(map[string]any)["code"], "statement_generation_failed")

	req = newAuthRequest(t, http.MethodPost, url, map[string]any{"operation": "select", "ref": ref}, tok)
	res = send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestApplyConnectionDDL(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-edit"), "Schema Edit", "Schema Edit Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	driver := &ddlFakeDriver{}
	sess := openSchemaSession(t, app, owner.ID, conn.ID, driver)
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/mutations",
		map[string]any{"operation": "create_table", "scope": scope, "name": "events", "columns": []map[string]any{{"name": "id", "data_type": "integer", "primary_key": true}}}, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["applied"], true)
	schemaStatus := res.BodyFields["schema"].(map[string]any)
	assert.Equal(t, schemaStatus["mode"], "ephemeral")

	driver.mu.Lock()
	defer driver.mu.Unlock()
	if len(driver.applied) != 1 || driver.applied[0].Name != "events" {
		t.Fatalf("applied edits = %+v", driver.applied)
	}
}

func TestApplyConnectionDDLRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-edit-invalid"), "Schema Edit", "Schema Edit Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	driver := &ddlFakeDriver{}
	sess := openSchemaSession(t, app, owner.ID, conn.ID, driver)

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/mutations",
		map[string]any{"operation": "create_table", "scope": metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}), "name": "events", "columns": []map[string]any{{"name": "id", "data_type": "text); DROP TABLE users"}}}, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assert.Equal(t, res.BodyFields["error"].(map[string]any)["code"], "invalid_schema_edit")
	if len(driver.applied) != 0 {
		t.Fatalf("invalid edit reached driver: %+v", driver.applied)
	}
}

func TestPostConnectionObjects(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-objects"), "Schema Objects", "Schema Objects Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/objects",
		map[string]any{"refs": []map[string]any{{"scope": []map[string]any{{"kind": "database", "name": "main"}}, "kind": "table", "name": "widgets"}}},
		tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	objects := res.BodyFields["objects"].([]any)
	first := objects[0].(map[string]any)
	ref := first["ref"].(map[string]any)
	assert.Equal(t, ref["name"], "widgets")
	rel := first["relational"].(map[string]any)
	columns := rel["columns"].([]any)
	column := columns[0].(map[string]any)
	assert.Equal(t, column["name"], "id")
}

func TestRefreshConnectionSchema(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-refresh"), "Schema Refresh", "Schema Refresh Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/refresh", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["status"], "ok")

	req = newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/refresh",
		map[string]any{"ref": map[string]any{"scope": []map[string]any{{"kind": "database", "name": "main"}}, "kind": "table", "name": "widgets"}},
		tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res = send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["status"], "ok")
}

func TestGetConnectionDirectory_SessionExpired(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-expired"), "Schema Expired", "Schema Expired Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	disableSchemaSnapshots(t, app, conn.ID)

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/directory", nil, tok)
	req.Header.Set("X-Warden-Session", "nonexistent-session-id")
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusGone)
}

func TestGetConnectionDirectory_SessionConnectionMismatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-mismatch"), "Schema Mismatch", "Schema Mismatch Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	connA := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Conn A")
	connB := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Conn B")
	disableSchemaSnapshots(t, app, connA.ID)
	sess := openSchemaSession(t, app, owner.ID, connB.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connA.ID, 10))+"/schema/directory", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

// nonSchemaInspectableDriver exercises the 501 unsupported-driver path.
type nonSchemaInspectableDriver struct{}

func (nonSchemaInspectableDriver) Connect(context.Context, engine.ConnectionConfig) error {
	return nil
}
func (nonSchemaInspectableDriver) Ping(context.Context) error { return nil }
func (nonSchemaInspectableDriver) Close() error               { return nil }
func (nonSchemaInspectableDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (nonSchemaInspectableDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (nonSchemaInspectableDriver) Dialect() engine.Dialect { return engine.DialectSQLite }

func TestGetConnectionDirectory_UnsupportedDriver(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-unsupported"), "Schema Unsupported", "Schema Unsupported Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, nonSchemaInspectableDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/directory", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
}

func schemaScopeParam(scope metadata.ScopePath) string {
	data, _ := json.Marshal(scope)
	return url.QueryEscape(string(data))
}

func openSchemaSession(t *testing.T, app *application, accountID, connectionID int64, drv engine.Driver) *connection.Session {
	t.Helper()
	disableSchemaSnapshots(t, app, connectionID)
	sess, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(accountID, 10),
		strconv.FormatInt(connectionID, 10),
		func() (engine.Driver, error) { return drv, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func disableSchemaSnapshots(t *testing.T, app *application, connectionID int64) {
	t.Helper()
	conn, found, err := app.db.GetConnection(context.Background(), connectionID)
	if err != nil || !found {
		t.Fatalf("get schema test connection: found=%v err=%v", found, err)
	}
	if err := app.db.UpdateConnectionWithPolicy(context.Background(), conn.ID, conn.Name, conn.DSNEncrypted, database.SchemaSnapshotPolicyDisabled); err != nil {
		t.Fatalf("disable snapshots for ephemeral schema test: %v", err)
	}
}
