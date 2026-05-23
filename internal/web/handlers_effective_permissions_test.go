package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func effectivePermissionsURL(orgSlug, resourceType string, resourceID int64) string {
	if resourceType == "" {
		return "/api/v1/orgs/" + orgSlug + "/permissions/effective"
	}
	url := "/api/v1/orgs/" + orgSlug + "/permissions/effective?resource_type=" + resourceType
	if resourceID > 0 {
		url += "&resource_id=" + strconv.FormatInt(resourceID, 10)
	}
	return url
}

func effectivePermissions(t *testing.T, app *application, token, orgSlug, resourceType string, resourceID int64) []string {
	t.Helper()

	res := send(t, newAuthRequest(t, http.MethodGet, effectivePermissionsURL(orgSlug, resourceType, resourceID), nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	raw, ok := res.BodyFields["permissions"].([]any)
	if !ok {
		t.Fatalf("permissions response has unexpected shape: %#v", res.BodyFields["permissions"])
	}
	permissions := make([]string, 0, len(raw))
	for _, item := range raw {
		permission, ok := item.(string)
		if !ok {
			t.Fatalf("permission item has unexpected shape: %#v", item)
		}
		permissions = append(permissions, permission)
	}
	return permissions
}

func assertHasPermissions(t *testing.T, permissions []string, expected ...string) {
	t.Helper()
	for _, permission := range expected {
		if !hasPermission(permissions, permission) {
			t.Fatalf("expected permissions to include %q, got %v", permission, permissions)
		}
	}
}

func assertMissingPermissions(t *testing.T, permissions []string, unexpected ...string) {
	t.Helper()
	for _, permission := range unexpected {
		if hasPermission(permissions, permission) {
			t.Fatalf("expected permissions not to include %q, got %v", permission, permissions)
		}
	}
}

func hasPermission(permissions []string, permission string) bool {
	for _, item := range permissions {
		if item == permission {
			return true
		}
	}
	return false
}

func TestEffectivePermissionsOrgDefaultsToCurrentOrg(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-org-owner"), "Owner", "Effective Org")

	permissions := effectivePermissions(t, app, ownerTok, org.Slug, "", 0)

	assertHasPermissions(t, permissions, "org:read", "ws:create", "conn:create", "policy:modify")
}

func TestEffectivePermissionsOrgMemberGetsDefaultBaseline(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-empty-owner"), "Owner", "Effective Empty")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "eff-empty-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	permissions := effectivePermissions(t, app, memberTok, org.Slug, "org", org.ID)

	assertHasPermissions(t, permissions, "org:read")
	assertMissingPermissions(t, permissions, "org:write", "org:invite", "policy:modify")
}

func TestEffectivePermissionsWorkspaceFiltersToWorkspaceApplicablePermissions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-ws-owner"), "Owner", "Effective Workspace")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "eff-ws-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")
	roleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", "ws:read", "env:create", "conn:create", "policy:read")

	grantRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(ws.ID, 10), roleID, "account", member.ID, "workspace", 0)
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	permissions := effectivePermissions(t, app, memberTok, org.Slug, "workspace", ws.ID)

	assertHasPermissions(t, permissions, "ws:read", "env:create", "conn:create", "policy:read")
	assertMissingPermissions(t, permissions, "org:read", "ws:create")
}

func TestEffectivePermissionsEnvironmentIncludesInheritedOrgAndWorkspacePermissions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-env-owner"), "Owner", "Effective Env")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "eff-env-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")
	envID := createWorkspaceEnvironment(t, app, ownerTok, org.Slug, strconv.FormatInt(ws.ID, 10), "staging")
	orgRoleID := createRoleForTest(t, app, org.ID, nil, "org", "ws:write", "env:write")
	workspaceRoleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", "env:read", "conn:create")

	assert.Equal(t, grantOrgPolicyRole(t, app, ownerTok, org.Slug, orgRoleID, "account", member.ID).StatusCode, http.StatusNoContent)
	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(ws.ID, 10), workspaceRoleID, "account", member.ID, "workspace", 0).StatusCode, http.StatusNoContent)

	permissions := effectivePermissions(t, app, memberTok, org.Slug, "environment", envID)

	assertHasPermissions(t, permissions, "env:write", "env:read", "conn:create")
	assertMissingPermissions(t, permissions, "ws:write", "ws:read")
}

func TestEffectivePermissionsConnectionFiltersToConnectionPermissionsOnly(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-conn-owner"), "Owner", "Effective Conn")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "eff-conn-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")
	wsID := strconv.FormatInt(ws.ID, 10)
	envID := createWorkspaceEnvironment(t, app, ownerTok, org.Slug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, org.Slug, wsID, "primary", &envID)
	orgRoleID := createRoleForTest(t, app, org.ID, nil, "org", "ws:write", "conn:write")
	envRoleID := createRoleForTest(t, app, org.ID, nil, "environment", "env:read", "conn:read")
	connRoleID := createRoleForTest(t, app, org.ID, nil, "connection", "conn:execute")

	assert.Equal(t, grantOrgPolicyRole(t, app, ownerTok, org.Slug, orgRoleID, "account", member.ID).StatusCode, http.StatusNoContent)
	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, wsID, envRoleID, "account", member.ID, "environment", envID).StatusCode, http.StatusNoContent)
	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, wsID, connRoleID, "account", member.ID, "connection", connID).StatusCode, http.StatusNoContent)

	permissions := effectivePermissions(t, app, memberTok, org.Slug, "connection", connID)

	assertHasPermissions(t, permissions, "conn:write", "conn:read", "conn:execute")
	assertMissingPermissions(t, permissions, "ws:write", "env:read")
}

func TestEffectivePermissionsIncludesTeamBindings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-team-owner"), "Owner", "Effective Team")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "eff-team-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Workspace", "")
	team, err := app.db.InsertTeam(context.Background(), org.ID, "effective-team", "Effective Team")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	app.enforcer.InvalidatePrincipals(org.ID, member.ID)
	roleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", "env:create")

	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(ws.ID, 10), roleID, "team", team.ID, "workspace", 0).StatusCode, http.StatusNoContent)

	permissions := effectivePermissions(t, app, memberTok, org.Slug, "workspace", ws.ID)

	assertHasPermissions(t, permissions, "env:create")
}

func TestEffectivePermissionsRejectsInvalidInputsAndCrossOrgResources(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "eff-invalid-owner"), "Owner", "Effective Invalid")
	otherOwner, _, otherOrg := seedOrgOwner(t, app, uniqueEmail(t, "eff-invalid-other"), "Other", "Effective Other")
	otherWorkspace := seedWorkspaceForAccount(t, app, otherOrg, otherOwner, "Other Workspace", "")
	_ = owner

	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{
			name:       "invalid resource type",
			url:        effectivePermissionsURL(org.Slug, "database", 1),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "missing resource id",
			url:        effectivePermissionsURL(org.Slug, "workspace", 0),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "invalid resource id",
			url:        "/api/v1/orgs/" + org.Slug + "/permissions/effective?resource_type=workspace&resource_id=abc",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "cross org workspace",
			url:        effectivePermissionsURL(org.Slug, "workspace", otherWorkspace.ID),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong org id",
			url:        fmt.Sprintf("/api/v1/orgs/%s/permissions/effective?resource_type=org&resource_id=%d", org.Slug, otherOrg.ID),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := send(t, newAuthRequest(t, http.MethodGet, tt.url, nil, ownerTok), app.routes())
			assert.Equal(t, res.StatusCode, tt.wantStatus)
		})
	}
}
