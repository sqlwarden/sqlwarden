package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/pkg/result"
)

// schemaFakeDriver implements schema.SchemaInspector without requiring a live
// target database, keeping schema handler tests focused on HTTP behavior.
type schemaFakeDriver struct{}

func (schemaFakeDriver) Connect(context.Context, driver.ConnectionConfig) error { return nil }
func (schemaFakeDriver) Ping(context.Context) error                             { return nil }
func (schemaFakeDriver) Close() error                                           { return nil }
func (schemaFakeDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (schemaFakeDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (schemaFakeDriver) Dialect() driver.Dialect { return driver.DialectSQLite }

func (schemaFakeDriver) SchemaSpec() schema.SchemaSpec {
	return schema.SchemaSpec{
		Dialect: "sqlite",
		Kinds: []schema.SchemaObjectKind{{
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

func (schemaFakeDriver) InspectCatalog(context.Context, schema.CatalogOptions) (*schema.Catalog, error) {
	return &schema.Catalog{
		Dialect:  "sqlite",
		Database: "test",
		Namespaces: []schema.NamespaceCatalog{{
			Name: "main",
			Groups: []schema.ObjectGroupCatalog{{
				Kind:    "table",
				Objects: []schema.ObjectRef{{Namespace: "main", Kind: "table", Name: "widgets"}},
			}},
		}},
	}, nil
}

func (schemaFakeDriver) InspectObjects(_ context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	out := make([]schema.Object, 0, len(refs))
	for _, ref := range refs {
		out = append(out, schema.Object{
			Ref: ref,
			Relational: &schema.RelationalDetail{
				Columns: []schema.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}},
			},
		})
	}
	return out, nil
}

func TestGetConnectionCatalog_RequiresSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-owner"), "Schema Owner", "Schema Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/catalog", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestGetConnectionCatalog_InspectsAndCaches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-owner2"), "Schema Owner2", "Schema Org2")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/catalog", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	catalogField, ok := res.BodyFields["catalog"].(map[string]any)
	if !ok {
		t.Fatalf("expected catalog object, got %v", res.BodyFields)
	}
	assert.Equal(t, catalogField["dialect"], "sqlite")
	namespaces, ok := catalogField["namespaces"].([]any)
	if !ok || len(namespaces) != 1 {
		t.Fatalf("expected one namespace, got %v", catalogField)
	}
	firstNamespace := namespaces[0].(map[string]any)
	groups := firstNamespace["groups"].([]any)
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
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
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
}

func TestPostConnectionObjects(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-objects"), "Schema Objects", "Schema Objects Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/objects",
		map[string]any{"refs": []map[string]any{{"namespace": "main", "kind": "table", "name": "widgets"}}},
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
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/refresh", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["status"], "ok")

	req = newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/refresh",
		map[string]any{"ref": map[string]any{"namespace": "main", "kind": "table", "name": "widgets"}},
		tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res = send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["status"], "ok")
}

func TestGetConnectionCatalog_SessionExpired(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-expired"), "Schema Expired", "Schema Expired Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/catalog", nil, tok)
	req.Header.Set("X-Warden-Session", "nonexistent-session-id")
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusGone)
}

func TestGetConnectionCatalog_SessionConnectionMismatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-mismatch"), "Schema Mismatch", "Schema Mismatch Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	connA := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Conn A", "open")
	connB := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Conn B", "open")
	sess := openSchemaSession(t, app, owner.ID, connB.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connA.ID, 10))+"/schema/catalog", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

// nonSchemaInspectableDriver exercises the 501 unsupported-driver path.
type nonSchemaInspectableDriver struct{}

func (nonSchemaInspectableDriver) Connect(context.Context, driver.ConnectionConfig) error { return nil }
func (nonSchemaInspectableDriver) Ping(context.Context) error                             { return nil }
func (nonSchemaInspectableDriver) Close() error                                           { return nil }
func (nonSchemaInspectableDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (nonSchemaInspectableDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (nonSchemaInspectableDriver) Dialect() driver.Dialect { return driver.DialectSQLite }

func TestGetConnectionCatalog_UnsupportedDriver(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-unsupported"), "Schema Unsupported", "Schema Unsupported Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, nonSchemaInspectableDriver{})

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/catalog", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
}

func openSchemaSession(t *testing.T, app *application, accountID, connectionID int64, drv driver.Driver) *connection.Session {
	t.Helper()
	sess, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(accountID, 10),
		strconv.FormatInt(connectionID, 10),
		func() (driver.Driver, error) { return drv, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}
