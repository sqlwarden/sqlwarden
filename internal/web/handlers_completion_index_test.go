package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/engine/metadata"
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

func TestCompletionIndexReturnsProjectedSchema(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-index"), "Index", "Index Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Index WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Index DB", "open")

	scope := metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: "app"},
		metadata.ScopeSegment{Kind: "schema", Name: "public"},
	)
	directory := &metadata.Directory{
		Engine: "postgres", DefaultScope: scope, GeneratedAt: time.Now(),
		Roots: []metadata.ScopeNode{{
			Path: scope,
			Groups: []metadata.ObjectGroup{{
				Kind: "table",
				Objects: []metadata.ObjectRef{
					{Scope: scope, Kind: "table", Name: "orders"},
					{Scope: scope, Kind: "view", Name: "active_orders"},
				},
			}},
		}},
	}
	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.PutObjects(context.Background(), snapshot.ID, []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "orders"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "int8", Nullable: false, Ordinal: 1},
				{Name: "total", DataType: "numeric", Nullable: true, Ordinal: 2},
			}},
		},
		{
			Ref:        metadata.ObjectRef{Scope: scope, Kind: "view", Name: "active_orders"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "int8", Ordinal: 1}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(context.Background(), snapshot.ID); err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodGet, completionIndexURL(org.Slug, ws.ID, envID, conn.ID), nil, token)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["version"], any(snapshot.ID))
	assert.Equal(t, res.BodyFields["default_schema"], "public")
	if !hasString(res.BodyFields["schemas"], "public") {
		t.Fatalf("schemas missing public: %s", res.BodyBytes)
	}
	objects := completionIndexObjects(res.BodyFields)
	if !hasIndexObject(objects, "public", "orders", "table") {
		t.Fatalf("objects missing public.orders table: %s", res.BodyBytes)
	}
	if !hasIndexObject(objects, "public", "active_orders", "view") {
		t.Fatalf("objects missing public.active_orders view: %s", res.BodyBytes)
	}
	columns := completionIndexColumns(res.BodyFields)
	if !hasIndexColumn(columns, "public", "orders", "id", "int8", false) {
		t.Fatalf("columns missing orders.id int8 NOT NULL: %s", res.BodyBytes)
	}
	if !hasIndexColumn(columns, "public", "orders", "total", "numeric", true) {
		t.Fatalf("columns missing orders.total numeric NULL: %s", res.BodyBytes)
	}
}

func TestCompletionIndexPendingSnapshotReturns202(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-index-pending"), "Index", "Index Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Index WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Index DB", "open")

	req := newAuthRequest(t, http.MethodGet, completionIndexURL(org.Slug, ws.ID, envID, conn.ID), nil, token)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusAccepted)
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
