package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
)

func TestResourceBelongsToWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, "resource-belongs@example.com", "Resource Belongs", "Resource Org")
	ws1 := seedWorkspaceForAccount(t, app, org, owner, "WS1", "")
	ws2 := seedWorkspaceForAccount(t, app, org, owner, "WS2", "")

	env, err := app.db.InsertEnvironment(context.Background(), ws2.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := app.db.InsertConnection(context.Background(), ws2.ID, &env.ID, "db", "sqlite", ":memory:", "open")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := app.resourceBelongsToWorkspace(newTestRequest(t, http.MethodGet, "/", nil), "workspace", ws1.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ok, true)

	ok, err = app.resourceBelongsToWorkspace(newTestRequest(t, http.MethodGet, "/", nil), "environment", env.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ok, false)

	ok, err = app.resourceBelongsToWorkspace(newTestRequest(t, http.MethodGet, "/", nil), "connection", conn.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ok, false)

	ok, err = app.resourceBelongsToWorkspace(newTestRequest(t, http.MethodGet, "/", nil), "invalid", 123, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ok, false)
}

func TestCreateWorkspacePolicy_WrongWorkspaceResourceReturns404(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-owner"), "Policy Owner", "Policy Org")
	wsA := seedWorkspaceForAccount(t, app, org, owner, "Workspace A", "")
	wsB := seedWorkspaceForAccount(t, app, org, owner, "Workspace B", "")
	envB := seedEnvironment(t, app, wsB.ID, org.ID, "prod")
	roleID := createRoleForTest(t, app, org.ID, nil, "environment", "env:read")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(wsA.ID, 10)+"/policies",
		map[string]any{
			"role_id":       roleID,
			"subject_type":  "account",
			"subject_id":    owner.ID,
			"resource_type": "environment",
			"resource_id":   envB.ID,
		}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestCreateWorkspacePolicy_MissingRoleIDReturns422(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-perm-owner"), "Policy Owner", "Policy Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies",
		map[string]any{
			"subject_type": "account",
			"subject_id":   owner.ID,
		}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "role_id")
}

func TestCreateWorkspacePolicy_MissingRoleReturns404(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-role-owner"), "Policy Owner", "Policy Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies",
		map[string]any{
			"role_id":      999999,
			"subject_type": "account",
			"subject_id":   owner.ID,
		}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestCreateWorkspaceRole_DuplicateNameReturns422(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "role-owner"), "Role Owner", "Role Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")

	first := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/roles",
		map[string]any{
			"name":        "analyst",
			"description": "read only",
			"permissions": []string{"ws:read"},
		}, token), app.routes())
	assert.Equal(t, first.StatusCode, http.StatusCreated)

	second := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/roles",
		map[string]any{
			"name":        "analyst",
			"description": "duplicate",
			"permissions": []string{"ws:read"},
		}, token), app.routes())
	assert.Equal(t, second.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, second, "name")
}

// wsSetup creates an org + workspace and returns the org slug, workspace ID, and owner token.
func wsSetup(t *testing.T, app *application, email, name string) (slug, wsID, tok string) {
	t.Helper()
	account, token, org := seedOrgOwner(t, app, email, name, name+"'s Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Main Workspace", "")
	slug = org.Slug
	wsID = strconv.FormatInt(ws.ID, 10)
	tok = token
	return
}

// wsJoinAs registers a new user, adds them to the org, binds them to a workspace role, and returns their token.
func wsJoinAs(t *testing.T, app *application, slug, wsID, roleName, email string, ownerTok string) string {
	t.Helper()
	member, memberTok := seedAccountWithToken(t, app, email, email)
	org, found, err := app.db.GetOrgBySlug(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("wsJoinAs: org %q not found", slug)
	}
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	// List workspace roles to find the target role ID.
	rolesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles", nil, ownerTok), app.routes())
	assert.Equal(t, rolesRes.StatusCode, http.StatusOK)
	var rolePayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rolesRes.BodyBytes, &rolePayload); err != nil {
		t.Fatal(err)
	}
	var roleID float64
	for _, r := range rolePayload.Items {
		if r["name"].(string) == roleName {
			roleID = r["id"].(float64)
			break
		}
	}
	if roleID == 0 {
		t.Fatalf("wsJoinAs: role %q not found in workspace", roleName)
	}

	// Bind role via workspace access endpoint.
	bindRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/policies",
		map[string]any{
			"subject_type": "account",
			"subject_id":   member.ID,
			"role_id":      int64(roleID),
		}, ownerTok), app.routes())
	if bindRes.StatusCode != http.StatusCreated && bindRes.StatusCode != http.StatusNoContent {
		t.Fatalf("wsJoinAs: bind role got %d: %s", bindRes.StatusCode, bindRes.BodyBytes)
	}

	return memberTok
}

