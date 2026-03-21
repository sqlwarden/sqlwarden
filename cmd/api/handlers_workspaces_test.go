package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestCreateAndListWorkspaces(t *testing.T) {
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

	var workspaces []map[string]any
	err := json.Unmarshal(res2.BodyBytes, &workspaces)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(workspaces), 1)
	assert.Equal(t, workspaces[0]["name"].(string), "Production")
}

func TestGetAndDeleteWorkspace(t *testing.T) {
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "ws-crud@example.com", "WS CRUD", "securepass99")

	// Create.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "Staging",
	})
	createReq.Header.Set("Authorization", "Bearer "+tok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := createRes.BodyFields["id"].(string)

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

func TestWorkspaceAccessGrant(t *testing.T) {
	app := newTestApp(t)

	// Owner creates an org and a workspace.
	_, ownerTok, slug := registerAndLogin(t, app, "ws-acc-owner@example.com", "WS Acc Owner", "securepass99")

	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "SecureWS",
	})
	createReq.Header.Set("Authorization", "Bearer "+ownerTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := createRes.BodyFields["id"].(string)

	// Register a member and add to org.
	memberID, memberTok, _ := registerAndLogin(t, app, "ws-acc-member@example.com", "WS Acc Member", "securepass99")

	addOrgReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "ws-acc-member@example.com",
		"role":  "member",
	})
	addOrgReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addOrgRes := send(t, addOrgReq, app.routes())
	assert.Equal(t, addOrgRes.StatusCode, http.StatusNoContent)

	// Member without grant cannot access workspace (gets empty list since listWorkspaces filters).
	listReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces", nil)
	listReq.Header.Set("Authorization", "Bearer "+memberTok)
	listRes := send(t, listReq, app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var visible []map[string]any
	err := json.Unmarshal(listRes.BodyBytes, &visible)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(visible), 0)

	// Grant connect access to member.
	grantReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/access", map[string]any{
		"subject": "account:" + memberID,
		"action":  "connect",
	})
	grantReq.Header.Set("Authorization", "Bearer "+ownerTok)
	grantRes := send(t, grantReq, app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusCreated)

	// Now member can see workspace in list.
	listReq2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces", nil)
	listReq2.Header.Set("Authorization", "Bearer "+memberTok)
	listRes2 := send(t, listReq2, app.routes())
	assert.Equal(t, listRes2.StatusCode, http.StatusOK)

	var visible2 []map[string]any
	err = json.Unmarshal(listRes2.BodyBytes, &visible2)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(visible2), 1)
}

func TestWorkspaceUpdatePermission(t *testing.T) {
	app := newTestApp(t)

	_, ownerTok, slug := registerAndLogin(t, app, "ws-upd@example.com", "WS Upd", "securepass99")

	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "UpdateMe",
	})
	createReq.Header.Set("Authorization", "Bearer "+ownerTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := createRes.BodyFields["id"].(string)

	// Owner can update (owner has manage via * wildcard).
	newName := "UpdatedName"
	patchReq := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, map[string]any{
		"name": newName,
	})
	patchReq.Header.Set("Authorization", "Bearer "+ownerTok)
	patchRes := send(t, patchReq, app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusNoContent)
}
