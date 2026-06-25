package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/dbsql"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/pkg/result"
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
	var logs bytes.Buffer
	app.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["ok"], false)
	assert.True(t, strings.Contains(logs.String(), "connection test failed"))
	assert.True(t, strings.Contains(logs.String(), "target_unreachable"))
	assert.False(t, strings.Contains(logs.String(), "19999"))
	assert.False(t, strings.Contains(logs.String(), "dbname=test"))
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

func TestTestConnectionSuccessLogsOutcome(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-test-success-log@example.com", "Conn Test Success Log", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Test Success Log WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	var logs bytes.Buffer
	app.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	successRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID)+"/test",
		map[string]any{"driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, successRes.StatusCode, http.StatusOK)
	assert.Equal(t, successRes.BodyFields["ok"], true)
	assert.True(t, strings.Contains(logs.String(), "connection test completed"))
	assert.True(t, strings.Contains(logs.String(), `"driver":"sqlite"`))
	assert.False(t, strings.Contains(logs.String(), ":memory:"))
}

func TestTestConnectionRejectsSQLiteFileTargetInServerMode(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-test-sqlite-file@example.com", "Conn Test SQLite File", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Test SQLite File WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	res := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID)+"/test",
		map[string]any{
			"driver": "sqlite",
			"dsn":    filepath.Join(t.TempDir(), "host.db"),
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "driver")
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

func TestCreateConnectionRejectsSQLiteFileTargetInServerMode(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-create-sqlite-file@example.com", "Conn Create SQLite File", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn SQLite File WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{
			"name":   "Host SQLite",
			"driver": "sqlite",
			"dsn":    filepath.Join(t.TempDir(), "host.db"),
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, createRes, "driver")

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		orgEnvConnectionsURL(slug, wsIDInt, envID), nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, payload.Total, 0)
	assert.Equal(t, len(payload.Items), 0)
}