// ── List workspace roles ──────────────────────────────────────────────────────

func TestListWorkspaceRolesBuiltins(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-list@example.com", "WSR List")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(res.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	// SeedWorkspace creates ws:admin and ws:member builtins.
	assert.Equal(t, len(payload.Items), 2)
	names := map[string]bool{}
	for _, r := range payload.Items {
		names[r["name"].(string)] = true
	}
	assert.Equal(t, names["ws:admin"], true)
	assert.Equal(t, names["ws:member"], true)
}

func TestListWorkspaceRolesPermissions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-lr@example.com", "WSR LR")

	// ws:member does NOT have policy:read → 403.
	memberTok := wsJoinAs(t, app, slug, wsID, "ws:member", "wsr-lr-member@example.com", ownerTok)
	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles", nil, memberTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)

	// ws:admin has policy:read → 200.
	wsAdminTok := wsJoinAs(t, app, slug, wsID, "ws:admin", "wsr-lr-admin@example.com", ownerTok)
	res2 := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles", nil, wsAdminTok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusOK)
}

func TestListWorkspaceRoles_SupportsPaginationSearchAndBuiltinFilter(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-filters@example.com", "WSR Filters")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{
			"name":        "qa-viewer",
			"description": "Read only role",
			"permissions": []string{"ws:read"},
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles?q=viewer&builtin=false&page=1&page_size=1&sort=name&order=asc", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, int(listRes.BodyFields["total"].(float64)), 1)

	items := listRes.BodyFields["items"].([]any)
	assert.Equal(t, len(items), 1)
	item := items[0].(map[string]any)
	assert.Equal(t, item["name"].(string), "qa-viewer")
}

// ── Create workspace role ─────────────────────────────────────────────────────

func TestCreateWorkspaceRoleByWsAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-create@example.com", "WSR Create")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{
			"name":        "read-only",
			"description": "Read only role",
			"permissions": []string{"ws:read", "conn:read"},
		}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	assert.Equal(t, res.BodyFields["name"].(string), "read-only")
}

func TestCreateWorkspaceRoleByOrgAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-orgadmin@example.com", "WSR OrgAdmin")

	// Seed a second user and promote them to org:admin.
	adminAccount, adminTok := seedAccountWithToken(t, app, "wsr-admin@example.com", "Admin")
	adminID := fmt.Sprintf("%d", adminAccount.ID)

	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members",
		map[string]any{"email": "wsr-admin@example.com"}, ownerTok), app.routes())

	send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+adminID,
		map[string]any{"role": "admin"}, ownerTok), app.routes())

	// org:admin can create workspace roles.
	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{
			"name":        "analyst",
			"permissions": []string{"ws:read", "conn:dql"},
		}, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
}

func TestCreateWorkspaceRoleByWsMemberForbidden(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-mforbid@example.com", "WSR MForbid")

	memberTok := wsJoinAs(t, app, slug, wsID, "ws:member", "wsr-mforbid-m@example.com", ownerTok)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "sneaky", "permissions": []string{"ws:read"}}, memberTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestCreateWorkspaceRoleValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-val@example.com", "WSR Val")

	// Missing name.
	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"permissions": []string{"ws:read"}}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)

	// Invalid permission for workspace scope.
	res2 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "bad", "permissions": []string{"org:delete"}}, tok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

// ── Get workspace role ────────────────────────────────────────────────────────

func TestGetWorkspaceRole(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-get@example.com", "WSR Get")

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "viewer", "permissions": []string{"ws:read"}}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	roleID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+roleID, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "viewer")
}

func TestGetWorkspaceRoleCrossWorkspaceIsolation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-iso@example.com", "WSR Iso")

	// Create a second workspace.
	ws2Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Second WS"}, tok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	// Create a role in ws2.
	roleRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+ws2ID+"/roles",
		map[string]any{"name": "ws2-role", "permissions": []string{"ws:read"}}, tok), app.routes())
	assert.Equal(t, roleRes.StatusCode, http.StatusCreated)
	ws2RoleID := fmt.Sprintf("%v", roleRes.BodyFields["id"])

	// Getting ws2's role via ws1's URL returns 404.
	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+ws2RoleID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestGetWorkspaceRoleNotFound(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-nf@example.com", "WSR NF")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/9999", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

// ── Delete workspace role ─────────────────────────────────────────────────────

func TestDeleteWorkspaceRole(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-del@example.com", "WSR Del")

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "temp-role", "permissions": []string{"ws:read"}}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	roleID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	delRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+roleID, nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Confirm it's gone.
	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+roleID, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusNotFound)
}

