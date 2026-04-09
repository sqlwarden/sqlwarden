package main

import (
	"context"
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
			"permissions": []string{"ws:read", "env:read", "conn:read"},
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

	perms := res.BodyFields["permissions"].([]any)
	scopeMap := res.BodyFields["scope_map"].(map[string]any)
	permSet := map[string]bool{}
	for _, perm := range perms {
		permSet[perm.(string)] = true
	}
	assert.Equal(t, permSet["conn:dql"], true)
	assert.Equal(t, permSet["conn:dml"], true)
	assert.Equal(t, permSet["conn:ddl"], true)
	assert.Equal(t, permSet["query:execute"], false)
	assert.Equal(t, permSet["job:read"], false)
	assert.Equal(t, permSet["file:read"], false)
	assert.Equal(t, permSet["conn:metadata"], false)

	connScope := scopeMap["connection"].([]any)
	connPerms := map[string]bool{}
	for _, perm := range connScope {
		connPerms[perm.(string)] = true
	}
	assert.Equal(t, connPerms["conn:dql"], true)
	assert.Equal(t, connPerms["conn:dml"], true)
	assert.Equal(t, connPerms["conn:ddl"], true)
	assert.Equal(t, connPerms["query:execute"], false)
}

func TestDeleteBuiltinRoleForbidden(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "builtin-del@example.com", "Builtin Del", "securepass99")

	// List roles to find a builtin role ID.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/roles", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	err := json.Unmarshal(listRes.BodyBytes, &payload)
	if err != nil {
		t.Fatal(err)
	}

	// Find a builtin role.
	var builtinID string
	for _, r := range payload.Items {
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

func TestListRoles_SupportsPaginationSearchAndBuiltinFilter(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "role-list@example.com", "Role List", "securepass99")

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{
			"name":        "qa-viewer",
			"scope_type":  "workspace",
			"permissions": []string{"ws:read"},
		}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/roles?q=viewer&builtin=false&page=1&page_size=1&sort=name&order=asc", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, int(res.BodyFields["total"].(float64)), 1)

	items := res.BodyFields["items"].([]any)
	assert.Equal(t, len(items), 1)
	item := items[0].(map[string]any)
	assert.Equal(t, item["name"].(string), "qa-viewer")
}

func TestListOrgPolicies_SupportsFiltersAndMetadata(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-owner"), "Org Policy Owner", "Org Policy Org")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-member"), "Policy Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	team, err := app.db.InsertTeam(context.Background(), org.ID, "qa-team-"+fmt.Sprint(org.ID), "QA Team")
	if err != nil {
		t.Fatal(err)
	}
	teamRoleID := createRoleForTest(t, app, org.ID, nil, "org", "policy:read")
	ownerRoleID := createRoleForTest(t, app, org.ID, nil, "org", "org:read")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      teamRoleID,
			"subject_type": "team",
			"subject_id":   team.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	res = send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      ownerRoleID,
			"subject_type": "account",
			"subject_id":   owner.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/policies?q=qa&subject_type=team&permission=policy:read&page=1&page_size=10", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, int(listRes.BodyFields["total"].(float64)), 1)

	items := listRes.BodyFields["items"].([]any)
	assert.Equal(t, len(items), 1)
	item := items[0].(map[string]any)
	assert.Equal(t, item["subject_name"].(string), "QA Team")
	assert.Equal(t, item["resource_name"].(string), org.Name)
	assert.Equal(t, item["resource_type"].(string), "org")
}

func TestGrantOrgPolicy_MissingRoleReturns404(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-missing-role-owner"), "Org Policy Owner", "Org Policy Missing Role")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-missing-role-member"), "Policy Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      999999,
			"subject_type": "account",
			"subject_id":   member.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestGrantOrgPolicy_MissingRoleIDReturns422(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-missing-roleid-owner"), "Org Policy Owner", "Org Policy Missing Role ID")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-missing-roleid-member"), "Policy Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"subject_type": "account",
			"subject_id":   member.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "role_id")
}

func TestRevokeOrgPolicy(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-revoke-owner"), "Org Policy Owner", "Org Policy Revoke")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-revoke-member"), "Policy Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	roles, err := app.db.ListOrgRoles(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	var adminRoleID int64
	for _, role := range roles {
		if role.Name == "admin" {
			adminRoleID = role.ID
			break
		}
	}
	if adminRoleID == 0 {
		t.Fatal("expected admin role to exist")
	}

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      adminRoleID,
			"subject_type": "account",
			"subject_id":   member.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/policies?subject_id="+fmt.Sprint(member.ID), nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	items := listRes.BodyFields["items"].([]any)
	assert.Equal(t, len(items), 1)
	bindingID := fmt.Sprintf("%v", items[0].(map[string]any)["binding_id"])

	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+bindingID, nil, tok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	listRes = send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/policies?subject_id="+fmt.Sprint(member.ID), nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, int(listRes.BodyFields["total"].(float64)), 0)
}
