package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/pkg/result"
)

// schemaFakeDriver is a driver.Driver that also implements schema.Introspector,
// returning a fixed schema so handler tests don't need a live target database.
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
func (schemaFakeDriver) Introspect(context.Context, schema.IntrospectOptions) (*schema.Schema, error) {
	return &schema.Schema{Namespaces: []schema.Namespace{{
		Name: "main",
		ObjectGroups: []schema.ObjectGroup{{Kind: "table", Label: "Tables", Objects: []schema.Object{
			{Name: "users", Columns: []schema.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}}},
		}}},
	}}}, nil
}

func TestGetConnectionSchema_RequiresSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-owner"), "Schema Owner", "Schema Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestGetConnectionSchema_IntrospectsAndCaches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-owner2"), "Schema Owner2", "Schema Org2")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	sess, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(conn.ID, 10),
		func() (driver.Driver, error) { return schemaFakeDriver{}, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	schemaField, ok := res.BodyFields["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %v", res.BodyFields)
	}
	namespaces, ok := schemaField["namespaces"].([]any)
	if !ok || len(namespaces) == 0 {
		t.Fatalf("expected namespaces, got %v", schemaField)
	}
}

func TestRefreshConnectionSchema(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-refresh"), "Schema Refresh", "Schema Refresh Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	sess, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(conn.ID, 10),
		func() (driver.Driver, error) { return schemaFakeDriver{}, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/refresh", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	if _, ok := res.BodyFields["schema"].(map[string]any); !ok {
		t.Fatalf("expected schema object, got %v", res.BodyFields)
	}
}

func TestGetConnectionSchema_SessionExpired(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-expired"), "Schema Expired", "Schema Expired Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema", nil, tok)
	req.Header.Set("X-Warden-Session", "nonexistent-session-id")
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusGone)
}

func TestGetConnectionSchema_SessionConnectionMismatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-mismatch"), "Schema Mismatch", "Schema Mismatch Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	connA := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Conn A", "open")
	connB := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Conn B", "open")

	// Session belongs to connB.
	sess, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(connB.ID, 10),
		func() (driver.Driver, error) { return schemaFakeDriver{}, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	// Request connA's schema using connB's session.
	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connA.ID, 10))+"/schema", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

// nonIntrospectableDriver is a driver.Driver that does NOT implement
// schema.Introspector, used to exercise the 501 unsupported-driver path.
type nonIntrospectableDriver struct{}

func (nonIntrospectableDriver) Connect(context.Context, driver.ConnectionConfig) error { return nil }
func (nonIntrospectableDriver) Ping(context.Context) error                             { return nil }
func (nonIntrospectableDriver) Close() error                                           { return nil }
func (nonIntrospectableDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (nonIntrospectableDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (nonIntrospectableDriver) Dialect() driver.Dialect { return driver.DialectSQLite }

func TestGetConnectionSchema_UnsupportedDriver(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-unsupported"), "Schema Unsupported", "Schema Unsupported Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")

	sess, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(conn.ID, 10),
		func() (driver.Driver, error) { return nonIntrospectableDriver{}, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema", nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
}
