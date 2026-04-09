package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestTestConnectionUnknownDriver(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-test-drv@example.com", "Conn Test Drv", "securepass99")

	// Create workspace.
	wsReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "ConnWS",
	})
	wsReq.Header.Set("Authorization", "Bearer "+tok)
	wsRes := send(t, wsReq, app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	// Test connection with unknown driver returns 422.
	req := newTestRequest(t, http.MethodPost, orgEnvConnectionsURL(slug, wsIDInt, envID)+"/test", map[string]any{
		"driver": "oracle",
		"dsn":    "some-dsn",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
}

func TestTestConnectionUnreachable(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-test-unr@example.com", "Conn Test Unr", "securepass99")

	// Create workspace.
	wsReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "ConnWS2",
	})
	wsReq.Header.Set("Authorization", "Bearer "+tok)
	wsRes := send(t, wsReq, app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	// Test connection with unreachable host returns 200 with ok:false.
	req := newTestRequest(t, http.MethodPost, orgEnvConnectionsURL(slug, wsIDInt, envID)+"/test", map[string]any{
		"driver": "postgres",
		"dsn":    "host=localhost port=19999 user=test dbname=test sslmode=disable connect_timeout=1",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["ok"], false)
}

func TestTestConnectionValidationAndSuccess(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-test-validation@example.com", "Conn Test Validation", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "ConnWS3"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	invalidRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID)+"/test",
		map[string]any{"driver": "sqlite"}, tok), app.routes())
	assert.Equal(t, invalidRes.StatusCode, http.StatusUnprocessableEntity)

	successRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID)+"/test",
		map[string]any{"driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, successRes.StatusCode, http.StatusOK)
	assert.Equal(t, successRes.BodyFields["ok"], true)
}

func TestTestConnectionRequiresConnCreate(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, ownerTok, org := seedOrgOwner(t, app, "conn-test-owner@example.com", "Conn Test Owner", "Conn Test Org")
	member, memberTok := seedAccountWithToken(t, app, "conn-test-member@example.com", "Conn Test Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	ws := seedWorkspaceForAccount(t, app, org, owner, "Conn Test WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)

	memberReq := newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID)+"/test",
		map[string]any{"driver": "sqlite", "dsn": ":memory:"}, memberTok)
	memberRes := send(t, memberReq, app.routes())
	assert.Equal(t, memberRes.StatusCode, http.StatusForbidden)

	ownerReq := newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID)+"/test",
		map[string]any{"driver": "sqlite", "dsn": ":memory:"}, ownerTok)
	ownerRes := send(t, ownerReq, app.routes())
	assert.Equal(t, ownerRes.StatusCode, http.StatusOK)
	assert.Equal(t, ownerRes.BodyFields["ok"], true)
}

func TestCreateConnectionAndGetExcludesDSN(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-create@example.com", "Conn Create", "securepass99")

	// Create workspace.
	wsReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "ConnCreateWS",
	})
	wsReq.Header.Set("Authorization", "Bearer "+tok)
	wsRes := send(t, wsReq, app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	// Create a connection.
	createReq := newTestRequest(t, http.MethodPost, orgEnvConnectionsURL(slug, wsIDInt, envID), map[string]any{
		"name":   "My Postgres",
		"driver": "postgres",
		"dsn":    "host=localhost port=5432 user=test dbname=test",
	})
	createReq.Header.Set("Authorization", "Bearer "+tok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// DSN should not appear in the create response (json:"-").
	if _, hasDSN := createRes.BodyFields["dsn"]; hasDSN {
		t.Fatal("DSN should not be present in response")
	}

	// Get the connection.
	getReq := newTestRequest(t, http.MethodGet, orgConnectionURL(slug, wsIDInt, envID, connID), nil)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getRes := send(t, getReq, app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "My Postgres")

	// DSN should not be in GET response either.
	if _, hasDSN := getRes.BodyFields["dsn"]; hasDSN {
		t.Fatal("DSN should not be present in GET response")
	}
}

func TestCreateConnectionUnknownDriverReturns422(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-create-driver@example.com", "Conn Create Driver", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Driver WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Bad Driver", "driver": "oracle", "dsn": "ignored"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, createRes, "driver")
}