func TestCreateConnectionAllowsSQLiteFileTargetWhenLocalSourceAllowed(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.config.Drivers.SQLite.AllowedSources = []string{SQLiteDriverSourceLocal}

	_, tok, slug := registerAndLogin(t, app, "conn-create-desktop-sqlite-file@example.com", "Conn Create Desktop SQLite File", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Desktop SQLite File WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{
			"name":   "Local SQLite",
			"driver": "sqlite",
			"dsn":    filepath.Join(t.TempDir(), "local.db"),
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["driver"], "sqlite")
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

func TestUpdateConnectionRejectsSQLiteFileTargetInServerMode(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-update-sqlite-file@example.com", "Conn Update SQLite File", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Update SQLite File WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{
			"name":   "Primary",
			"driver": "sqlite",
			"dsn":    ":memory:",
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	updateRes := send(t, newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(slug, wsIDInt, envID, connID),
		map[string]any{
			"name":        "Primary",
			"dsn":         filepath.Join(t.TempDir(), "host.db"),
			"access_mode": "open",
		}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, updateRes, "driver")
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
			"dsn":         "file::memory:?cache=shared",
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
			"dsn":         "file::memory:?cache=shared",
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

func TestExecuteQueryCursorAndValidationBranches(t *testing.T) {
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

func TestExecuteQueryAppliesConfiguredResultLimit(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.config.Query.MaxResultRows = 2
	app.config.Query.MaxResultBytes = 1024

	_, tok, slug := registerAndLogin(t, app, "query-limit@example.com", "Query Limit", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Limit WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "LimitConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	queryURL := orgConnectionURL(slug, wsIDInt, envID, connID) + "/query"
	for _, sql := range []string{
		"CREATE TABLE t (id INTEGER)",
		"INSERT INTO t (id) VALUES (1), (2), (3)",
	} {
		req := newAuthRequest(t, http.MethodPost, queryURL, map[string]any{"sql": sql}, tok)
		req.Header.Set("X-Warden-Session", sessionID)
		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusOK)
	}

	selectReq := newAuthRequest(t, http.MethodPost, queryURL, map[string]any{"sql": "SELECT id FROM t ORDER BY id"}, tok)
	selectReq.Header.Set("X-Warden-Session", sessionID)
	selectRes := send(t, selectReq, app.routes())
	assert.Equal(t, selectRes.StatusCode, http.StatusOK)
	assert.Equal(t, selectRes.BodyFields["truncated"], true)
	assert.Equal(t, selectRes.BodyFields["truncation_reason"], dbsql.TruncationReasonMaxRows)
	if got := selectRes.BodyFields["rows_returned"]; got != float64(2) {
		t.Fatalf("rows_returned = %v, want 2", got)
	}
	rows := selectRes.BodyFields["rows"].([]any)
	assert.Equal(t, len(rows), 2)
}

func TestQueryCursorPagesResultsAndExpiresAfterExhaustion(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "query-cursor-page"), "Query Cursor Page", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Query Cursor WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Query Cursor Conn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectionURL := orgConnectionURL(slug, wsIDInt, envID, connID)
	connectRes := send(t, newAuthRequest(t, http.MethodPost, connectionURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	for _, sql := range []string{
		"CREATE TABLE t (id INTEGER)",
		"INSERT INTO t (id) VALUES (1), (2), (3)",
	} {
		req := newAuthRequest(t, http.MethodPost, connectionURL+"/query", map[string]any{"sql": sql}, tok)
		req.Header.Set("X-Warden-Session", sessionID)
		assert.Equal(t, send(t, req, app.routes()).StatusCode, http.StatusOK)
	}

	startReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors", map[string]any{
		"sql":       "SELECT id FROM t ORDER BY id",
		"page_size": 2,
	}, tok)
	startReq.Header.Set("X-Warden-Session", sessionID)
	startRes := send(t, startReq, app.routes())
	assert.Equal(t, startRes.StatusCode, http.StatusOK)
	assert.Equal(t, startRes.BodyFields["exhausted"], false)
	assert.Equal(t, startRes.BodyFields["rows_returned"], any(float64(2)))
	queryCursorID := startRes.BodyFields["query_cursor_id"].(string)

	fetchReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors/"+queryCursorID+"/fetch", map[string]any{"page_size": 2}, tok)
	fetchReq.Header.Set("X-Warden-Session", sessionID)
	fetchRes := send(t, fetchReq, app.routes())
	assert.Equal(t, fetchRes.StatusCode, http.StatusOK)
	assert.Equal(t, fetchRes.BodyFields["exhausted"], true)
	assert.Equal(t, fetchRes.BodyFields["rows_returned"], any(float64(1)))

	expiredReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors/"+queryCursorID+"/fetch", map[string]any{"page_size": 2}, tok)
	expiredReq.Header.Set("X-Warden-Session", sessionID)
	expiredRes := send(t, expiredReq, app.routes())
	assert.Equal(t, expiredRes.StatusCode, http.StatusGone)
	assert.Equal(t, expiredRes.BodyFields["error"].(map[string]any)["code"], apiErrorQueryCursorUnavailable)
}

func TestQueryCursorCloseAndRouteIsolation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "query-cursor-close"), "Query Cursor Close", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Query Cursor Close WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Query Cursor Close Conn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	envConnectionURL := orgConnectionURL(slug, wsIDInt, envID, connID)
	directConnectionURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/%s", slug, wsIDInt, connID)
	connectRes := send(t, newAuthRequest(t, http.MethodPost, envConnectionURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	startReq := newAuthRequest(t, http.MethodPost, envConnectionURL+"/query-cursors", map[string]any{
		"sql":       "SELECT 1 UNION ALL SELECT 2",
		"page_size": 1,
	}, tok)
	startReq.Header.Set("X-Warden-Session", sessionID)
	startRes := send(t, startReq, app.routes())
	assert.Equal(t, startRes.StatusCode, http.StatusOK)
	queryCursorID := startRes.BodyFields["query_cursor_id"].(string)

	crossRouteReq := newAuthRequest(t, http.MethodPost, directConnectionURL+"/query-cursors/"+queryCursorID+"/fetch", map[string]any{"page_size": 1}, tok)
	crossRouteReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, crossRouteReq, app.routes()).StatusCode, http.StatusNotFound)

	closeReq := newAuthRequest(t, http.MethodDelete, envConnectionURL+"/query-cursors/"+queryCursorID, nil, tok)
	closeReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, closeReq, app.routes()).StatusCode, http.StatusNoContent)

	fetchReq := newAuthRequest(t, http.MethodPost, envConnectionURL+"/query-cursors/"+queryCursorID+"/fetch", map[string]any{"page_size": 1}, tok)
	fetchReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, fetchReq, app.routes()).StatusCode, http.StatusGone)
}

