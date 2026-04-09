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

func TestGetMeStillWorksWhenPersonalSpacesDisabled(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.config.personalSpacesEnabled = false
	_, tok := setupMeTest(t, app, "me-disabled@example.com")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/me", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["email"].(string), "me-disabled@example.com")
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

func TestCreateMyWorkspaceValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "mews-validation@example.com")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"description": "missing name"}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
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

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(res.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 25)
	assert.Equal(t, payload.Total, 2)
	assert.Equal(t, len(payload.Items), 2)
}

func TestListMyWorkspaces_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "melist-contract@example.com")

	for _, body := range []map[string]any{
		{"name": "Alpha Space", "description": "A"},
		{"name": "Zulu Space", "description": "Z"},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces", body, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/me/workspaces?q=space&name=Zulu%20Space&sort=name&order=desc&page=1&page_size=1", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"], "Zulu Space")
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

func TestMyWorkspaceRoutesReturn404WhenPersonalSpacesDisabledByConfig(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.config.personalSpacesEnabled = false
	_, tok := setupMeTest(t, app, "me-gated@example.com")

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/me/workspaces", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusNotFound)

	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Blocked"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusNotFound)
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

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	for _, ws := range payload.Items {
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
	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Total, 2)

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

func TestListMyEnvironments_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meenv-list@example.com")

	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Env List WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	for _, name := range []string{"staging", "dev", "prod"} {
		res := send(t, newAuthRequest(t, http.MethodPost, meWsURL(wsID)+"/environments",
			map[string]any{"name": name}, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newAuthRequest(t, http.MethodGet,
		meWsURL(wsID)+"/environments?q=pro&name=prod&sort=name&order=asc&page=1&page_size=1", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"], "prod")
}

func TestMyWorkspaceEnvironmentValidationAndIsolation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meenv-validation@example.com")

	ws1Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Env WS 1"}, tok), app.routes())
	assert.Equal(t, ws1Res.StatusCode, http.StatusCreated)
	ws1ID := fmt.Sprintf("%v", ws1Res.BodyFields["id"])

	ws2Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Env WS 2"}, tok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	badCreate := send(t, newAuthRequest(t, http.MethodPost, meWsURL(ws1ID)+"/environments",
		map[string]any{"description": "missing name"}, tok), app.routes())
	assert.Equal(t, badCreate.StatusCode, http.StatusUnprocessableEntity)

	envRes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(ws1ID)+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	crossGet := send(t, newAuthRequest(t, http.MethodGet, meWsURL(ws2ID)+"/environments/"+envID, nil, tok), app.routes())
	assert.Equal(t, crossGet.StatusCode, http.StatusNotFound)

	badUpdate := send(t, newAuthRequest(t, http.MethodPatch, meWsURL(ws1ID)+"/environments/"+envID,
		map[string]any{"description": "missing name"}, tok), app.routes())
	assert.Equal(t, badUpdate.StatusCode, http.StatusUnprocessableEntity)
}

func TestOrgEnvironmentNotAccessibleViaMeRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, orgSlug := registerAndLogin(t, app, "org-env-via-me@example.com", "OrgEnvUser", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "Org WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodGet, meWsURL(wsID)+"/environments/"+envID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
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
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := strconv.FormatInt(defaultEnvironmentID(t, app, wsIDInt), 10)
	connsURL := meEnvConnectionsURL(wsID, envID)

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
	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Total, 1)

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

func TestListMyConnections_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meconn-list@example.com")

	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Conn List WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envARes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(wsID)+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envARes.StatusCode, http.StatusCreated)
	envAID := fmt.Sprintf("%v", envARes.BodyFields["id"])

	envBRes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(wsID)+"/environments",
		map[string]any{"name": "staging"}, tok), app.routes())
	assert.Equal(t, envBRes.StatusCode, http.StatusCreated)

	for _, tc := range []struct {
		envID string
		body  map[string]any
	}{
		{envID: envAID, body: map[string]any{"name": "Primary DB", "driver": "sqlite", "dsn": "file::memory:?cache=shared"}},
		{envID: fmt.Sprintf("%v", envBRes.BodyFields["id"]), body: map[string]any{"name": "Replica DB", "driver": "sqlite", "dsn": "file::memory:?cache=shared", "access_mode": "restricted"}},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(wsID, tc.envID), tc.body, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newAuthRequest(t, http.MethodGet,
		meEnvConnectionsURL(wsID, envAID)+"?q=db&driver=sqlite&sort=name&order=asc&page=1&page_size=1", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"], "Primary DB")
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
	ws1IDInt, _ := strconv.ParseInt(ws1ID, 10, 64)
	defaultEnv1 := strconv.FormatInt(defaultEnvironmentID(t, app, ws1IDInt), 10)
	ws2IDInt, _ := strconv.ParseInt(ws2ID, 10, 64)
	defaultEnv2 := strconv.FormatInt(defaultEnvironmentID(t, app, ws2IDInt), 10)

	// Create a connection in WS A.
	connRes := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(ws1ID, defaultEnv1), map[string]any{
		"name":   "Conn A",
		"driver": "sqlite",
		"dsn":    "file::memory:",
	}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	// Try to access it through WS B — should 404.
	res := send(t, newAuthRequest(t, http.MethodGet,
		meEnvConnectionsURL(ws2ID, defaultEnv2)+"/"+connID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestMyWorkspaceConnectionValidationAndEnvironmentChecks(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := setupMeTest(t, app, "meconn-validation@example.com")

	ws1Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Conn WS 1"}, tok), app.routes())
	assert.Equal(t, ws1Res.StatusCode, http.StatusCreated)
	ws1ID := fmt.Sprintf("%v", ws1Res.BodyFields["id"])

	ws2Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Conn WS 2"}, tok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])
	ws1IDInt, _ := strconv.ParseInt(ws1ID, 10, 64)
	defaultEnv1 := strconv.FormatInt(defaultEnvironmentID(t, app, ws1IDInt), 10)

	badCreate := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(ws1ID, defaultEnv1),
		map[string]any{"name": "Bad Conn", "driver": "sqlite"}, tok), app.routes())
	assert.Equal(t, badCreate.StatusCode, http.StatusUnprocessableEntity)

	envRes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(ws2ID)+"/environments",
		map[string]any{"name": "other-env"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)

	crossEnvCreate := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(ws1ID, fmt.Sprintf("%v", envRes.BodyFields["id"])), map[string]any{
		"name":   "Cross Env Conn",
		"driver": "sqlite",
		"dsn":    ":memory:",
	}, tok), app.routes())
	assert.Equal(t, crossEnvCreate.StatusCode, http.StatusNotFound)
}

func TestOrgConnectionNotAccessibleViaMeRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, orgSlug := registerAndLogin(t, app, "org-conn-via-me@example.com", "OrgConnUser", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "Org WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := strconv.FormatInt(defaultEnvironmentID(t, app, wsIDInt), 10)

	connRes := send(t, newAuthRequest(t, http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%s/environments/%s/connections", orgSlug, wsID, envID),
		map[string]any{"name": "Org DB", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodGet, meEnvConnectionsURL(wsID, envID)+"/"+connID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestPersonalEnvironmentNotAccessibleViaOrgRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, orgSlug := registerAndLogin(t, app, "space-env-via-org@example.com", "SpaceEnvUser", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Personal WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost, meWsURL(wsID)+"/environments",
		map[string]any{"name": "personal-env"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestPersonalConnectionNotAccessibleViaOrgRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, orgSlug := registerAndLogin(t, app, "space-conn-via-org@example.com", "SpaceConnUser", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Personal WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := strconv.FormatInt(defaultEnvironmentID(t, app, wsIDInt), 10)

	connRes := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(wsID, envID),
		map[string]any{"name": "Personal DB", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID+"/connections/"+connID, nil, tok), app.routes())
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
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := strconv.FormatInt(defaultEnvironmentID(t, app, wsIDInt), 10)

	connRes := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(wsID, envID), map[string]any{
		"name":   "MemDB",
		"driver": "sqlite",
		"dsn":    "file::memory:?cache=shared",
	}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	// Connect.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		meEnvConnectionsURL(wsID, envID)+"/"+connID+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID, ok := connectRes.BodyFields["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatal("expected non-empty session_id")
	}

	// Query.
	queryReq := newAuthRequest(t, http.MethodPost,
		meEnvConnectionsURL(wsID, envID)+"/"+connID+"/query",
		map[string]any{"sql": "SELECT 1"}, tok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	queryRes := send(t, queryReq, app.routes())
	assert.Equal(t, queryRes.StatusCode, http.StatusOK)
}

func TestDisablingPersonalSpacesDropsSessionsAndGatesRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	_, tok := setupMeTest(t, app, "me-runtime-disable@example.com")

	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/me/workspaces",
		map[string]any{"name": "Query WS"}, tok), app.routes())
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := strconv.FormatInt(defaultEnvironmentID(t, app, wsIDInt), 10)

	connRes := send(t, newAuthRequest(t, http.MethodPost, meEnvConnectionsURL(wsID, envID), map[string]any{
		"name":   "MemDB",
		"driver": "sqlite",
		"dsn":    "file::memory:?cache=shared",
	}, tok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		meEnvConnectionsURL(wsID, envID)+"/"+connID+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)
	if sessionID == "" {
		t.Fatal("expected session id")
	}
	assert.Equal(t, app.connManager.CountForConnection(connID), 1)

	disableRes := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings",
		map[string]any{"personal_spaces_enabled": false}, adminTok), app.routes())
	assert.Equal(t, disableRes.StatusCode, http.StatusOK)
	assert.Equal(t, app.connManager.CountForConnection(connID), 0)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/me/workspaces", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusNotFound)

	queryReq := newAuthRequest(t, http.MethodPost,
		meEnvConnectionsURL(wsID, envID)+"/"+connID+"/query",
		map[string]any{"sql": "SELECT 1"}, tok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	queryRes := send(t, queryReq, app.routes())
	assert.Equal(t, queryRes.StatusCode, http.StatusNotFound)
}
