package web

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func setUpTransactionTestConnection(t *testing.T, emailPrefix string) (app *application, tok string, connURL string) {
	t.Helper()
	app = newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, emailPrefix+"@example.com", "Tx Test", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Tx WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "TxConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connURL = orgConnectionURL(slug, wsIDInt, envID, connID)
	return app, tok, connURL
}

func TestSetTransactionMode_RequiresSessionHeader(t *testing.T) {
	t.Parallel()
	app, tok, connURL := setUpTransactionTestConnection(t, "tx-mode-missing-session")

	req := newAuthRequest(t, http.MethodPost, connURL+"/transaction/mode", map[string]any{"mode": "manual"}, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestTransactionLifecycle_ModeCommitRollbackStatus(t *testing.T) {
	t.Parallel()
	app, tok, connURL := setUpTransactionTestConnection(t, "tx-lifecycle")

	connectRes := send(t, newAuthRequest(t, http.MethodPost, connURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	// Switch to manual.
	modeReq := newAuthRequest(t, http.MethodPost, connURL+"/transaction/mode", map[string]any{"mode": "manual"}, tok)
	modeReq.Header.Set("X-Warden-Session", sessionID)
	modeRes := send(t, modeReq, app.routes())
	assert.Equal(t, modeRes.StatusCode, http.StatusOK)
	assert.Equal(t, modeRes.BodyFields["mode"], any("manual"))

	// Run a statement to open a transaction.
	queryReq := newAuthRequest(t, http.MethodPost, connURL+"/query", map[string]any{"sql": "CREATE TABLE t (id INTEGER)"}, tok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	queryRes := send(t, queryReq, app.routes())
	assert.Equal(t, queryRes.StatusCode, http.StatusOK)
	txField, ok := queryRes.BodyFields["transaction"].(map[string]any)
	if !ok {
		t.Fatalf("expected transaction field on query response, got %v", queryRes.BodyFields)
	}
	assert.Equal(t, txField["open"], any(true))

	// Switching back to auto while open must 409.
	autoReq := newAuthRequest(t, http.MethodPost, connURL+"/transaction/mode", map[string]any{"mode": "auto"}, tok)
	autoReq.Header.Set("X-Warden-Session", sessionID)
	autoRes := send(t, autoReq, app.routes())
	assert.Equal(t, autoRes.StatusCode, http.StatusConflict)

	// Commit.
	commitReq := newAuthRequest(t, http.MethodPost, connURL+"/transaction/commit", nil, tok)
	commitReq.Header.Set("X-Warden-Session", sessionID)
	commitRes := send(t, commitReq, app.routes())
	assert.Equal(t, commitRes.StatusCode, http.StatusOK)
	assert.Equal(t, commitRes.BodyFields["open"], any(false))

	// Status reflects closed transaction.
	statusReq := newAuthRequest(t, http.MethodGet, connURL+"/transaction", nil, tok)
	statusReq.Header.Set("X-Warden-Session", sessionID)
	statusRes := send(t, statusReq, app.routes())
	assert.Equal(t, statusRes.StatusCode, http.StatusOK)
	assert.Equal(t, statusRes.BodyFields["open"], any(false))

	// No open transaction to roll back.
	rollbackReq := newAuthRequest(t, http.MethodPost, connURL+"/transaction/rollback", nil, tok)
	rollbackReq.Header.Set("X-Warden-Session", sessionID)
	rollbackRes := send(t, rollbackReq, app.routes())
	assert.Equal(t, rollbackRes.StatusCode, http.StatusConflict)
}

func TestSetTransactionMode_ValidatesMode(t *testing.T) {
	t.Parallel()
	app, tok, connURL := setUpTransactionTestConnection(t, "tx-mode-validate")

	connectRes := send(t, newAuthRequest(t, http.MethodPost, connURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	req := newAuthRequest(t, http.MethodPost, connURL+"/transaction/mode", map[string]any{"mode": "bogus"}, tok)
	req.Header.Set("X-Warden-Session", sessionID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
}

func TestGetTransactionStatus_ExpiredSession(t *testing.T) {
	t.Parallel()
	app, tok, connURL := setUpTransactionTestConnection(t, "tx-status-expired")

	req := newAuthRequest(t, http.MethodGet, connURL+"/transaction", nil, tok)
	req.Header.Set("X-Warden-Session", "missing-session")
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusGone)
}
