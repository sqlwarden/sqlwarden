package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

// setupMeTest seeds an authenticated account and returns the access token and account ID.
func setupMeTest(t *testing.T, app *application, email string) (accountID string, tok string) {
	t.Helper()
	account, tok := seedAccountWithToken(t, app, email, "Me User")
	return strconv.FormatInt(account.ID, 10), tok
}

// meWsURL returns the URL for a personal workspace.
func meWsURL(wsID string) string { return "/api/v1/me/workspaces/" + wsID }

// ── GET /me ─────────────────────────────────────────────────────────────────

func TestGetMe(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "me@example.com")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/me", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["email"].(string), "me@example.com")

	// Unauthenticated returns 401.
	res2 := send(t, newTestRequest(t, http.MethodGet, "/api/v1/me", nil), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnauthorized)
}

// ── Workspace CRUD ───────────────────────────────────────────────────────────

func TestCreateMyWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "mews@example.com")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "My WS", "description": "personal"}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	assert.Equal(t, res.BodyFields["name"].(string), "My WS")
	assert.Equal(t, res.BodyFields["owner_type"].(string), "space")
}

func TestCreateMyWorkspaceDuplicateNameReturns422(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "medup@example.com")

	body := map[string]any{"name": "Dupe WS"}
	res1 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces", body, tok), app.routes())
	assert.Equal(t, res1.StatusCode, http.StatusCreated)

	res2 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces", body, tok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestListMyWorkspaces(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "melist@example.com")

	// Create two workspaces.
	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces", map[string]any{"name": "WS1"}, tok), app.routes())
	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces", map[string]any{"name": "WS2"}, tok), app.routes())

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/me/workspaces", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var wss []map[string]any
	if err := json.Unmarshal(res.BodyBytes, &wss); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(wss), 2)
}

func TestGetMyWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meget@example.com")

	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "GetMe WS"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodGet, meWsURL(wsID), nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["name"].(string), "GetMe WS")
}

func TestUpdateMyWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meupd@example.com")

	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Old Name"}, tok), app.routes())
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	patchRes := send(t, newAuthRequest(t, http.MethodPatch, meWsURL(wsID),
		map[string]any{"name": "New Name"}, tok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet, meWsURL(wsID), nil, tok), app.routes())
	assert.Equal(t, getRes.BodyFields["name"].(string), "New Name")
}

func TestDeleteMyWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "medel@example.com")

	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Del WS"}, tok), app.routes())
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	delRes := send(t, newAuthRequest(t, http.MethodDelete, meWsURL(wsID), nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet, meWsURL(wsID), nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusNotFound)
}

// ── Workspace isolation ──────────────────────────────────────────────────────