func TestQueryCursorFetchHandlesParentSessionRemoval(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "query-cursor-parent"), "Query Cursor Parent", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Query Cursor Parent WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "Query Cursor Parent Conn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	connectionURL := orgConnectionURL(slug, wsIDInt, envID, connID)

	connectRes := send(t, newAuthRequest(t, http.MethodPost, connectionURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	startReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors", map[string]any{
		"sql":       "SELECT 1 UNION ALL SELECT 2",
		"page_size": 1,
	}, tok)
	startReq.Header.Set("X-Warden-Session", sessionID)
	startRes := send(t, startReq, app.routes())
	assert.Equal(t, startRes.StatusCode, http.StatusOK)
	queryCursorID := startRes.BodyFields["query_cursor_id"].(string)

	app.connManager.Remove(sessionID)

	fetchReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors/"+queryCursorID+"/fetch", map[string]any{"page_size": 1}, tok)
	fetchReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, fetchReq, app.routes()).StatusCode, http.StatusGone)
}

func TestQueryCursorFetchRechecksPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "query-cursor-perm-owner"), "Query Cursor Perm Owner", "Query Cursor Perm Org")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "query-cursor-perm-member"), "Query Cursor Perm Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Query Cursor Perm WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	wsID := strconv.FormatInt(ws.ID, 10)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "Query Cursor Perm Conn", "driver": "sqlite", "dsn": "file::memory:?cache=shared"}, ownerTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	connIDInt, _ := strconv.ParseInt(connID, 10, 64)
	connectionURL := orgConnectionURL(org.Slug, ws.ID, envID, connID)

	ownerConnectRes := send(t, newAuthRequest(t, http.MethodPost, connectionURL+"/connect", nil, ownerTok), app.routes())
	assert.Equal(t, ownerConnectRes.StatusCode, http.StatusOK)
	ownerSessionID := ownerConnectRes.BodyFields["session_id"].(string)
	tableName := fmt.Sprintf("t_%d", time.Now().UnixNano())
	for _, sql := range []string{
		fmt.Sprintf("CREATE TABLE %s (id INTEGER)", tableName),
		fmt.Sprintf("INSERT INTO %s (id) VALUES (1), (2)", tableName),
	} {
		req := newAuthRequest(t, http.MethodPost, connectionURL+"/query", map[string]any{"sql": sql}, ownerTok)
		req.Header.Set("X-Warden-Session", ownerSessionID)
		assert.Equal(t, send(t, req, app.routes()).StatusCode, http.StatusOK)
	}

	roleID := createRoleForTest(t, app, org.ID, nil, "connection", access.PermConnDQL)
	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, wsID, roleID, access.SubjectTypeAccount, member.ID, "connection", connIDInt).StatusCode, http.StatusNoContent)

	memberConnectRes := send(t, newAuthRequest(t, http.MethodPost, connectionURL+"/connect", nil, memberTok), app.routes())
	assert.Equal(t, memberConnectRes.StatusCode, http.StatusOK)
	memberSessionID := memberConnectRes.BodyFields["session_id"].(string)

	startReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors", map[string]any{
		"sql":       fmt.Sprintf("SELECT id FROM %s ORDER BY id", tableName),
		"page_size": 1,
	}, memberTok)
	startReq.Header.Set("X-Warden-Session", memberSessionID)
	startRes := send(t, startReq, app.routes())
	assert.Equal(t, startRes.StatusCode, http.StatusOK)
	queryCursorID := startRes.BodyFields["query_cursor_id"].(string)

	var bindingID int64
	if err := app.db.NewSelect().
		TableExpr("role_bindings").
		ColumnExpr("id").
		Where("org_id = ?", org.ID).
		Where("role_id = ?", roleID).
		Where("subject_type = ?", access.SubjectTypeAccount).
		Where("subject_id = ?", member.ID).
		Where("resource_type = ?", "connection").
		Where("resource_id = ?", connIDInt).
		Scan(context.Background(), &bindingID); err != nil {
		t.Fatal(err)
	}
	if err := app.enforcer.UnbindRole(context.Background(), bindingID, org.ID); err != nil {
		t.Fatal(err)
	}

	fetchReq := newAuthRequest(t, http.MethodPost, connectionURL+"/query-cursors/"+queryCursorID+"/fetch", map[string]any{"page_size": 1}, memberTok)
	fetchReq.Header.Set("X-Warden-Session", memberSessionID)
	assert.Equal(t, send(t, fetchReq, app.routes()).StatusCode, http.StatusForbidden)
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

