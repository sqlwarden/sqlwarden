package web

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func setupQueryHistoryTestConnection(t *testing.T, app *application) (database.Organization, database.Connection, string, string) {
	t.Helper()

	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "backend"
	})

	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "query-history"), "Query History Owner", "Query History Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Query History WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Query History Conn", "open")

	return org, conn, tok, orgConnectionURL(org.Slug, ws.ID, envID, fmt.Sprintf("%d", conn.ID))
}

func TestCreateAndListQueryHistoryEntry(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, tok, baseURL := setupQueryHistoryTestConnection(t, app)

	createRes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
		"sql_text":      "select * from widgets",
		"status":        "ok",
		"duration_ms":   12,
		"rows_affected": 3,
	}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["sql_text"], "select * from widgets")

	listRes := send(t, newAuthRequest(t, http.MethodGet, baseURL+"/history", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	items := listRes.BodyFields["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
}

func TestListQueryHistorySearchFiltersBySQLText(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, tok, baseURL := setupQueryHistoryTestConnection(t, app)

	for _, sql := range []string{"select * from accounts", "select * from widgets"} {
		res := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
			"sql_text": sql,
			"status":   "ok",
		}, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	listRes := send(t, newAuthRequest(t, http.MethodGet, baseURL+"/history?q=widgets", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	items := listRes.BodyFields["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 matching history item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	assert.Equal(t, first["sql_text"], "select * from widgets")
}

func TestCreateQueryHistoryEntryRejectedWhenModeNotBackend(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, tok, baseURL := setupQueryHistoryTestConnection(t, app)

	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "off"
	})

	res := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
		"sql_text": "select 1",
		"status":   "ok",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
}

func TestCreateQueryHistoryEntryValidatesInput(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, tok, baseURL := setupQueryHistoryTestConnection(t, app)

	res := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
		"sql_text": "",
		"status":   "bogus",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	fieldErrors := res.BodyFields["error"].(map[string]any)["field_errors"].(map[string]any)
	if _, ok := fieldErrors["sql_text"]; !ok {
		t.Fatalf("expected sql_text field error, got %+v", fieldErrors)
	}
	if _, ok := fieldErrors["status"]; !ok {
		t.Fatalf("expected status field error, got %+v", fieldErrors)
	}
}

func TestListWorkspaceQueryHistory_AcrossConnectionsAndFilter(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	org, conn, tok, baseURL := setupQueryHistoryTestConnection(t, app)
	envID := defaultEnvironmentID(t, app, conn.WorkspaceID)
	otherConn := seedConnection(t, app, conn.WorkspaceID, &envID, org.ID, "sqlite", "Query History Conn 2", "open")
	otherConnURL := orgConnectionURL(org.Slug, conn.WorkspaceID, envID, fmt.Sprintf("%d", otherConn.ID))
	workspaceHistoryURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/history", org.Slug, conn.WorkspaceID)

	assert.Equal(t, send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
		"sql_text": "select 1",
		"status":   "ok",
	}, tok), app.routes()).StatusCode, http.StatusCreated)
	assert.Equal(t, send(t, newAuthRequest(t, http.MethodPost, otherConnURL+"/history", map[string]any{
		"sql_text": "select 2",
		"status":   "ok",
	}, tok), app.routes()).StatusCode, http.StatusCreated)

	allRes := send(t, newAuthRequest(t, http.MethodGet, workspaceHistoryURL, nil, tok), app.routes())
	assert.Equal(t, allRes.StatusCode, http.StatusOK)
	assert.Equal(t, allRes.BodyFields["total"].(float64), float64(2))

	filteredRes := send(t, newAuthRequest(t, http.MethodGet, fmt.Sprintf("%s?connection_id=%d", workspaceHistoryURL, otherConn.ID), nil, tok), app.routes())
	assert.Equal(t, filteredRes.StatusCode, http.StatusOK)
	items := filteredRes.BodyFields["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item filtered to connection %d, got %d", otherConn.ID, len(items))
	}
	first := items[0].(map[string]any)
	assert.Equal(t, first["sql_text"], "select 2")
}

func TestListWorkspaceQueryHistory_EmptyResultReturnsEmptyItemsArray(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	org, conn, tok, _ := setupQueryHistoryTestConnection(t, app)
	workspaceHistoryURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/history", org.Slug, conn.WorkspaceID)

	res := send(t, newAuthRequest(t, http.MethodGet, workspaceHistoryURL, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	items, ok := res.BodyFields["items"].([]any)
	if !ok {
		t.Fatalf("expected items to decode as an array, got %#v", res.BodyFields["items"])
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestDeleteQueryHistoryEntry(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, tok, baseURL := setupQueryHistoryTestConnection(t, app)

	createRes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
		"sql_text": "select 1",
		"status":   "ok",
	}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	id := fmt.Sprintf("%v", createRes.BodyFields["id"])

	delRes := send(t, newAuthRequest(t, http.MethodDelete, baseURL+"/history/"+id, nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	delAgainRes := send(t, newAuthRequest(t, http.MethodDelete, baseURL+"/history/"+id, nil, tok), app.routes())
	assert.Equal(t, delAgainRes.StatusCode, http.StatusNotFound)
}

func TestClearQueryHistoryForConnection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, tok, baseURL := setupQueryHistoryTestConnection(t, app)

	for range 3 {
		res := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/history", map[string]any{
			"sql_text": "select 1",
			"status":   "ok",
		}, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	clearRes := send(t, newAuthRequest(t, http.MethodDelete, baseURL+"/history", nil, tok), app.routes())
	assert.Equal(t, clearRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, baseURL+"/history", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, listRes.BodyFields["total"].(float64), float64(0))
}
