package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
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
			"scope_type":  "org",
			"permissions": []string{"org:read", "ws:read", "env:read", "conn:read"},
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
		map[string]any{"scope_type": "org"}, tok), app.routes())
	assert.Equal(t, badRes.StatusCode, http.StatusUnprocessableEntity)

	// Invalid scope_type returns 422.
	badRes2 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{"name": "test", "scope_type": "invalid"}, tok), app.routes())
	assert.Equal(t, badRes2.StatusCode, http.StatusUnprocessableEntity)

	// Child scope through org role endpoint returns 422.
	badResChildScope := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{"name": "test", "scope_type": "workspace", "permissions": []string{"ws:read"}}, tok), app.routes())
	assert.Equal(t, badResChildScope.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, badResChildScope, "scope_type")

	// Explicit workspace_id through org role endpoint returns 422.
	badResWorkspaceID := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{"name": "test", "scope_type": "org", "workspace_id": 1, "permissions": []string{"org:read"}}, tok), app.routes())
	assert.Equal(t, badResWorkspaceID.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, badResWorkspaceID, "workspace_id")

	// Permission not valid for scope returns 422.
	badRes3 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{
			"name":        "test",
			"scope_type":  "org",
			"permissions": []string{"not:a_permission"},
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
	_, hasPermissionDetails := res.BodyFields["permission_details"]
	_, hasScopeDetails := res.BodyFields["scope_details"]
	_, hasResourceMap := res.BodyFields["resource_map"]
	_, hasResourceDetails := res.BodyFields["resource_details"]
	assert.True(t, hasPerms)
	assert.True(t, hasScopeMap)
	assert.True(t, hasPermissionDetails)
	assert.True(t, hasScopeDetails)
	assert.True(t, hasResourceMap)
	assert.True(t, hasResourceDetails)

	perms := res.BodyFields["permissions"].([]any)
	details := res.BodyFields["permission_details"].([]any)
	scopeMap := res.BodyFields["scope_map"].(map[string]any)
	scopeDetails := res.BodyFields["scope_details"].(map[string]any)
	resourceMap := res.BodyFields["resource_map"].(map[string]any)
	resourceDetails := res.BodyFields["resource_details"].(map[string]any)
	permSet := map[string]bool{}
	for _, perm := range perms {
		permSet[perm.(string)] = true
	}
	assert.Equal(t, len(details), len(perms))
	firstDetail := details[0].(map[string]any)
	assert.True(t, firstDetail["key"] != "")
	assert.True(t, firstDetail["label"] != "")
	assert.True(t, firstDetail["description"] != "")
	assert.True(t, firstDetail["group"] != "")
	assert.Equal(t, permSet["conn:dql"], true)
	assert.Equal(t, permSet["conn:dml"], true)
	assert.Equal(t, permSet["conn:ddl"], true)
	assert.Equal(t, permSet["query:execute"], false)
	assert.Equal(t, permSet["job:read"], false)
	assert.Equal(t, permSet["file:read"], false)
	assert.Equal(t, permSet["conn:metadata"], false)

	connScope := scopeMap["connection"].([]any)
	connScopeDetails := scopeDetails["connection"].([]any)
	assert.Equal(t, len(connScopeDetails), len(connScope))
	connPerms := map[string]bool{}
	for _, perm := range connScope {
		connPerms[perm.(string)] = true
	}
	assert.Equal(t, connPerms["conn:dql"], true)
	assert.Equal(t, connPerms["conn:dml"], true)
	assert.Equal(t, connPerms["conn:ddl"], true)
	assert.Equal(t, connPerms["query:execute"], false)

	workspaceScope := scopeMap["workspace"].([]any)
	workspaceResource := resourceMap["workspace"].([]any)
	workspaceResourceDetails := resourceDetails["workspace"].([]any)
	assert.Equal(t, len(workspaceResourceDetails), len(workspaceResource))
	workspaceScopePerms := map[string]bool{}
	for _, perm := range workspaceScope {
		workspaceScopePerms[perm.(string)] = true
	}
	workspaceResourcePerms := map[string]bool{}
	for _, perm := range workspaceResource {
		workspaceResourcePerms[perm.(string)] = true
	}
	assert.Equal(t, workspaceScopePerms["ws:delete"], false)
	assert.Equal(t, workspaceResourcePerms["ws:delete"], true)
	assert.Equal(t, workspaceResourcePerms["ws:create"], false)
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

func TestDeleteRoleWithBindingsReturnsConflict(t *testing.T) {
	app := newTestApp(t)
	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "role-in-use-owner"), "Role In Use Owner", "Role In Use")
	member := seedAccount(t, app, uniqueEmail(t, "role-in-use-member"), "Role In Use Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	roleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgRead)
	assert.Equal(t, grantOrgPolicyRole(t, app, tok, org.Slug, roleID, access.SubjectTypeAccount, member.ID).StatusCode, http.StatusNoContent)

	res := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/roles/"+strconv.FormatInt(roleID, 10), nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusConflict)
	assert.Equal(t, res.BodyFields["binding_count"].(float64), float64(1))

	// The binding remains; role deletion must not silently revoke policies.
	var bindingCount int
	if err := app.db.NewSelect().TableExpr("role_bindings").ColumnExpr("COUNT(*)").Where("role_id = ?", roleID).Scan(context.Background(), &bindingCount); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, bindingCount, 1)
}