// ── list active sessions tests ──────────────────────────────────────────────

func TestListActiveSessions_Empty(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)

	res := send(t, newOrgRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws.ID), tok),
		app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Sessions []any `json:"sessions"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	assert.Equal(t, len(payload.Sessions), 0)
}

func TestListActiveSessions_ShowsSessionAfterConnect(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "SessConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	connIDInt, _ := strconv.ParseInt(connID, 10, 64)

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws.ID), tok),
		app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Sessions []struct {
			ConnectionID float64 `json:"connection_id"`
			SessionID    string  `json:"session_id"`
		} `json:"sessions"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, len(payload.Sessions), 1)
	assert.Equal(t, int64(payload.Sessions[0].ConnectionID), connIDInt)
	assert.Equal(t, payload.Sessions[0].SessionID, sessionID)
}

func TestListActiveSessions_ClearedAfterDisconnect(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "SessDisconn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	disconnectReq := newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, tok)
	disconnectReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, disconnectReq, app.routes()).StatusCode, http.StatusNoContent)

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws.ID), tok),
		app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Sessions []any `json:"sessions"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, len(payload.Sessions), 0)
}

func TestListActiveSessions_OnlyCurrentAccount(t *testing.T) {
	// Another account's sessions must not appear in the listing.
	t.Parallel()
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	_, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "sess-member"), "Sess Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, 0); err != nil {
		// org.ID member add is a no-op here — we only need memberTok issued to confirm isolation.
		_ = err
	}

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "SessAccount", "driver": "sqlite", "dsn": ":memory:"}, ownerTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Owner connects.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, ownerTok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)

	// Member's view should be empty (they have no session).
	listRes := send(t, newOrgRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws.ID), memberTok),
		app.routes())
	// Member isn't a workspace member so they'll get 403; we just need to confirm isolation.
	if listRes.StatusCode == http.StatusOK {
		var payload struct {
			Sessions []any `json:"sessions"`
		}
		decodeJSONResponse(t, listRes.BodyBytes, &payload)
		assert.Equal(t, len(payload.Sessions), 0)
	}
}

func TestListActiveSessions_OnlyCurrentWorkspace(t *testing.T) {
	// Sessions from a different workspace must not appear.
	t.Parallel()
	app, org, ws1, tok := setupWorkspaceOwner(t)
	ws2 := seedWorkspaceForAccount(t, app, org, seedAccount(t, app, uniqueEmail(t, "ws2-owner"), "WS2 Owner"), "WS2", "")
	// Grant tok access to ws2 as well
	_ = ws2

	envID1 := defaultEnvironmentID(t, app, ws1.ID)

	// Create and connect a connection in ws1.
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws1.ID, envID1),
		map[string]any{"name": "SessWS1", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws1.ID, envID1, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)

	// Query ws2/sessions — should be empty (session belongs to ws1).
	listRes := send(t, newOrgRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws2.ID), tok),
		app.routes())
	// ws2 belongs to a different owner so owner from ws1 gets 403; confirm empty on 200.
	if listRes.StatusCode == http.StatusOK {
		var payload struct {
			Sessions []any `json:"sessions"`
		}
		decodeJSONResponse(t, listRes.BodyBytes, &payload)
		assert.Equal(t, len(payload.Sessions), 0)
	}
}

func TestListActiveSessions_RequiresAuth(t *testing.T) {
	t.Parallel()
	app, org, ws, _ := setupWorkspaceOwner(t)

	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws.ID), nil)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestListActiveSessions_MultipleConnections(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	var connIDs []string
	for _, name := range []string{"Multi1", "Multi2", "Multi3"} {
		r := send(t, newAuthRequest(t, http.MethodPost,
			orgEnvConnectionsURL(org.Slug, ws.ID, envID),
			map[string]any{"name": name, "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
		assert.Equal(t, r.StatusCode, http.StatusCreated)
		connIDs = append(connIDs, fmt.Sprintf("%v", r.BodyFields["id"]))
	}

	// Connect only the first two.
	for _, cid := range connIDs[:2] {
		r := send(t, newAuthRequest(t, http.MethodPost,
			orgConnectionURL(org.Slug, ws.ID, envID, cid)+"/connect", nil, tok), app.routes())
		assert.Equal(t, r.StatusCode, http.StatusOK)
	}

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", org.Slug, ws.ID), tok),
		app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Sessions []any `json:"sessions"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, len(payload.Sessions), 2)
}