func TestListConnections(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-list@example.com", "Conn List", "securepass99")

	// Create workspace.
	wsReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "ListConnWS",
	})
	wsReq.Header.Set("Authorization", "Bearer "+tok)
	wsRes := send(t, wsReq, app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	// Create two connections.
	for _, name := range []string{"conn1", "conn2"} {
		req := newTestRequest(t, http.MethodPost, orgEnvConnectionsURL(slug, wsIDInt, envID), map[string]any{
			"name":   name,
			"driver": "sqlite",
			"dsn":    ":memory:",
		})
		req.Header.Set("Authorization", "Bearer "+tok)
		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	// List connections.
	listReq := newTestRequest(t, http.MethodGet, orgEnvConnectionsURL(slug, wsIDInt, envID), nil)
	listReq.Header.Set("Authorization", "Bearer "+tok)
	listRes := send(t, listReq, app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	err := json.Unmarshal(listRes.BodyBytes, &payload)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(payload.Items), 2)
}

func TestListConnections_SupportsSearchFilterSortAndPagination(t *testing.T) {
	t.Parallel()

	app, org, ws, token := setupWorkspaceOwner(t)
	envA := seedEnvironment(t, app, ws.ID, org.ID, "prod")
	envB := seedEnvironment(t, app, ws.ID, org.ID, "staging")
	seedConnection(t, app, ws.ID, &envA.ID, org.ID, "postgres", "Primary DB", "open")
	seedConnection(t, app, ws.ID, &envB.ID, org.ID, "mysql", "Replica DB", "restricted")

	req := newOrgRequest(t, http.MethodGet,
		orgEnvConnectionsURL(org.Slug, ws.ID, envA.ID)+"?q=db&driver=postgres&sort=name&order=asc&page=1&page_size=10",
		token,
	)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)

	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 10)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"], "Primary DB")
}

func TestUpdateConnection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-update@example.com", "Conn Update", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Update WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envIDInt, _ := strconv.ParseInt(envID, 10, 64)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envIDInt),
		map[string]any{
			"name":   "Primary",
			"driver": "sqlite",
			"dsn":    ":memory:",
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(slug, wsIDInt, envIDInt, connID),
		map[string]any{
			"name":        "Primary Updated",
			"dsn":         ":memory:",
			"access_mode": "restricted",
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		orgConnectionURL(slug, wsIDInt, envIDInt, connID),
		nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"], "Primary Updated")
	assert.Equal(t, getRes.BodyFields["access_mode"], "restricted")
	assert.Equal(t, fmt.Sprintf("%v", getRes.BodyFields["environment_id"]), envID)
}

func TestUpdateConnectionRejectsImmutableFields(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-update-immutable@example.com", "Conn Update Immutable", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Immutable WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envIDInt, _ := strconv.ParseInt(envID, 10, 64)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envIDInt),
		map[string]any{
			"name":   "Primary",
			"driver": "sqlite",
			"dsn":    ":memory:",
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(slug, wsIDInt, envIDInt, connID),
		map[string]any{
			"name":   "Primary Updated",
			"driver": "postgres",
			"dsn":    ":memory:",
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusUnprocessableEntity)
}

func TestUpdateConnectionBlocksDSNChangeWhileSessionActive(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-update-active@example.com", "Conn Update Active", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Active WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Primary", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(slug, wsIDInt, envID, connID),
		map[string]any{
			"name":        "Primary Rotated",
			"dsn":         "file:new.db?mode=memory&cache=shared",
			"access_mode": "open",
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusConflict)
}

func TestUpdateConnectionForceDropsActiveSessionsOnDSNChange(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-update-force@example.com", "Conn Update Force", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Force WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Primary", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(slug, wsIDInt, envID, connID),
		map[string]any{
			"name":        "Primary Rotated",
			"dsn":         "file:new.db?mode=memory&cache=shared",
			"access_mode": "open",
			"force":       true,
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusNoContent)

	queryReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, tok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	queryRes := send(t, queryReq, app.routes())
	assert.Equal(t, queryRes.StatusCode, http.StatusGone)
}

func TestUpdateConnectionAllowsNonDSNChangesWithActiveSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-update-non-dsn@example.com", "Conn Update Non DSN", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Non DSN WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Primary", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(slug, wsIDInt, envID, connID),
		map[string]any{
			"name":        "Primary Updated",
			"dsn":         ":memory:",
			"access_mode": "restricted",
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusNoContent)
}

func TestCreateConnectionRejectsEnvironmentFromOtherWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-env-check@example.com", "Conn Env Check", "securepass99")

	ws1Res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "WS1"}, tok), app.routes())
	assert.Equal(t, ws1Res.StatusCode, http.StatusCreated)
	ws1ID := fmt.Sprintf("%v", ws1Res.BodyFields["id"])

	ws2Res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "WS2"}, tok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+ws2ID+"/environments",
		map[string]any{"name": "other-env"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	ws1IDInt, _ := strconv.ParseInt(ws1ID, 10, 64)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])
	envIDInt, _ := strconv.ParseInt(envID, 10, 64)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, ws1IDInt, envIDInt),
		map[string]any{
			"name":   "Bad Conn",
			"driver": "sqlite",
			"dsn":    ":memory:",
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusNotFound)
}

