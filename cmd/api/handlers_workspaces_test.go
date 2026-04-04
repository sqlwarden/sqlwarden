package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestCreateAndListWorkspaces(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "ws-owner@example.com", "WS Owner", "securepass99")

	// Create a workspace.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name":        "Production",
		"description": "Prod workspace",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)

	// List workspaces.
	req2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	err := json.Unmarshal(res2.BodyBytes, &payload)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 25)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"].(string), "Production")
}

func TestListWorkspaces_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "workspace-list-owner"), "Workspace Owner", "securepass99")

	for _, workspace := range []map[string]any{
		{"name": "Data Lake", "description": "Lake"},
		{"name": "Analytics", "description": "BI"},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", workspace, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces?q=data&name=Data%20Lake&sort=name&order=asc&page=1&page_size=1", tok), app.routes())
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
	assert.Equal(t, payload.Items[0]["name"], "Data Lake")
}

func TestGetAndDeleteWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "ws-crud@example.com", "WS CRUD", "securepass99")

	// Create.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "Staging",
	})
	createReq.Header.Set("Authorization", "Bearer "+tok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Get.
	getReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, nil)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getRes := send(t, getReq, app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "Staging")

	// Delete.
	delReq := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, nil)
	delReq.Header.Set("Authorization", "Bearer "+tok)
	delRes := send(t, delReq, app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Get returns 404 after deletion.
	getReq2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, nil)
	getReq2.Header.Set("Authorization", "Bearer "+tok)
	getRes2 := send(t, getReq2, app.routes())
	assert.Equal(t, getRes2.StatusCode, http.StatusNotFound)
}

func TestWorkspaceUpdatePermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, ownerTok, slug := registerAndLogin(t, app, "ws-upd@example.com", "WS Upd", "securepass99")

	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "UpdateMe",
	})
	createReq.Header.Set("Authorization", "Bearer "+ownerTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Owner can update (owner has ws:write via builtin role).
	newName := "UpdatedName"
	patchReq := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, map[string]any{
		"name": newName,
	})
	patchReq.Header.Set("Authorization", "Bearer "+ownerTok)
	patchRes := send(t, patchReq, app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusNoContent)
}