func TestMyWorkspaceOwnedByAnotherAccountReturns404(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok1 := setupMeTest(t, app, "owner1@example.com")

	// Create second user.
	_, tok2 := setupMeTest(t, app, "owner2@example.com")

	// Owner1 creates a workspace.
	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Owner1 WS"}, tok1), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Owner2 cannot access it.
	res := send(t, newAuthRequest(t, http.MethodGet, meWsURL(wsID), nil, tok2), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestOrgWorkspaceNotAccessibleViaMeRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, orgSlug := registerAndLogin(t, app, "orguser@example.com", "OrgUser", "securepass99")

	// Create an org workspace.
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "Org WS"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Org workspace should not be accessible via /me routes.
	res := send(t, newAuthRequest(t, http.MethodGet, meWsURL(wsID), nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestPersonalWorkspaceNotVisibleInOrgWorkspaceList(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, orgSlug := registerAndLogin(t, app, "orgpersonal@example.com", "OrgPersonal", "securepass99")

	// Create a personal workspace.
	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Personal WS"}, tok), app.routes())

	// It should not appear in org workspace list.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var wss []map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &wss); err != nil {
		t.Fatal(err)
	}
	for _, ws := range wss {
		if ws["name"].(string) == "Personal WS" {
			t.Fatal("personal workspace should not appear in org workspace list")
		}
	}
}

// ── Environment CRUD ─────────────────────────────────────────────────────────

func TestMyWorkspaceEnvironmentCRUD(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meenv@example.com")

	// Create workspace.
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Env WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	envsURL := meWsURL(wsID) + "/environments"

	// Create environment.
	createRes := send(t, newAuthRequest(t, http.MethodPost, envsURL,
		map[string]any{"name": "staging", "description": "Staging env"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["name"].(string), "staging")
	envID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// List environments.
	listRes := send(t, newAuthRequest(t, http.MethodGet, envsURL, nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	var envs []map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &envs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(envs), 1)

	// Get environment.
	getRes := send(t, newAuthRequest(t, http.MethodGet, envsURL+"/"+envID, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "staging")

	// Update environment.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch, envsURL+"/"+envID,
		map[string]any{"name": "production"}, tok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusNoContent)

	// Delete environment.
	delRes := send(t, newAuthRequest(t, http.MethodDelete, envsURL+"/"+envID, nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Gone after delete.
	getAfterDel := send(t, newAuthRequest(t, http.MethodGet, envsURL+"/"+envID, nil, tok), app.routes())
	assert.Equal(t, getAfterDel.StatusCode, http.StatusNotFound)
}

// ── Connection CRUD ──────────────────────────────────────────────────────────

func TestMyWorkspaceConnectionCRUD(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meconn@example.com")

	// Create workspace.
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Conn WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	connsURL := meWsURL(wsID) + "/connections"

	// Create connection.
	createRes := send(t, newAuthRequest(t, http.MethodPost, connsURL, map[string]any{
		"name":   "My DB",
		"driver": "sqlite",
		"dsn":    "file::memory:?cache=shared",
	}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["name"].(string), "My DB")
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// List connections.
	listRes := send(t, newAuthRequest(t, http.MethodGet, connsURL, nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	var conns []map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &conns); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(conns), 1)

	// Get connection.
	getRes := send(t, newAuthRequest(t, http.MethodGet, connsURL+"/"+connID, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "My DB")

	// Delete connection.
	delRes := send(t, newAuthRequest(t, http.MethodDelete, connsURL+"/"+connID, nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Gone after delete.
	getAfterDel := send(t, newAuthRequest(t, http.MethodGet, connsURL+"/"+connID, nil, tok), app.routes())
	assert.Equal(t, getAfterDel.StatusCode, http.StatusNotFound)
}

func TestMyWorkspaceConnectionFromOtherWorkspaceReturns404(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "mecross@example.com")

	// Create two workspaces.
	ws1Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "WS A"}, tok), app.routes())
	ws1ID := fmt.Sprintf("%v", ws1Res.BodyFields["id"])

	ws2Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "WS B"}, tok), app.routes())
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	// Create a connection in WS A.
	connRes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(ws1ID)+"/connections", map[string]any{
		"name":   "Conn A",
		"driver": "sqlite",
		"dsn":    "file::memory:",
	}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	// Try to access it through WS B — should 404.
	res := send(t, newAuthRequest(t, http.MethodGet,
		meWsURL(ws2ID)+"/connections/"+connID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

// ── Connect and Query ────────────────────────────────────────────────────────

func TestMyWorkspaceConnectAndQuery(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "mequery@example.com")

	// Create workspace and connection to in-memory SQLite.
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Query WS"}, tok), app.routes())
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	connRes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(wsID)+"/connections", map[string]any{
		"name":   "MemDB",
		"driver": "sqlite",
		"dsn":    "file::memory:?cache=shared",
	}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	// Connect.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		meWsURL(wsID)+"/connections/"+connID+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID, ok := connectRes.BodyFields["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatal("expected non-empty session_id")
	}

	// Query.
	queryReq := newAuthRequest(t, http.MethodPost,
		meWsURL(wsID)+"/connections/"+connID+"/query",
		map[string]any{"sql": "SELECT 1"}, tok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	queryRes := send(t, queryReq, app.routes())
	assert.Equal(t, queryRes.StatusCode, http.StatusOK)
}