func TestDeleteBuiltinWorkspaceRoleForbidden(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-dbuilt@example.com", "WSR DBuilt")

	// Get the ws:admin builtin role ID.
	rolesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles", nil, tok), app.routes())
	var rolePayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rolesRes.BodyBytes, &rolePayload); err != nil {
		t.Fatal(err)
	}
	var builtinID string
	for _, r := range rolePayload.Items {
		if r["name"].(string) == "ws:admin" {
			builtinID = fmt.Sprintf("%v", r["id"])
			break
		}
	}

	res := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+builtinID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestDeleteWorkspaceRoleByWsMemberForbidden(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-dmforbid@example.com", "WSR DMForbid")

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "deletable", "permissions": []string{"ws:read"}}, ownerTok), app.routes())
	roleID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	memberTok := wsJoinAs(t, app, slug, wsID, "ws:member", "wsr-dmforbid-m@example.com", ownerTok)

	res := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+roleID, nil, memberTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestDeleteWorkspaceRoleCrossWorkspaceIsolation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-diso@example.com", "WSR DIso")

	ws2Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Second WS"}, tok), app.routes())
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	roleRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+ws2ID+"/roles",
		map[string]any{"name": "ws2-role", "permissions": []string{"ws:read"}}, tok), app.routes())
	ws2RoleID := fmt.Sprintf("%v", roleRes.BodyFields["id"])

	// Deleting ws2's role via ws1's URL returns 404.
	res := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+ws2RoleID, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

// ── List workspace permissions ────────────────────────────────────────────────

func TestListWorkspacePermissions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, tok := wsSetup(t, app, "wsr-perms@example.com", "WSR Perms")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/permissions", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	perms, ok := res.BodyFields["permissions"].([]any)
	assert.Equal(t, ok, true)
	// Workspace scope has more than zero permissions.
	assert.Equal(t, len(perms) > 0, true)

	// Org-only permissions must NOT appear.
	for _, p := range perms {
		pstr := p.(string)
		if pstr == "org:delete" || pstr == "org:transfer_ownership" || pstr == "ws:create" || pstr == "query:execute" || pstr == "job:read" || pstr == "file:read" || pstr == "conn:metadata" {
			t.Errorf("org-only permission %q should not appear in workspace permissions", pstr)
		}
	}

	seen := map[string]bool{}
	for _, p := range perms {
		seen[p.(string)] = true
	}
	assert.Equal(t, seen["conn:dql"], true)
	assert.Equal(t, seen["conn:dml"], true)
	assert.Equal(t, seen["conn:ddl"], true)
}

func TestListWorkspacePermissionsAccessibleByWsMember(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-perms-m@example.com", "WSR Perms M")

	memberTok := wsJoinAs(t, app, slug, wsID, "ws:member", "wsr-perms-m-m@example.com", ownerTok)

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/permissions", nil, memberTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
}

// ── Org-level role access at workspace scope ──────────────────────────────────

func TestOrgOwnerCanManageWorkspaceRoles(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-owner@example.com", "WSR Owner")

	// Owner creates a workspace role.
	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "owner-created", "permissions": []string{"ws:read"}}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
}

func TestWsAdminCanCreateAndDeleteWorkspaceRoles(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsr-wsadmin@example.com", "WSR WsAdmin")

	wsAdminTok := wsJoinAs(t, app, slug, wsID, "ws:admin", "wsr-wsadmin-a@example.com", ownerTok)

	// ws:admin creates a role.
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles",
		map[string]any{"name": "custom", "permissions": []string{"ws:read", "conn:read"}}, wsAdminTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	roleID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// ws:admin deletes it.
	delRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/roles/"+roleID, nil, wsAdminTok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)
}

func TestListWorkspacePolicies_SupportsSubjectPermissionAndResourceFilters(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-list-owner"), "Policy List Owner", "Policy List Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Policy Workspace", "")
	env := seedEnvironment(t, app, ws.ID, org.ID, "prod")
	conn := seedConnection(t, app, ws.ID, &env.ID, org.ID, "postgres", "Primary DB", "open")
	roleID := createRoleForTest(t, app, org.ID, nil, "connection", "conn:execute")

	team, err := app.db.InsertTeam(context.Background(), org.ID, "qa-team", "QA Team")
	if err != nil {
		t.Fatal(err)
	}
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "policy-list-member"), "Member User")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies",
		map[string]any{
			"role_id":       roleID,
			"subject_type":  "team",
			"subject_id":    team.ID,
			"resource_type": "connection",
			"resource_id":   conn.ID,
		}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies?q=db&subject_type=team&permission=conn:execute&resource_type=connection&page=1&page_size=10",
		ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)

	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 10)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["subject_type"], "team")
	assert.Equal(t, payload.Items[0]["subject_name"], "QA Team")
	assert.Equal(t, payload.Items[0]["resource_type"], "connection")
	assert.Equal(t, payload.Items[0]["resource_name"], "Primary DB")
	assert.Equal(t, payload.Items[0]["binding_kind"], "role")
	assert.Equal(t, payload.Items[0]["role_name"] != "", true)
}

