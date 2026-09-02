package web

import (
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"

	_ "github.com/sqlwarden/internal/engine/engines/mysql"
	_ "github.com/sqlwarden/internal/engine/engines/oracle"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
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

	assert.Equal(t, caps["sql.explain"], true)
	pgExplain, ok := pg["explain"].(map[string]any)
	if !ok {
		t.Fatalf("expected postgres explain spec in %v", pg)
	}
	assert.Equal(t, pgExplain["supports_analyze"], true)

	mysqlExplain, ok := byID["mysql"]["explain"].(map[string]any)
	if !ok {
		t.Fatalf("expected mysql explain spec in %v", byID["mysql"])
	}
	assert.Equal(t, mysqlExplain["supports_analyze"], true)

	sqliteExplain, ok := byID["sqlite"]["explain"].(map[string]any)
	if !ok {
		t.Fatalf("expected sqlite explain spec in %v", byID["sqlite"])
	}
	assert.Equal(t, sqliteExplain["supports_analyze"], false)

	oracle := byID["oracle"]
	if oracle == nil {
		t.Fatalf("oracle engine missing from %v", engines)
	}
	assert.Equal(t, oracle["display_name"], "Oracle")
	assert.Equal(t, oracle["capabilities"].(map[string]any)["sql.explain"], false)
	if _, hasExplain := oracle["explain"]; hasExplain {
		t.Fatalf("oracle must not carry an explain spec: %v", oracle)
	}
}

func TestGetEngineIncludesExplainSpec(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "engine-explain"), "Explain", "Explain Org")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/engines/postgres", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["capabilities"].(map[string]any)["sql.explain"], true)
	explainSpec, ok := res.BodyFields["explain"].(map[string]any)
	if !ok {
		t.Fatalf("expected explain spec in %v", res.BodyFields)
	}
	assert.Equal(t, explainSpec["supports_analyze"], true)

	sqliteRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/engines/sqlite", nil, tok), app.routes())
	assert.Equal(t, sqliteRes.StatusCode, http.StatusOK)
	sqliteExplainSpec, ok := sqliteRes.BodyFields["explain"].(map[string]any)
	if !ok {
		t.Fatalf("expected explain spec in %v", sqliteRes.BodyFields)
	}
	assert.Equal(t, sqliteExplainSpec["supports_analyze"], false)
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

func TestGetEngineOracleOmitsExplain(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "engine-oracle"), "Oracle", "Oracle Org")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/engines/oracle", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["capabilities"].(map[string]any)["sql.explain"], false)
	if _, ok := res.BodyFields["explain"]; ok {
		t.Fatalf("oracle response must not include an explain spec: %v", res.BodyFields)
	}
	assert.Equal(t, res.BodyFields["capabilities"].(map[string]any)["sql.complete"], true)
}

func TestGetEngineOracleCompletionVocabulary(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, uniqueEmail(t, "oracle-vocab"), "OV", "OV Org")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/engines/oracle/completion-vocabulary", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["dialect"], "oracle")
	suggestions := res.BodyFields["suggestions"].([]any)
	foundSelect := false
	for _, raw := range suggestions {
		s := raw.(map[string]any)
		if s["label"] == "SELECT" && s["kind"] == "keyword" {
			foundSelect = true
		}
	}
	if !foundSelect {
		t.Fatal("oracle vocabulary missing SELECT keyword")
	}
}