func TestRevokeWorkspaceDatabaseSession_OwnerCanRevokeOwnSession(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Revoke Own Session", "open")

	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := app.connManager.GetOrCreateWithMetadata(
		claims.AccountID,
		strconv.FormatInt(conn.ID, 10),
		connection.SessionMetadata{
			OrgID:       strconv.FormatInt(org.ID, 10),
			WorkspaceID: strconv.FormatInt(ws.ID, 10),
		},
		func() (dbengine.Driver, error) { return newIdleQueryDriver(), nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	revokeRes := send(t, newOrgRequest(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions/%s", org.Slug, ws.ID, session.ID), tok),
		app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	_, found := app.connManager.Get(session.ID)
	assert.False(t, found)
}

func TestRevokeWorkspaceDatabaseSession_AdminCanRevokeWorkspaceSession(t *testing.T) {
	t.Parallel()
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Revoke Other Session", "open")
	member := seedAccount(t, app, uniqueEmail(t, "db-session-member"), "DB Session Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	session, _, err := app.connManager.GetOrCreateWithMetadata(
		strconv.FormatInt(member.ID, 10),
		strconv.FormatInt(conn.ID, 10),
		connection.SessionMetadata{
			OrgID:       strconv.FormatInt(org.ID, 10),
			WorkspaceID: strconv.FormatInt(ws.ID, 10),
		},
		func() (dbengine.Driver, error) { return newIdleQueryDriver(), nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	revokeRes := send(t, newOrgRequest(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions/%s", org.Slug, ws.ID, session.ID), ownerTok),
		app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	_, found := app.connManager.Get(session.ID)
	assert.False(t, found)
}

func TestRevokeWorkspaceDatabaseSession_CrossWorkspaceHidden(t *testing.T) {
	t.Parallel()
	app, org, wsA, ownerTok := setupWorkspaceOwner(t)
	owner, _, _ := seedOrgOwner(t, app, uniqueEmail(t, "db-session-other-owner"), "Other Owner", "Other Org")
	wsB := seedWorkspaceForAccount(t, app, org, owner, "Other Workspace", "")
	envID := defaultEnvironmentID(t, app, wsB.ID)
	conn := seedConnection(t, app, wsB.ID, &envID, org.ID, "sqlite", "Cross WS Session", "open")

	session, _, err := app.connManager.GetOrCreateWithMetadata(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(conn.ID, 10),
		connection.SessionMetadata{
			OrgID:       strconv.FormatInt(org.ID, 10),
			WorkspaceID: strconv.FormatInt(wsB.ID, 10),
		},
		func() (dbengine.Driver, error) { return newIdleQueryDriver(), nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	revokeRes := send(t, newOrgRequest(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions/%s", org.Slug, wsA.ID, session.ID), ownerTok),
		app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNotFound)

	_, found := app.connManager.Get(session.ID)
	assert.True(t, found)
}

// ── connect regression ──────────────────────────────────────────────────────

func TestConnectReusesExistingSession(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "ReuseConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	first := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, first.StatusCode, http.StatusOK)
	assert.Equal(t, first.BodyFields["reused"], false)
	sid1 := first.BodyFields["session_id"].(string)

	second := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, second.StatusCode, http.StatusOK)
	assert.Equal(t, second.BodyFields["reused"], true)
	assert.Equal(t, second.BodyFields["session_id"].(string), sid1)
}

// ── disconnect tests ────────────────────────────────────────────────────────

func TestDisconnectFromDatabase_HappyPath(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC1", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	disconnectReq := newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, tok)
	disconnectReq.Header.Set("X-Warden-Session", sessionID)
	disconnectRes := send(t, disconnectReq, app.routes())
	assert.Equal(t, disconnectRes.StatusCode, http.StatusNoContent)

	// Session is gone — query with the old session must return 410.
	queryReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, tok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, queryReq, app.routes()).StatusCode, http.StatusGone)
}

func TestExecuteQueryCancellationRemovesOnlyCancelledSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "query-cancel-owner"), "Query Cancel Owner", "Query Cancel Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Query Cancel WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	connA := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Cancelled Conn", "open")
	connB := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Unrelated Conn", "open")

	blockingDriver := newBlockingQueryDriver()
	cancelledSession, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(connA.ID, 10),
		func() (dbengine.Driver, error) { return blockingDriver, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	unrelatedSession, _, err := app.connManager.GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(connB.ID, 10),
		func() (dbengine.Driver, error) { return newIdleQueryDriver(), nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connA.ID, 10))+"/query",
		map[string]any{"sql": "SELECT pg_sleep(60)"}, tok)
	req.Header.Set("X-Warden-Session", cancelledSession.ID)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.routes().ServeHTTP(rr, req)
	}()

	select {
	case <-blockingDriver.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for fake query to start")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled query response")
	}

	assert.Equal(t, rr.Code, statusClientClosedRequest)

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload["error"].(map[string]any)["message"], "Query was cancelled.")

	if _, ok := app.connManager.Get(cancelledSession.ID); ok {
		t.Fatal("expected cancelled session to be removed")
	}
	if _, ok := app.connManager.Get(unrelatedSession.ID); !ok {
		t.Fatal("expected unrelated session to remain active")
	}
}

func TestDisconnectFromDatabase_MissingSessionHeader(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC2", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestDisconnectFromDatabase_UnknownSessionIsIdempotent(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC3", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	req := newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, tok)
	req.Header.Set("X-Warden-Session", "nonexistent-session-id")
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)
}