func TestListRoles_SupportsPaginationSearchAndBuiltinFilter(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "role-list@example.com", "Role List", "securepass99")

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/roles",
		map[string]any{
			"name":        "qa-viewer",
			"scope_type":  "org",
			"permissions": []string{"org:read"},
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

func TestListRoles_SupportsScopeFilter(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "role-scope-owner"), "Role Scope Owner", "Role Scope Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Scoped Workspace", "")

	orgRoleRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/roles",
		map[string]any{
			"name":        "org-auditor",
			"scope_type":  "org",
			"permissions": []string{"org:read"},
		}, tok), app.routes())
	assert.Equal(t, orgRoleRes.StatusCode, http.StatusCreated)

	workspaceRoleRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/roles",
		map[string]any{
			"name":        "workspace-auditor",
			"permissions": []string{"ws:read"},
		}, tok), app.routes())
	assert.Equal(t, workspaceRoleRes.StatusCode, http.StatusCreated)

	orgRolesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/roles?scope=org&page=1&page_size=100", nil, tok), app.routes())
	assert.Equal(t, orgRolesRes.StatusCode, http.StatusOK)

	orgItems := orgRolesRes.BodyFields["items"].([]any)
	for _, raw := range orgItems {
		item := raw.(map[string]any)
		assert.Equal(t, item["scope_type"].(string), "org")
	}

	workspaceRolesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/roles?scope=workspace&page=1&page_size=100", nil, tok), app.routes())
	assert.Equal(t, workspaceRolesRes.StatusCode, http.StatusOK)

	workspaceItems := workspaceRolesRes.BodyFields["items"].([]any)
	for _, raw := range workspaceItems {
		item := raw.(map[string]any)
		assert.Equal(t, item["scope_type"].(string), "workspace")
	}

	badScopeRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/roles?scope=connection", nil, tok), app.routes())
	assert.Equal(t, badScopeRes.StatusCode, http.StatusUnprocessableEntity)
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

func TestGrantOrgPolicySupportsOrgMembersPrincipal(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-org-members-owner"), "Org Policy Owner", "Org Policy Org Members")
	roleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      roleID,
			"subject_type": access.SubjectTypeOrgMembers,
			"subject_id":   org.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/policies?subject_type=org_members&permission=org:write&page=1&page_size=10", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, int(listRes.BodyFields["total"].(float64)), 1)

	items := listRes.BodyFields["items"].([]any)
	item := items[0].(map[string]any)
	assert.Equal(t, item["subject_name"].(string), "All organization members")
	assert.Equal(t, item["subject_id"].(float64), float64(org.ID))
}

