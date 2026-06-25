package web

import (
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"

	_ "github.com/sqlwarden/internal/dbengine/engines/mysql"
	_ "github.com/sqlwarden/internal/dbengine/engines/postgres"
	_ "github.com/sqlwarden/internal/dbengine/engines/sqlite"
)

func TestListEngines(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "engines"), "Engines", "Engines Org")

	req := newAuthRequest(t, http.MethodGet, "/api/v1/engines", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	engines := res.BodyFields["engines"].([]any)
	var pg map[string]any
	for _, e := range engines {
		m := e.(map[string]any)
		if m["id"] == "postgres" {
			pg = m
		}
	}
	if pg == nil {
		t.Fatalf("postgres engine missing from %v", engines)
	}
	assert.Equal(t, pg["display_name"], "PostgreSQL")
	caps := pg["capabilities"].(map[string]any)
	assert.Equal(t, caps["schema.catalog"], true)
	assert.Equal(t, caps["query.cursor"], true)
	assert.Equal(t, caps["sql.complete"], false)
}

func TestGetEngineUnknownReturns404(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "engines404"), "E404", "E404 Org")

	req := newAuthRequest(t, http.MethodGet, "/api/v1/engines/does-not-exist", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestListEnginesRequiresAuth(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/engines", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}