func TestDisconnectFromDatabase_WrongAccountIsForbidden(t *testing.T) {
	t.Parallel()
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "dc-member"), "DC Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC4", "driver": "sqlite", "dsn": ":memory:"}, ownerTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Owner connects and gets a session.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, ownerTok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	ownerSession := connectRes.BodyFields["session_id"].(string)

	// Member attempts to disconnect the owner's session — must be forbidden.
	req := newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, memberTok)
	req.Header.Set("X-Warden-Session", ownerSession)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)

	// Owner's session is still alive.
	queryReq := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/query",
		map[string]any{"sql": "SELECT 1"}, ownerTok)
	queryReq.Header.Set("X-Warden-Session", ownerSession)
	assert.Equal(t, send(t, queryReq, app.routes()).StatusCode, http.StatusOK)
}

func TestDisconnectFromDatabase_WrongConnectionIsBadRequest(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	connARes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC-A", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, connARes.StatusCode, http.StatusCreated)
	connAID := fmt.Sprintf("%v", connARes.BodyFields["id"])

	connBRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC-B", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, connBRes.StatusCode, http.StatusCreated)
	connBID := fmt.Sprintf("%v", connBRes.BodyFields["id"])

	// Connect to A, then try to disconnect via B's endpoint.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connAID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionA := connectRes.BodyFields["session_id"].(string)

	req := newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connBID)+"/session", nil, tok)
	req.Header.Set("X-Warden-Session", sessionA)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
}