func TestGrantOrgPolicyRejectsOrgMembersSubjectFromOtherOrg(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-org-members-owner-a"), "Org Policy Owner A", "Org Policy Org Members A")
	_, _, otherOrg := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-org-members-owner-b"), "Org Policy Owner B", "Org Policy Org Members B")
	roleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      roleID,
			"subject_type": access.SubjectTypeOrgMembers,
			"subject_id":   otherOrg.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestGrantOrgPolicyRejectsCrossOrgAccountSubject(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-account-owner-a"), "Org Policy Owner A", "Org Policy Account A")
	otherOwner, _, _ := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-account-owner-b"), "Org Policy Owner B", "Org Policy Account B")
	roleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      roleID,
			"subject_type": "account",
			"subject_id":   otherOwner.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestGrantOrgPolicyRejectsCrossOrgTeamSubject(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-team-owner-a"), "Org Policy Owner A", "Org Policy Team A")
	_, _, otherOrg := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-team-owner-b"), "Org Policy Owner B", "Org Policy Team B")
	otherTeam, err := app.db.InsertTeam(context.Background(), otherOrg.ID, "other-team", "Other Team")
	if err != nil {
		t.Fatal(err)
	}
	roleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      roleID,
			"subject_type": "team",
			"subject_id":   otherTeam.ID,
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
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
		if role.Name == access.BuiltinOrgAdminRole {
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

func TestGrantOrgPolicy_AdminCannotBindOwnerOrProtectedOrgRoles(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, _, admin, adminTok, org := seedOrgAdministratorForPolicyTest(t, app)

	ownerRoleID := orgBuiltinRoleID(t, app, org.ID, access.BuiltinOrgOwnerRole)
	deleteRoleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgDelete)
	transferRoleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgTransferOwnership)
	ordinaryRoleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite, access.PermPolicyRead)

	for _, tc := range []struct {
		name   string
		roleID int64
	}{
		{name: "builtin owner", roleID: ownerRoleID},
		{name: "custom delete", roleID: deleteRoleID},
		{name: "custom transfer ownership", roleID: transferRoleID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := grantOrgPolicyRole(t, app, adminTok, org.Slug, tc.roleID, access.SubjectTypeAccount, admin.ID)
			assert.Equal(t, res.StatusCode, http.StatusForbidden)
		})
	}

	ordinaryRes := grantOrgPolicyRole(t, app, adminTok, org.Slug, ordinaryRoleID, access.SubjectTypeAccount, admin.ID)
	assert.Equal(t, ordinaryRes.StatusCode, http.StatusNoContent)

	assert.Equal(t, app.enforcer.Can(context.Background(), admin.ID, org.ID, "org", "org", org.ID, access.PermOrgDelete), false)
	assert.Equal(t, app.enforcer.Can(context.Background(), admin.ID, org.ID, "org", "org", org.ID, access.PermOrgTransferOwnership), false)
}

func TestGrantOrgPolicy_OwnerCanBindProtectedOrgRoles(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-owner-protected"), "Owner Protected", "Owner Protected Org")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-owner-protected-member"), "Protected Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	ownerRoleID := orgBuiltinRoleID(t, app, org.ID, access.BuiltinOrgOwnerRole)
	res := grantOrgPolicyRole(t, app, ownerTok, org.Slug, ownerRoleID, access.SubjectTypeAccount, member.ID)
	assert.Equal(t, res.StatusCode, http.StatusNoContent)
	assert.Equal(t, app.enforcer.Can(context.Background(), member.ID, org.ID, "org", "org", org.ID, access.PermOrgTransferOwnership), true)
	assert.Equal(t, app.enforcer.Can(context.Background(), owner.ID, org.ID, "org", "org", org.ID, access.PermOrgTransferOwnership), true)
}

func TestRevokeOrgPolicy_AdminCannotRevokeOwnerPolicy(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, ownerTok, admin, adminTok, org := seedOrgAdministratorForPolicyTest(t, app)
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-revoke-owner-member"), "Second Owner")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	ownerRoleID := orgBuiltinRoleID(t, app, org.ID, access.BuiltinOrgOwnerRole)
	grantRes := grantOrgPolicyRole(t, app, ownerTok, org.Slug, ownerRoleID, access.SubjectTypeAccount, member.ID)
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	protectedRoleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgDelete)
	protectedGrantRes := grantOrgPolicyRole(t, app, ownerTok, org.Slug, protectedRoleID, access.SubjectTypeAccount, member.ID)
	assert.Equal(t, protectedGrantRes.StatusCode, http.StatusNoContent)

	ordinaryRoleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite)
	ordinaryGrantRes := grantOrgPolicyRole(t, app, ownerTok, org.Slug, ordinaryRoleID, access.SubjectTypeAccount, member.ID)
	assert.Equal(t, ordinaryGrantRes.StatusCode, http.StatusNoContent)

	bindingID := orgPolicyBindingID(t, app, adminTok, org.Slug, member.ID, access.BuiltinOrgOwnerRole)
	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+bindingID, nil, adminTok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusForbidden)

	protectedBindingID := orgPolicyBindingIDByRoleID(t, app, adminTok, org.Slug, member.ID, protectedRoleID)
	protectedRevokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+protectedBindingID, nil, adminTok), app.routes())
	assert.Equal(t, protectedRevokeRes.StatusCode, http.StatusForbidden)

	ordinaryBindingID := orgPolicyBindingIDByRoleID(t, app, adminTok, org.Slug, member.ID, ordinaryRoleID)
	ordinaryRevokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+ordinaryBindingID, nil, adminTok), app.routes())
	assert.Equal(t, ordinaryRevokeRes.StatusCode, http.StatusNoContent)

	assert.Equal(t, app.enforcer.Can(context.Background(), member.ID, org.ID, "org", "org", org.ID, access.PermOrgTransferOwnership), true)
	assert.Equal(t, app.enforcer.Can(context.Background(), admin.ID, org.ID, "org", "org", org.ID, access.PermOrgTransferOwnership), false)

	ownerBindingID := orgPolicyBindingID(t, app, ownerTok, org.Slug, owner.ID, access.BuiltinOrgOwnerRole)
	ownerRevokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+ownerBindingID, nil, ownerTok), app.routes())
	assert.Equal(t, ownerRevokeRes.StatusCode, http.StatusNoContent)
}

