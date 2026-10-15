package web

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func setupQueryFavoritesTestWorkspace(t *testing.T, app *application) (string, string) {
	t.Helper()

	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryFavoritesMode = "backend"
	})

	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "query-favorites"), "Query Favorites Owner", "Query Favorites Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Query Favorites WS", "")

	return tok, fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites", org.Slug, ws.ID)
}

func TestCreateAndListQueryFavorite(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tok, baseURL := setupQueryFavoritesTestWorkspace(t, app)

	createRes := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{
		"name":     "Top customers",
		"sql_text": "select * from customers limit 10",
	}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["name"], "Top customers")

	listRes := send(t, newAuthRequest(t, http.MethodGet, baseURL, nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	items := listRes.BodyFields["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 favorite, got %d", len(items))
	}
}

func TestListQueryFavoritesSearchFiltersByNameOrSQL(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tok, baseURL := setupQueryFavoritesTestWorkspace(t, app)

	for _, fav := range []map[string]any{
		{"name": "Top customers", "sql_text": "select * from customers limit 10"},
		{"name": "Widget audit", "sql_text": "select * from widgets"},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, baseURL, fav, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	byName := send(t, newAuthRequest(t, http.MethodGet, baseURL+"?q=customers", nil, tok), app.routes())
	assert.Equal(t, byName.StatusCode, http.StatusOK)
	items := byName.BodyFields["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 favorite matching name, got %d", len(items))
	}

	bySQL := send(t, newAuthRequest(t, http.MethodGet, baseURL+"?q=widgets", nil, tok), app.routes())
	assert.Equal(t, bySQL.StatusCode, http.StatusOK)
	items = bySQL.BodyFields["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 favorite matching sql_text, got %d", len(items))
	}
}

func TestCreateQueryFavoriteRejectedWhenModeNotBackend(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tok, baseURL := setupQueryFavoritesTestWorkspace(t, app)

	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryFavoritesMode = "off"
	})

	res := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{
		"name":     "Top customers",
		"sql_text": "select 1",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
}

func TestCreateQueryFavoriteValidatesInput(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tok, baseURL := setupQueryFavoritesTestWorkspace(t, app)

	res := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{
		"name":     "",
		"sql_text": "",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	fieldErrors := res.BodyFields["error"].(map[string]any)["field_errors"].(map[string]any)
	if _, ok := fieldErrors["name"]; !ok {
		t.Fatalf("expected name field error, got %+v", fieldErrors)
	}
	if _, ok := fieldErrors["sql_text"]; !ok {
		t.Fatalf("expected sql_text field error, got %+v", fieldErrors)
	}
}

func TestQueryFavoriteConnectionMustBelongToWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryFavoritesMode = "backend"
	})
	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "query-favorites-conn"), "Favorite Conn Owner", "Favorite Conn Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Favorite WS", "")
	otherWS := seedWorkspaceForAccount(t, app, org, account, "Other Favorite WS", "")
	ownConn := createPostgresConnectionWithPassword(t, app, org.Slug, ws.ID, defaultEnvironmentID(t, app, ws.ID), tok, "pw")
	foreignConn := createPostgresConnectionWithPassword(t, app, org.Slug, otherWS.ID, defaultEnvironmentID(t, app, otherWS.ID), tok, "pw")
	baseURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites", org.Slug, ws.ID)
	toID := func(s string) int64 {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	rejected := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{
		"name": "Foreign", "sql_text": "select 1", "connection_id": toID(foreignConn),
	}, tok), app.routes())
	assert.Equal(t, rejected.StatusCode, http.StatusUnprocessableEntity)
	assert.Equal(t, rejected.ErrorCode(), "validation_failed")
	fieldErrors := rejected.BodyFields["error"].(map[string]any)["field_errors"].(map[string]any)
	assert.Equal(t, fieldErrors["connection_id"], "Connection was not found in this workspace.")

	accepted := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{
		"name": "Own", "sql_text": "select 1", "connection_id": toID(ownConn),
	}, tok), app.routes())
	assert.Equal(t, accepted.StatusCode, http.StatusCreated)
}

func TestUpdateAndDeleteQueryFavorite(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tok, baseURL := setupQueryFavoritesTestWorkspace(t, app)

	createRes := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{
		"name":     "Top customers",
		"sql_text": "select * from customers limit 10",
	}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	id := fmt.Sprintf("%v", createRes.BodyFields["id"])

	updateRes := send(t, newAuthRequest(t, http.MethodPatch, baseURL+"/"+id, map[string]any{
		"name":     "Top 10 customers",
		"sql_text": "select * from customers limit 10",
	}, tok), app.routes())
	assert.Equal(t, updateRes.StatusCode, http.StatusOK)
	assert.Equal(t, updateRes.BodyFields["name"], "Top 10 customers")

	deleteRes := send(t, newAuthRequest(t, http.MethodDelete, baseURL+"/"+id, nil, tok), app.routes())
	assert.Equal(t, deleteRes.StatusCode, http.StatusNoContent)

	deleteAgainRes := send(t, newAuthRequest(t, http.MethodDelete, baseURL+"/"+id, nil, tok), app.routes())
	assert.Equal(t, deleteAgainRes.StatusCode, http.StatusNotFound)
}
