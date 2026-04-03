package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestTestConnectionUnknownDriver(t *testing.T) {
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

	// Test connection with unknown driver returns 422.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections/test", map[string]any{
		"driver": "oracle",
		"dsn":    "some-dsn",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
}

func TestTestConnectionUnreachable(t *testing.T) {
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

	// Test connection with unreachable host returns 200 with ok:false.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections/test", map[string]any{
		"driver": "postgres",
		"dsn":    "host=localhost port=19999 user=test dbname=test sslmode=disable connect_timeout=1",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["ok"], false)
}

func TestCreateConnectionAndGetExcludesDSN(t *testing.T) {
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

	// Create a connection.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections", map[string]any{
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
	getReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections/"+connID, nil)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getRes := send(t, getReq, app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "My Postgres")

	// DSN should not be in GET response either.
	if _, hasDSN := getRes.BodyFields["dsn"]; hasDSN {
		t.Fatal("DSN should not be present in GET response")
	}
}

func TestListConnections(t *testing.T) {
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

	// Create two connections.
	for _, name := range []string{"conn1", "conn2"} {
		req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections", map[string]any{
			"name":   name,
			"driver": "sqlite",
			"dsn":    ":memory:",
		})
		req.Header.Set("Authorization", "Bearer "+tok)
		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	// List connections.
	listReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections", nil)
	listReq.Header.Set("Authorization", "Bearer "+tok)
	listRes := send(t, listReq, app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var conns []map[string]any
	err := json.Unmarshal(listRes.BodyBytes, &conns)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(conns), 2)
}

func TestUpdateConnection(t *testing.T) {
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

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":           "Primary",
			"driver":         "sqlite",
			"dsn":            ":memory:",
			"environment_id": envRes.BodyFields["id"],
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections/"+connID,
		map[string]any{
			"name":        "Primary Updated",
			"dsn":         ":memory:",
			"access_mode": "restricted",
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections/"+connID,
		nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"], "Primary Updated")
	assert.Equal(t, getRes.BodyFields["access_mode"], "restricted")
	assert.Equal(t, fmt.Sprintf("%v", getRes.BodyFields["environment_id"]), envID)
}

func TestUpdateConnectionRejectsImmutableFields(t *testing.T) {
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

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":           "Primary",
			"driver":         "sqlite",
			"dsn":            ":memory:",
			"environment_id": envRes.BodyFields["id"],
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/connections/"+connID,
		map[string]any{
			"name":           "Primary Updated",
			"driver":         "postgres",
			"dsn":            ":memory:",
			"environment_id": envRes.BodyFields["id"],
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusUnprocessableEntity)
}

func TestCreateConnectionRejectsEnvironmentFromOtherWorkspace(t *testing.T) {
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

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+ws1ID+"/connections",
		map[string]any{
			"name":           "Bad Conn",
			"driver":         "sqlite",
			"dsn":            ":memory:",
			"environment_id": envRes.BodyFields["id"],
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusNotFound)
}
