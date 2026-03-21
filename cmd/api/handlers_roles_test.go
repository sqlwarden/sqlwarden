package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestCreateAndListRoles(t *testing.T) {
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "role-create@example.com", "Role Create", "securepass99")

	// Create a role.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/roles", map[string]any{
		"name":        "analyst",
		"description": "Read-only analyst role",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	assert.Equal(t, res.BodyFields["name"].(string), "analyst")

	// List roles.
	listReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/roles", nil)
	listReq.Header.Set("Authorization", "Bearer "+tok)
	listRes := send(t, listReq, app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var roles []map[string]any
	err := json.Unmarshal(listRes.BodyBytes, &roles)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(roles), 1)
}

func TestGetAndDeleteRole(t *testing.T) {
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "role-crud@example.com", "Role CRUD", "securepass99")

	// Create.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/roles", map[string]any{
		"name": "devops",
	})
	createReq.Header.Set("Authorization", "Bearer "+tok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	roleID := createRes.BodyFields["id"].(string)

	// Get.
	getReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/roles/"+roleID, nil)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getRes := send(t, getReq, app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "devops")

	// Delete.
	delReq := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/roles/"+roleID, nil)
	delReq.Header.Set("Authorization", "Bearer "+tok)
	delRes := send(t, delReq, app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Get after delete returns 404.
	getReq2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/roles/"+roleID, nil)
	getReq2.Header.Set("Authorization", "Bearer "+tok)
	getRes2 := send(t, getReq2, app.routes())
	assert.Equal(t, getRes2.StatusCode, http.StatusNotFound)
}

func TestRoleNonAdminForbidden(t *testing.T) {
	app := newTestApp(t)

	// Owner.
	_, ownerTok, slug := registerAndLogin(t, app, "role-owner@example.com", "Role Owner", "securepass99")

	// Member.
	_, memberTok, _ := registerAndLogin(t, app, "role-member@example.com", "Role Member", "securepass99")

	// Add member to org.
	addReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "role-member@example.com",
		"role":  "member",
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Member cannot list roles (requires admin).
	listReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/roles", nil)
	listReq.Header.Set("Authorization", "Bearer "+memberTok)
	listRes := send(t, listReq, app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusForbidden)

	// Member cannot create roles.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/roles", map[string]any{
		"name": "hacker-role",
	})
	createReq.Header.Set("Authorization", "Bearer "+memberTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusForbidden)
}
