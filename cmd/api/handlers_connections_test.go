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