func TestExecuteQuerySessionAndValidationBranches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, ownerTok, org := seedOrgOwner(t, app, "query-owner@example.com", "Query Owner", "Query Org")
	member, memberTok := seedAccountWithToken(t, app, "query-member@example.com", "Query Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Query WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "SQLConn", "driver": "sqlite", "dsn": ":memory:"}, ownerTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	queryURL := orgConnectionURL(org.Slug, ws.ID, envID, connID) + "/query"
	connectURL := orgConnectionURL(org.Slug, ws.ID, envID, connID) + "/connect"

	validationRes := send(t, newAuthRequest(t, http.MethodPost, queryURL, map[string]any{}, ownerTok), app.routes())
	assert.Equal(t, validationRes.StatusCode, http.StatusUnprocessableEntity)

	missingSessionRes := send(t, newAuthRequest(t, http.MethodPost, queryURL, map[string]any{"sql": "SELECT 1"}, ownerTok), app.routes())
	assert.Equal(t, missingSessionRes.StatusCode, http.StatusBadRequest)

	expiredSessionReq := newAuthRequest(t, http.MethodPost, queryURL, map[string]any{"sql": "SELECT 1"}, ownerTok)
	expiredSessionReq.Header.Set("X-Warden-Session", "missing-session")
	expiredSessionRes := send(t, expiredSessionReq, app.routes())
	assert.Equal(t, expiredSessionRes.StatusCode, http.StatusGone)

	connectRes := send(t, newAuthRequest(t, http.MethodPost, connectURL, nil, ownerTok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	wrongOwnerReq := newAuthRequest(t, http.MethodPost, queryURL, map[string]any{"sql": "SELECT 1"}, memberTok)
	wrongOwnerReq.Header.Set("X-Warden-Session", sessionID)
	wrongOwnerRes := send(t, wrongOwnerReq, app.routes())
	assert.Equal(t, wrongOwnerRes.StatusCode, http.StatusForbidden)

	selectErrReq := newAuthRequest(t, http.MethodPost, queryURL, map[string]any{"sql": "SELECT * FROM missing_table"}, ownerTok)
	selectErrReq.Header.Set("X-Warden-Session", sessionID)
	selectErrRes := send(t, selectErrReq, app.routes())
	assert.Equal(t, selectErrRes.StatusCode, http.StatusUnprocessableEntity)
}

func TestExecuteQueryExecuteBranch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "query-exec@example.com", "Query Exec", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Exec WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "ExecConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	execReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/query",
		map[string]any{"sql": "CREATE TABLE t (id INTEGER)"}, tok)
	execReq.Header.Set("X-Warden-Session", sessionID)
	execRes := send(t, execReq, app.routes())
	assert.Equal(t, execRes.StatusCode, http.StatusOK)
}

func TestExecuteQueryRejectsSessionFromDifferentConnection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "query-cross-conn@example.com", "Query Cross Conn", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Cross Conn WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	connARes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Conn A", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, connARes.StatusCode, http.StatusCreated)
	connAID := fmt.Sprintf("%v", connARes.BodyFields["id"])

	connBRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Conn B", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, connBRes.StatusCode, http.StatusCreated)
	connBID := fmt.Sprintf("%v", connBRes.BodyFields["id"])

	connectARes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connAID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectARes.StatusCode, http.StatusOK)
	sessionID := connectARes.BodyFields["session_id"].(string)

	crossReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connBID)+"/query",
		map[string]any{"sql": "SELECT 1"}, tok)
	crossReq.Header.Set("X-Warden-Session", sessionID)
	crossRes := send(t, crossReq, app.routes())
	assert.Equal(t, crossRes.StatusCode, http.StatusForbidden)
}

