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
	byID := make(map[string]map[string]any)
	for _, e := range engines {
		m := e.(map[string]any)
		byID[m["id"].(string)] = m
	}
	pg := byID["postgres"]
	if pg == nil {
		t.Fatalf("postgres engine missing from %v", engines)
	}
	assert.Equal(t, pg["display_name"], "PostgreSQL")
	caps := pg["capabilities"].(map[string]any)
	assert.Equal(t, caps["schema.directory"], true)
	assert.Equal(t, caps["query.cursor"], true)
	assert.Equal(t, caps["sql.complete"], true)
	assert.Equal(t, byID["mysql"]["capabilities"].(map[string]any)["sql.complete"], true)
	assert.Equal(t, byID["sqlite"]["capabilities"].(map[string]any)["sql.complete"], false)
}

func TestGetEngineUnknownReturns404(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "engines404"), "E404", "E404 Org")

	req := newAuthRequest(t, http.MethodGet, "/api/v1/engines/does-not-exist", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestGetEngineCompletionVocabulary(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "engine-vocabulary"), "Vocabulary", "Vocabulary Org")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/engines/postgres/completion-vocabulary", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["dialect"], "postgres")
	if res.BodyFields["version"] == "" {
		t.Fatal("expected deterministic vocabulary version")
	}
	suggestions := res.BodyFields["suggestions"].([]any)
	foundSelect, foundCount := false, false
	for _, raw := range suggestions {
		suggestion := raw.(map[string]any)
		if suggestion["label"] == "SELECT" && suggestion["kind"] == "keyword" {
			foundSelect = true
		}
		if suggestion["label"] == "count" && suggestion["kind"] == "function" {
			foundCount = true
		}
	}
	if !foundSelect || !foundCount {
		t.Fatalf("representative vocabulary missing: SELECT=%v count=%v", foundSelect, foundCount)
	}

	sqlite := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/engines/sqlite/completion-vocabulary", nil, tok), app.routes())
	assert.Equal(t, sqlite.StatusCode, http.StatusNotImplemented)

	unknown := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/engines/unknown/completion-vocabulary", nil, tok), app.routes())
	assert.Equal(t, unknown.StatusCode, http.StatusNotFound)
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