func TestGrantWorkspacePolicySupportsOrgMembersPrincipal(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-policy-org-members-owner"), "Workspace Policy Owner", "Workspace Policy Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Policy Workspace", "")
	roleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", access.PermWsRead)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies",
		map[string]any{
			"role_id":       roleID,
			"subject_type":  access.SubjectTypeOrgMembers,
			"subject_id":    org.ID,
			"resource_type": "workspace",
		}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies?subject_type=org_members&permission=ws:read&page=1&page_size=10",
		ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, payload.Items[0]["subject_name"], "All organization members")
}

func TestListWorkspacePoliciesPermissions(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	slug, wsID, ownerTok := wsSetup(t, app, "wsp-list-perms@example.com", "WSP List Perms")

	memberTok := wsJoinAs(t, app, slug, wsID, "ws:member", "wsp-list-member@example.com", ownerTok)
	memberRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/policies", nil, memberTok), app.routes())
	assert.Equal(t, memberRes.StatusCode, http.StatusForbidden)

	wsAdminTok := wsJoinAs(t, app, slug, wsID, "ws:admin", "wsp-list-admin@example.com", ownerTok)
	adminRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/policies", nil, wsAdminTok), app.routes())
	assert.Equal(t, adminRes.StatusCode, http.StatusOK)
}

func TestListWorkspacePolicies_ReturnsRenderableSubjectAndResourceMetadata(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-render-owner"), "Policy Render Owner", "Policy Render Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Policy Workspace", "")
	env := seedEnvironment(t, app, ws.ID, org.ID, "prod")
	conn := seedConnection(t, app, ws.ID, &env.ID, org.ID, "postgres", "Primary DB", "open")
	roleID := createRoleForTest(t, app, org.ID, nil, "connection", "conn:read")

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies",
		map[string]any{
			"role_id":       roleID,
			"subject_type":  "account",
			"subject_id":    owner.ID,
			"resource_type": "connection",
			"resource_id":   conn.ID,
		}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies?resource_type=connection",
		ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, len(payload.Items) > 0, true)
	assert.Equal(t, payload.Items[0]["subject_type"], "account")
	if payload.Items[0]["subject_name"] != owner.Name {
		t.Fatalf("expected subject_name %q, got %v", owner.Name, payload.Items[0]["subject_name"])
	}
	assert.Equal(t, payload.Items[0]["resource_type"], "connection")
	assert.Equal(t, payload.Items[0]["resource_name"], "Primary DB")
}

func TestListWorkspacePolicies_SupportsSubjectIDAndResourceIDFilters(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-id-list-owner"), "Policy ID Owner", "Policy ID Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Policy Workspace", "")
	env := seedEnvironment(t, app, ws.ID, org.ID, "prod")
	connA := seedConnection(t, app, ws.ID, &env.ID, org.ID, "postgres", "Primary DB", "open")
	connB := seedConnection(t, app, ws.ID, &env.ID, org.ID, "postgres", "Replica DB", "open")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "policy-id-member"), "Member User")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	roleExecA := createRoleForTest(t, app, org.ID, nil, "connection", "conn:execute")
	roleReadA := createRoleForTest(t, app, org.ID, nil, "connection", "conn:read")
	roleExecB := createRoleForTest(t, app, org.ID, nil, "connection", "conn:execute")

	for _, body := range []map[string]any{
		{
			"role_id":       roleExecA,
			"subject_type":  "account",
			"subject_id":    owner.ID,
			"resource_type": "connection",
			"resource_id":   connA.ID,
		},
		{
			"role_id":       roleReadA,
			"subject_type":  "account",
			"subject_id":    member.ID,
			"resource_type": "connection",
			"resource_id":   connA.ID,
		},
		{
			"role_id":       roleExecB,
			"subject_type":  "account",
			"subject_id":    owner.ID,
			"resource_type": "connection",
			"resource_id":   connB.ID,
		},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost,
			"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies",
			body, ownerTok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNoContent)
	}

	listRes := send(t, newOrgRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/workspaces/"+strconv.FormatInt(ws.ID, 10)+"/policies?subject_id="+strconv.FormatInt(owner.ID, 10)+"&resource_id="+strconv.FormatInt(connA.ID, 10)+"&resource_type=connection",
		ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, int64(payload.Items[0]["subject_id"].(float64)), owner.ID)
	assert.Equal(t, int64(payload.Items[0]["resource_id"].(float64)), connA.ID)
}