func TestConnectionRuntimePermissionClasses(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, ownerTok, org := seedOrgOwner(t, app, "conn-perm-owner@example.com", "Conn Perm Owner", "Conn Perm Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Conn Perm WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	wsID := strconv.FormatInt(ws.ID, 10)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "PermConn", "driver": "sqlite", "dsn": "file::memory:?cache=shared"}, ownerTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	connIDInt, _ := strconv.ParseInt(connID, 10, 64)

	ownerConnectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, ownerTok), app.routes())
	assert.Equal(t, ownerConnectRes.StatusCode, http.StatusOK)
	ownerSessionID := ownerConnectRes.BodyFields["session_id"].(string)

	createTableReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "CREATE TABLE t (id INTEGER)"}, ownerTok)
	createTableReq.Header.Set("X-Warden-Session", ownerSessionID)
	assert.Equal(t, send(t, createTableReq, app.routes()).StatusCode, http.StatusOK)

	insertReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "INSERT INTO t (id) VALUES (1)"}, ownerTok)
	insertReq.Header.Set("X-Warden-Session", ownerSessionID)
	assert.Equal(t, send(t, insertReq, app.routes()).StatusCode, http.StatusOK)

	mkMember := func(email, name string, permissions ...string) (string, string) {
		member, tok := seedAccountWithToken(t, app, email, name)
		if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
			t.Fatal(err)
		}
		roleID := createRoleForTest(t, app, org.ID, nil, "connection", permissions...)
		res := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, wsID, roleID, "account", member.ID, "connection", connIDInt)
		assert.Equal(t, res.StatusCode, http.StatusNoContent)
		return strconv.FormatInt(member.ID, 10), tok
	}

	_, readTok := mkMember("conn-perm-read@example.com", "Conn Perm Read", "conn:read")
	_, dqlTok := mkMember("conn-perm-dql@example.com", "Conn Perm DQL", "conn:dql")
	_, dmlTok := mkMember("conn-perm-dml@example.com", "Conn Perm DML", "conn:dml")
	_, ddlTok := mkMember("conn-perm-ddl@example.com", "Conn Perm DDL", "conn:ddl")
	_, execTok := mkMember("conn-perm-exec@example.com", "Conn Perm Exec", "conn:execute")

	readConnectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, readTok), app.routes())
	assert.Equal(t, readConnectRes.StatusCode, http.StatusForbidden)

	connectAndSession := func(tok string) string {
		res := send(t, newAuthRequest(t, http.MethodPost,
			orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusOK)
		return res.BodyFields["session_id"].(string)
	}

	dqlSession := connectAndSession(dqlTok)
	dmlSession := connectAndSession(dmlTok)
	ddlSession := connectAndSession(ddlTok)
	execSession := connectAndSession(execTok)

	dqlSelectReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, dqlTok)
	dqlSelectReq.Header.Set("X-Warden-Session", dqlSession)
	assert.Equal(t, send(t, dqlSelectReq, app.routes()).StatusCode, http.StatusOK)

	dqlUpdateReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "UPDATE t SET id = 2"}, dqlTok)
	dqlUpdateReq.Header.Set("X-Warden-Session", dqlSession)
	assert.Equal(t, send(t, dqlUpdateReq, app.routes()).StatusCode, http.StatusForbidden)

	dmlUpdateReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "UPDATE t SET id = 3"}, dmlTok)
	dmlUpdateReq.Header.Set("X-Warden-Session", dmlSession)
	assert.Equal(t, send(t, dmlUpdateReq, app.routes()).StatusCode, http.StatusOK)

	dmlSelectReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, dmlTok)
	dmlSelectReq.Header.Set("X-Warden-Session", dmlSession)
	assert.Equal(t, send(t, dmlSelectReq, app.routes()).StatusCode, http.StatusForbidden)

	ddlCreateReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "CREATE TABLE ddl_only (id INTEGER)"}, ddlTok)
	ddlCreateReq.Header.Set("X-Warden-Session", ddlSession)
	assert.Equal(t, send(t, ddlCreateReq, app.routes()).StatusCode, http.StatusOK)

	ddlSelectReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, ddlTok)
	ddlSelectReq.Header.Set("X-Warden-Session", ddlSession)
	assert.Equal(t, send(t, ddlSelectReq, app.routes()).StatusCode, http.StatusForbidden)

	execSelectReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, execTok)
	execSelectReq.Header.Set("X-Warden-Session", execSession)
	assert.Equal(t, send(t, execSelectReq, app.routes()).StatusCode, http.StatusOK)

	execDDLReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "CREATE TABLE exec_only (id INTEGER)"}, execTok)
	execDDLReq.Header.Set("X-Warden-Session", execSession)
	assert.Equal(t, send(t, execDDLReq, app.routes()).StatusCode, http.StatusOK)
}

func TestConnectToDatabaseReturns422ForTargetDatabaseError(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-connect-fail@example.com", "Conn Connect Fail", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Fail WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{
			"name":   "Broken Postgres",
			"driver": "postgres",
			"dsn":    "host=localhost port=19999 user=test dbname=test sslmode=disable connect_timeout=1",
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusUnprocessableEntity)
}
