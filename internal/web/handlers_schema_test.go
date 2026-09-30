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
			Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1,
			Relational: true, SupportsDiagram: true, Listing: "enumerated",
		}},
	}
}

func (schemaFakeDriver) InspectDirectory(context.Context, metadata.DirectoryOptions) (*metadata.Directory, error) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	return &metadata.Directory{
		Engine: "sqlite", DefaultScope: scope,
		Roots: []metadata.ScopeNode{{Path: scope, Groups: []metadata.ObjectGroup{{
			Kind: "table", Objects: []metadata.ObjectRef{{Scope: scope, Kind: "table", Name: "widgets"}},
		}}}},
	}, nil
}

func (schemaFakeDriver) InspectObjects(_ context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	out := make([]metadata.Object, 0, len(refs))
	for _, ref := range refs {
		out = append(out, metadata.Object{
			Ref:        ref,
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}}},
		})
	}
	return out, nil
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

func TestGenerateConnectionStatement(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "statement-generate"), "Statement", "Statement Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Statement WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Statement Conn", "open")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})
	ref := map[string]any{
		"scope": []map[string]any{{"kind": "database", "name": "main"}},
		"kind":  "table",
		"name":  "widgets",
	}
	endpoint := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10)) + "/schema/statements"

	for _, test := range []struct {
		operation string
		contains  string
	}{
		{operation: "select", contains: "SELECT\n  \"id\"\nFROM \"main\".\"widgets\";"},
		{operation: "insert", contains: "VALUES (\n  ?\n);"},
		{operation: "update", contains: "WHERE 1 = 0;"},
		{operation: "delete", contains: "DELETE FROM \"main\".\"widgets\"\nWHERE 1 = 0;"},
	} {
		t.Run(test.operation, func(t *testing.T) {
			req := newAuthRequest(t, http.MethodPost, endpoint, map[string]any{"operation": test.operation, "ref": ref}, tok)
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
	req := newAuthRequest(t, http.MethodPost, endpoint, map[string]any{"operation": "delete", "ref": viewRef}, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assert.Equal(t, res.BodyFields["error"].(map[string]any)["code"], "statement_generation_failed")

	req = newAuthRequest(t, http.MethodPost, endpoint, map[string]any{"operation": "select", "ref": ref}, tok)
	res = send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
}

func TestApplyConnectionDDL(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-edit"), "Schema Edit", "Schema Edit Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
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
	assert.Equal(t, res.BodyFields["schema"].(map[string]any)["mode"], "ephemeral")

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
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
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
		func() (engine.Driver, func(), error) { return drv, nil, nil },
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
	if err := app.db.UpdateConnectionWithPolicy(context.Background(), conn.ID, conn.Name, conn.DSNEncrypted, conn.AccessMode, database.SchemaSnapshotPolicyDisabled); err != nil {
		t.Fatalf("disable snapshots for ephemeral schema test: %v", err)
	}
}

type tableEditFakeDriver struct{ ddlFakeDriver }

func (*tableEditFakeDriver) DDLSpec() ddl.Spec {
	return ddl.Spec{
		Operations:               []ddl.Operation{ddl.OperationAddColumn, ddl.OperationAlterColumn, ddl.OperationCreateIndex},
		ColumnTypes:              []string{"integer", "text"},
		CreatableTableScopeKinds: []string{"database"},
		SupportsColumnDefaults:   true,
	}
}

func TestApplyConnectionTableEditPayloads(t *testing.T) {
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "table-edit"), "Table Edit", "Table Edit Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
	driver := &tableEditFakeDriver{}
	session := openSchemaSession(t, app, owner.ID, conn.ID, driver)
	ref := metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}), Kind: "table", Name: "events"}
	for _, payload := range []map[string]any{
		{"operation": "add_column", "ref": ref, "column": map[string]any{"name": "count", "data_type": "integer", "nullable": false, "default": "0"}},
		{"operation": "alter_column", "ref": ref, "name": "count", "changes": map[string]any{"nullable": true}},
		{"operation": "create_index", "ref": ref, "name": "ix_events", "unique": true, "index_columns": []map[string]any{{"name": "count", "descending": true}, {"name": "id"}}},
	} {
		req := newAuthRequest(t, http.MethodPost, orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/mutations", payload, tok)
		req.Header.Set("X-Warden-Session", session.ID)
		res := send(t, req, app.routes())
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %+v", payload["operation"], res.StatusCode, res.BodyFields)
		}
	}
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if len(driver.applied) != 3 {
		t.Fatalf("applied %d edits", len(driver.applied))
	}
	if column := driver.applied[0].Column; column == nil || column.Default == nil || *column.Default != "0" || column.Nullable {
		t.Fatalf("column payload: %+v", column)
	}
	if changes := driver.applied[1].Changes; changes == nil || changes.Nullable == nil || !*changes.Nullable || changes.Default != nil || changes.DataType != nil {
		t.Fatalf("patch payload: %+v", changes)
	}
	index := driver.applied[2]
	if !index.Unique || len(index.IndexColumns) != 2 || index.IndexColumns[0].Name != "count" || !index.IndexColumns[0].Descending || index.IndexColumns[1].Name != "id" {
		t.Fatalf("index payload: %+v", index)
	}
}
