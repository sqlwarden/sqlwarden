package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestRoleLifecycle(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "policyowner@example.com", "Policy Owner", "securepass99")

	// List roles (builtin roles should exist after org creation).
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/roles", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	// Create a custom role.
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{
			"name":        "viewer",
			"scope_type":  "workspace",
			"permissions": []string{"ws:read", "env:read", "conn:metadata"},
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["name"].(string), "viewer")

	roleID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Get the role.
	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/roles/"+roleID, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "viewer")

	// Delete the role.
	delRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/roles/"+roleID, nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Get returns 404 after deletion.
	getAfterDel := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/roles/"+roleID, nil, tok), app.routes())
	assert.Equal(t, getAfterDel.StatusCode, http.StatusNotFound)
}

func TestCreateRoleValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "policy-val@example.com", "Policy Val", "securepass99")

	// Missing name returns 422.
	badRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{"scope_type": "workspace"}, tok), app.routes())
	assert.Equal(t, badRes.StatusCode, http.StatusUnprocessableEntity)

	// Invalid scope_type returns 422.
	badRes2 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{"name": "test", "scope_type": "invalid"}, tok), app.routes())
	assert.Equal(t, badRes2.StatusCode, http.StatusUnprocessableEntity)

	// Permission not valid for scope returns 422.
	badRes3 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{
			"name":        "test",
			"scope_type":  "connection",
			"permissions": []string{"org:write"},
		}, tok), app.routes())
	assert.Equal(t, badRes3.StatusCode, http.StatusUnprocessableEntity)
}

func TestListPermissions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "perm-list@example.com", "Perm List", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/permissions", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	_, hasPerms := res.BodyFields["permissions"]
	_, hasScopeMap := res.BodyFields["scope_map"]
	assert.True(t, hasPerms)
	assert.True(t, hasScopeMap)
}

func TestDeleteBuiltinRoleForbidden(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "builtin-del@example.com", "Builtin Del", "securepass99")

	// List roles to find a builtin role ID.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/roles", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var roles []map[string]any
	err := json.Unmarshal(listRes.BodyBytes, &roles)
	if err != nil {
		t.Fatal(err)
	}

	// Find a builtin role.
	var builtinID string
	for _, r := range roles {
		if isBuiltin, ok := r["is_builtin"].(bool); ok && isBuiltin {
			builtinID = fmt.Sprintf("%v", r["id"])
			break
		}
	}

	if builtinID == "" {
		t.Skip("no builtin roles found")
	}

	// Attempt to delete a builtin role should be forbidden.
	delRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/roles/"+builtinID, nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusForbidden)
}

func TestDeleteRoleNotFoundReturns404(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "missing-role@example.com", "Missing Role", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/roles/999999", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}