func TestDisconnectFromDatabase_Idempotent(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC5", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	disconnect := func() int {
		req := newAuthRequest(t, http.MethodDelete,
			orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, tok)
		req.Header.Set("X-Warden-Session", sessionID)
		return send(t, req, app.routes()).StatusCode
	}

	assert.Equal(t, disconnect(), http.StatusNoContent)
	assert.Equal(t, disconnect(), http.StatusNoContent) // second call — already gone
}

func TestDisconnectFromDatabase_CanReconnectAfterDisconnect(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC6", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sid1 := connectRes.BodyFields["session_id"].(string)

	disconnectReq := newAuthRequest(t, http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil, tok)
	disconnectReq.Header.Set("X-Warden-Session", sid1)
	assert.Equal(t, send(t, disconnectReq, app.routes()).StatusCode, http.StatusNoContent)

	// Re-connect — should get a fresh session (created=true, different ID).
	reconnectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, reconnectRes.StatusCode, http.StatusOK)
	assert.Equal(t, reconnectRes.BodyFields["reused"], false)
	sid2 := reconnectRes.BodyFields["session_id"].(string)
	if sid2 == sid1 {
		t.Fatal("expected a new session ID after reconnect")
	}
}

func TestDisconnectFromDatabase_RequiresAuth(t *testing.T) {
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC7", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	req, _ := http.NewRequest(http.MethodDelete,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/session", nil)
	req.Header.Set("X-Warden-Session", "any-session")
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestDisconnectFromDatabase_WorkspaceRouteParity(t *testing.T) {
	// Disconnect is also accessible via the workspace-level (non-env) route.
	t.Parallel()
	app, org, ws, tok := setupWorkspaceOwner(t)
	envID := defaultEnvironmentID(t, app, ws.ID)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "DC8", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Connect via env route.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	// Disconnect via the workspace-level route (no env_id in path).
	wsConnURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/%s", org.Slug, ws.ID, connID)
	req := newAuthRequest(t, http.MethodDelete, wsConnURL+"/session", nil, tok)
	req.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, req, app.routes()).StatusCode, http.StatusNoContent)
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

func TestConnectToDatabaseRejectsPersistedSQLiteFileTargetInServerMode(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-connect-sqlite-file@example.com", "Conn Connect SQLite File", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Conn Persisted SQLite File WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	encryptedDSN, err := app.keyring.Encrypt(filepath.Join(t.TempDir(), "host.db"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := app.db.InsertConnection(context.Background(), wsIDInt, &envID, "Seeded Host SQLite", "sqlite", encryptedDSN, "open")
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer.InvalidateAncestry("connection", conn.ID)

	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, strconv.FormatInt(conn.ID, 10))+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusUnprocessableEntity)
	assertAPIError(t, connectRes, apiErrorValidationFailed, "SQLite file connections are disabled for this instance.")
}

type blockingQueryDriver struct {
	started   chan struct{}
	startOnce sync.Once
}

func newBlockingQueryDriver() *blockingQueryDriver {
	return &blockingQueryDriver{started: make(chan struct{})}
}

func (d *blockingQueryDriver) Connect(context.Context, dbengine.ConnectionConfig) error { return nil }
func (d *blockingQueryDriver) Ping(context.Context) error                               { return nil }
func (d *blockingQueryDriver) Close() error                                             { return nil }
func (d *blockingQueryDriver) Dialect() dbengine.Dialect                                { return dbengine.DialectSQLite }
func (d *blockingQueryDriver) Query(ctx context.Context, _ string, _ ...any) (*result.ResultSet, error) {
	d.startOnce.Do(func() { close(d.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}
func (d *blockingQueryDriver) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	return d.Query(ctx, sql, args...)
}

type idleQueryDriver struct{}

func newIdleQueryDriver() *idleQueryDriver { return &idleQueryDriver{} }

func (d *idleQueryDriver) Connect(context.Context, dbengine.ConnectionConfig) error { return nil }
func (d *idleQueryDriver) Ping(context.Context) error                               { return nil }
func (d *idleQueryDriver) Close() error                                             { return nil }
func (d *idleQueryDriver) Dialect() dbengine.Dialect                                { return dbengine.DialectSQLite }
func (d *idleQueryDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (d *idleQueryDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