func TestRevokeOnlyOrgOwnerPolicyForbidden(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-last-owner"), "Policy Owner", "Policy Last Owner")

	bindingID := orgPolicyBindingID(t, app, tok, org.Slug, owner.ID, access.BuiltinOrgOwnerRole)
	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+bindingID, nil, tok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusUnprocessableEntity)

	roles, err := app.db.ListOrgRoles(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ownerRoleID int64
	for _, role := range roles {
		if role.Name == access.BuiltinOrgOwnerRole && role.IsBuiltin {
			ownerRoleID = role.ID
			break
		}
	}
	if ownerRoleID == 0 {
		t.Fatal("expected owner role to exist")
	}
	count, err := app.db.CountRoleBindings(context.Background(), org.ID, ownerRoleID, "org", org.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, 1)
}

func TestRevokeOrgOwnerPolicyAllowedWhenAnotherOwnerPolicyExists(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-extra-owner"), "Policy Owner", "Policy Extra Owner")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-extra-owner-member"), "Second Owner")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	roles, err := app.db.ListOrgRoles(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ownerRoleID int64
	for _, role := range roles {
		if role.Name == access.BuiltinOrgOwnerRole && role.IsBuiltin {
			ownerRoleID = role.ID
			break
		}
	}
	if ownerRoleID == 0 {
		t.Fatal("expected owner role to exist")
	}

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      ownerRoleID,
			"subject_type": "account",
			"subject_id":   member.ID,
		}, tok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	bindingID := orgPolicyBindingID(t, app, tok, org.Slug, owner.ID, access.BuiltinOrgOwnerRole)
	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+bindingID, nil, tok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	count, err := app.db.CountRoleBindings(context.Background(), org.ID, ownerRoleID, "org", org.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, 1)
}

func orgPolicyBindingID(t *testing.T, app *application, token, orgSlug string, accountID int64, roleName string) string {
	t.Helper()

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/policies?subject_type=account&subject_id="+fmt.Sprint(accountID), nil, token), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	items := listRes.BodyFields["items"].([]any)
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["role_name"] == roleName {
			return fmt.Sprintf("%v", item["binding_id"])
		}
	}
	t.Fatalf("expected policy binding for role %q", roleName)
	return ""
}

func orgPolicyBindingIDByRoleID(t *testing.T, app *application, token, orgSlug string, accountID, roleID int64) string {
	t.Helper()

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/policies?subject_type=account&subject_id="+fmt.Sprint(accountID), nil, token), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	items := listRes.BodyFields["items"].([]any)
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["role_id"].(float64) == float64(roleID) {
			return fmt.Sprintf("%v", item["binding_id"])
		}
	}
	t.Fatalf("expected policy binding for role ID %d", roleID)
	return ""
}

func seedOrgAdministratorForPolicyTest(t *testing.T, app *application) (database.Account, string, database.Account, string, database.Organization) {
	t.Helper()

	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "org-policy-admin-owner"), "Admin Owner", "Admin Org")
	admin, adminTok := seedAccountWithToken(t, app, uniqueEmail(t, "org-policy-admin"), "Org Admin")
	if err := app.db.AddOrgMember(context.Background(), org.ID, admin.ID); err != nil {
		t.Fatal(err)
	}
	adminRoleID := orgBuiltinRoleID(t, app, org.ID, access.BuiltinOrgAdminRole)
	res := grantOrgPolicyRole(t, app, ownerTok, org.Slug, adminRoleID, access.SubjectTypeAccount, admin.ID)
	assert.Equal(t, res.StatusCode, http.StatusNoContent)
	return owner, ownerTok, admin, adminTok, org
}

func orgBuiltinRoleID(t *testing.T, app *application, orgID int64, name string) int64 {
	t.Helper()

	roles, err := app.db.ListOrgRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if role.Name == name && role.IsBuiltin {
			return role.ID
		}
	}
	t.Fatalf("expected builtin org role %q", name)
	return 0
}
