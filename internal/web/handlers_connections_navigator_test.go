package web

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

type navConnections struct {
	app    *application
	tok    string
	create string
	one    func(id any) string
}

func newNavConnections(t *testing.T) navConnections {
	t.Helper()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "navconn"), "Nav Conn Owner", "Nav Conn Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Nav Conn WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	return navConnections{
		app:    app,
		tok:    tok,
		create: orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		one: func(id any) string {
			return orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(int64(id.(float64)), 10))
		},
	}
}

func (c navConnections) send(t *testing.T, method, target string, body map[string]any) testResponse {
	t.Helper()
	return send(t, newAuthRequest(t, method, target, body, c.tok), c.app.routes())
}

var reportsScope = []any{map[string]any{"kind": "database", "name": "reports"}}

func TestCreateConnectionAppliesShowAllDatabasesRule(t *testing.T) {
	t.Parallel()
	c := newNavConnections(t)
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"no default database forces on", map[string]any{"driver": navTestEngine, "dsn": "dsn", "show_all_databases": false}, true},
		{"default database honours off", map[string]any{"driver": navTestEngine, "dsn": "dsn", "default_scope": reportsScope, "show_all_databases": false}, false},
		{"default database honours on", map[string]any{"driver": navTestEngine, "dsn": "dsn", "default_scope": reportsScope, "show_all_databases": true}, true},
		{"driver without a database level ignores it", map[string]any{"driver": navFlatEngine, "dsn": "dsn", "show_all_databases": true}, false},
	}
	for _, tc := range cases {
		tc.body["name"] = tc.name
		res := c.send(t, http.MethodPost, c.create, tc.body)
		assert.Equal(t, res.StatusCode, http.StatusCreated)
		assert.Equal[any](t, res.BodyFields["show_all_databases"], tc.want)
	}
}

func TestUpdateConnectionAppliesShowAllDatabasesRule(t *testing.T) {
	t.Parallel()
	c := newNavConnections(t)
	created := c.send(t, http.MethodPost, c.create, map[string]any{
		"name": "Nav", "driver": navTestEngine, "dsn": "dsn", "default_scope": reportsScope, "show_all_databases": true,
	})
	assert.Equal(t, created.StatusCode, http.StatusCreated)
	target := c.one(created.BodyFields["id"])

	res := c.send(t, http.MethodPatch, target, map[string]any{"show_all_databases": false})
	assert.Equal(t, res.StatusCode, http.StatusNoContent)
	got := c.send(t, http.MethodGet, target, nil)
	assert.Equal[any](t, got.BodyFields["show_all_databases"], false)

	res = c.send(t, http.MethodPatch, target, map[string]any{"default_scope": []any{}})
	assert.Equal(t, res.StatusCode, http.StatusNoContent)
	got = c.send(t, http.MethodGet, target, nil)
	assert.Equal[any](t, got.BodyFields["show_all_databases"], true)
}

func TestTestConnectionDiscoversScopesFromGrammar(t *testing.T) {
	t.Parallel()
	c := newNavConnections(t)
	res := c.send(t, http.MethodPost, c.create+"/test", map[string]any{"driver": navTestEngine, "dsn": "dsn"})
	assert.Equal(t, res.StatusCode, http.StatusOK)
	discovery, ok := res.BodyFields["scope_discovery"].(map[string]any)
	if !ok {
		t.Fatalf("scope_discovery = %#v", res.BodyFields["scope_discovery"])
	}
	assert.Equal[any](t, discovery["current"], []any{
		map[string]any{"kind": "database", "name": "app"},
		map[string]any{"kind": "schema", "name": "public"},
	})
	assert.Equal(t, len(discovery["scopes"].([]any)), 3)
}

func TestDriverSupportsSystemSchemasReadsTheGrammar(t *testing.T) {
	t.Parallel()
	assert.Equal(t, driverSupportsSystemSchemas(navTestEngine), true)
	assert.Equal(t, driverSupportsSystemSchemas("no-such-driver"), false)
}
