package access_test

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

// findRoleID returns the ID of the named org-level role in the given org, fatal if not found.
func findRoleID(t *testing.T, db *database.DB, orgID int64, name string) int64 {
	t.Helper()
	roles, err := db.ListOrgRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Name == name {
			return r.ID
		}
	}
	t.Fatalf("org role %q not found in org %d", name, orgID)
	return 0
}

// findWorkspaceRoleID returns the ID of the named workspace-scoped role, fatal if not found.
func findWorkspaceRoleID(t *testing.T, db *database.DB, orgID, workspaceID int64, name string) int64 {
	t.Helper()
	roles, err := db.ListWorkspaceRoles(context.Background(), orgID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Name == name {
			return r.ID
		}
	}
	t.Fatalf("workspace role %q not found in org %d workspace %d", name, orgID, workspaceID)
	return 0
}

// newMember creates an account, adds it to the org, and returns its ID.
func newMember(t *testing.T, db *database.DB, orgID int64, email string) int64 {
	t.Helper()
	acct, err := db.InsertAccount(context.Background(), email, email, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, acct.ID); err != nil {
		t.Fatal(err)
	}
	return acct.ID
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource hierarchy / transitive inheritance
// ─────────────────────────────────────────────────────────────────────────────

// TestConnectionInheritsOrgRoleViaHierarchy verifies that an org-level owner role
// binding flows through workspace → connection via the resource_hierarchy table.
func TestConnectionInheritsOrgRoleViaHierarchy(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "conn-hier")
	ctx := context.Background()

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "MyConn", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	// Owner's org-level role includes conn:execute and should propagate.
	if !e.Can(ctx, ownerID, orgID, "org", "connection", conn.ID, access.PermConnExecute) {
		t.Error("owner should inherit conn:execute at connection scope via org→workspace→connection hierarchy")
	}
	if !e.Can(ctx, ownerID, orgID, "org", "connection", conn.ID, access.PermConnRead) {
		t.Error("owner should inherit conn:read at connection scope")
	}
}

// TestEnvironmentInheritsOrgRoleViaHierarchy verifies that an org-level role flows
// down to environments through the hierarchy.
func TestEnvironmentInheritsOrgRoleViaHierarchy(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "env-hier")
	ctx := context.Background()

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	if err != nil {
		t.Fatal(err)
	}

	if !e.Can(ctx, ownerID, orgID, "org", "environment", env.ID, access.PermEnvDeploy) {
		t.Error("owner should inherit env:deploy at environment scope via hierarchy")
	}
}

// TestWorkspaceRoleBindingFlowsToConnection verifies that a role bound at workspace scope
// grants its permissions on connections within that workspace.
func TestWorkspaceRoleBindingFlowsToConnection(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-to-conn")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "ws-to-conn@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "C", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	// Create a workspace-scope custom role with conn:execute.
	roleID, err := e.CreateRole(ctx, orgID, nil, "conn-runner", "", "workspace", []string{access.PermConnExecute})
	if err != nil {
		t.Fatal(err)
	}

	// Bind the custom role to the member at workspace scope.
	if err = e.BindRole(ctx, orgID, roleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	// Member should now have conn:execute on connections inside ws.
	if !e.Can(ctx, memberID, orgID, "org", "connection", conn.ID, access.PermConnExecute) {
		t.Error("workspace role binding should flow to connection inside that workspace")
	}
	// But not permissions the role doesn't include.
	if e.Can(ctx, memberID, orgID, "org", "connection", conn.ID, access.PermConnWrite) {
		t.Error("member should NOT have conn:write — it is not in the role")
	}
}

// TestWorkspaceRoleBindingFlowsToEnvironment verifies that a workspace-scoped role
// binding grants permissions on environments within that workspace.
func TestWorkspaceRoleBindingFlowsToEnvironment(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-to-env")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "ws-to-env@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.InsertEnvironment(context.Background(), ws.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	roleID, err := e.CreateRole(ctx, orgID, nil, "deployer", "", "workspace", []string{access.PermEnvDeploy})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.BindRole(ctx, orgID, roleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	if !e.Can(ctx, memberID, orgID, "org", "environment", env.ID, access.PermEnvDeploy) {
		t.Error("workspace role binding should flow to environment inside that workspace")
	}
}

// TestDirectPermissionOnWorkspaceFlowsToConnection verifies that a direct permission
// binding placed on a workspace propagates to connections beneath it.
func TestDirectPermissionOnWorkspaceFlowsToConnection(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-perm-conn")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "ws-perm-conn@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "C", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-conn-exec", "workspace", []string{access.PermConnExecute}, "account", memberID, "workspace", ws.ID, ownerID)

	if !e.Can(ctx, memberID, orgID, "org", "connection", conn.ID, access.PermConnExecute) {
		t.Error("direct permission on workspace should flow to connection via hierarchy")
	}
}

// TestDirectPermissionOnWorkspaceFlowsToEnvironment is the same as above for environments.
func TestDirectPermissionOnWorkspaceFlowsToEnvironment(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-perm-env")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "ws-perm-env@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.InsertEnvironment(context.Background(), ws.ID, "dev", "")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-env-deploy", "workspace", []string{access.PermEnvDeploy}, "account", memberID, "workspace", ws.ID, ownerID)

	if !e.Can(ctx, memberID, orgID, "org", "environment", env.ID, access.PermEnvDeploy) {
		t.Error("direct permission on workspace should flow to environment via hierarchy")
	}
}

// TestOrgRoleFlowsToAllWorkspaces verifies that an org-level role binding grants
// workspace permissions on every workspace in the org.
func TestOrgRoleFlowsToAllWorkspaces(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "org-all-ws")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "org-all-ws@example.com")

	ws1, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS1", "")
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS2", "")
	if err != nil {
		t.Fatal(err)
	}

	adminRoleID := findRoleID(t, db, orgID, "admin")
	if err = e.BindRole(ctx, orgID, adminRoleID, "account", memberID, "org", orgID, ownerID); err != nil {
		t.Fatal(err)
	}

	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws1.ID, access.PermWsWrite) {
		t.Error("admin role at org should grant ws:write on ws1")
	}
	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws2.ID, access.PermWsWrite) {
		t.Error("admin role at org should grant ws:write on ws2")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Isolation: bindings must NOT leak to sibling or parent resources
// ─────────────────────────────────────────────────────────────────────────────

// TestConnectionBindingDoesNotLeakToSibling verifies that a permission on conn1
// does not grant access on conn2 in the same workspace.
func TestConnectionBindingDoesNotLeakToSibling(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "conn-sib")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "conn-sib@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	conn1, err := db.InsertConnection(context.Background(), ws.ID, nil, "C1", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}
	conn2, err := db.InsertConnection(context.Background(), ws.ID, nil, "C2", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, nil, "conn-exec-only", "connection", []string{access.PermConnExecute}, "account", memberID, "connection", conn1.ID, ownerID)

	if !e.Can(ctx, memberID, orgID, "org", "connection", conn1.ID, access.PermConnExecute) {
		t.Error("member should have conn:execute on conn1")
	}
	if e.Can(ctx, memberID, orgID, "org", "connection", conn2.ID, access.PermConnExecute) {
		t.Error("conn1 binding must NOT leak to sibling conn2")
	}
}

// TestEnvironmentBindingDoesNotLeakToSibling verifies that a permission on env1
// does not grant access on env2 in the same workspace.
func TestEnvironmentBindingDoesNotLeakToSibling(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "env-sib")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "env-sib@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	env1, err := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	if err != nil {
		t.Fatal(err)
	}
	env2, err := db.InsertEnvironment(context.Background(), ws.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, nil, "env-deploy-only", "environment", []string{access.PermEnvDeploy}, "account", memberID, "environment", env1.ID, ownerID)

	if !e.Can(ctx, memberID, orgID, "org", "environment", env1.ID, access.PermEnvDeploy) {
		t.Error("member should have env:deploy on env1")
	}
	if e.Can(ctx, memberID, orgID, "org", "environment", env2.ID, access.PermEnvDeploy) {
		t.Error("env1 binding must NOT leak to sibling env2")
	}
}

// TestWorkspaceBindingDoesNotLeakToSiblingWorkspace verifies that a binding on ws1
// does not affect ws2 in the same org.
func TestWorkspaceBindingDoesNotLeakToSiblingWorkspace(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-sib")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "ws-sib@example.com")

	ws1, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS1", "")
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS2", "")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, &ws1.ID, "ws-write-only", "workspace", []string{access.PermWsWrite}, "account", memberID, "workspace", ws1.ID, ownerID)

	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws1.ID, access.PermWsWrite) {
		t.Error("member should have ws:write on ws1")
	}
	if e.Can(ctx, memberID, orgID, "org", "workspace", ws2.ID, access.PermWsWrite) {
		t.Error("ws1 binding must NOT leak to sibling ws2")
	}
}

// TestChildBindingDoesNotPropagateToParent verifies that a permission binding on a
// connection does NOT grant access on the parent workspace (hierarchy is parent→child only).
func TestChildBindingDoesNotPropagateToParent(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "child-no-parent")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "child-no-parent@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "C", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	// Grant a valid connection-scoped permission. Child bindings still must not
	// bubble up into parent workspace permissions.
	createRoleAndBind(t, e, db, orgID, nil, "conn-read-only", "connection", []string{access.PermConnRead}, "account", memberID, "connection", conn.ID, ownerID)

	// conn → ws: child's binding should NOT appear when checking workspace.
	if e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsWrite) {
		t.Error("connection binding must NOT propagate up to the parent workspace")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Cross-org isolation
// ─────────────────────────────────────────────────────────────────────────────

// TestCrossOrgIsolation verifies that a binding in orgA does not grant any access in orgB.
func TestCrossOrgIsolation(t *testing.T) {
	e, db := newTestEnforcer(t)
	ctx := context.Background()

	orgAID, ownerAID := seedOrg(t, db, e, "iso-orgA")
	orgBID, _ := seedOrg(t, db, e, "iso-orgB")

	// ownerA has full access to orgA resources.
	if !e.Can(ctx, ownerAID, orgAID, "org", "org", orgAID, access.PermOrgWrite) {
		t.Error("ownerA should have org:write in orgA")
	}
	// ownerA has NO access to orgB resources.
	if e.Can(ctx, ownerAID, orgBID, "org", "org", orgBID, access.PermOrgRead) {
		t.Error("ownerA binding in orgA must NOT grant access in orgB")
	}

	wsB, err := db.InsertWorkspace(context.Background(), &orgBID, "org", orgBID, "WS-B", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Can(ctx, ownerAID, orgBID, "org", "workspace", wsB.ID, access.PermWsRead) {
		t.Error("ownerA must NOT have ws:read on orgB workspaces")
	}
}

// TestAccountWithNoBindingDenied verifies that an account that is an org member but
// has no role or permission bindings cannot perform any org operations.
func TestAccountWithNoBindingDenied(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "no-bind")
	ctx := context.Background()

	// Account is in the org but has no role binding.
	noBindID := newMember(t, db, orgID, "no-bind@example.com")

	for _, perm := range []string{
		access.PermOrgRead, access.PermOrgWrite, access.PermWsRead, access.PermWsWrite,
		access.PermEnvRead, access.PermConnExecute, access.PermPolicyRead,
	} {
		if e.Can(ctx, noBindID, orgID, "org", "org", orgID, perm) {
			t.Errorf("unbound member should NOT have permission %q", perm)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Invalid / unknown permissions
// ─────────────────────────────────────────────────────────────────────────────

// TestGrantUnknownPermissionRejected verifies that CreateRole rejects an
// unrecognised permission string.
func TestGrantUnknownPermissionRejected(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "unk-perm")
	ctx := context.Background()

	_, err := e.CreateRole(ctx, orgID, nil, "invalid-role", "", "org", []string{"invalid:permission"})
	if err == nil {
		t.Error("expected error for unknown permission, got nil")
	}
}

// TestCreateRoleUnknownPermInBatchRejected verifies that if any permission in a
// role definition is invalid, the whole call fails and no role is inserted.
func TestCreateRoleUnknownPermInBatchRejected(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "unk-batch")
	ctx := context.Background()

	_, err := e.CreateRole(ctx, orgID, nil, "bad-batch", "", "workspace", []string{access.PermWsRead, "completely:bogus"})
	if err == nil {
		t.Error("expected error when role contains unknown permission")
	}

	roles, err := db.ListRoles(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if role.Name == "bad-batch" {
			t.Fatal("expected invalid role creation to leave no inserted role")
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Custom roles
// ─────────────────────────────────────────────────────────────────────────────

// TestCustomRoleAtWorkspaceScope creates a workspace-scope custom role, binds it, and
// verifies the correct permissions are granted — and org-only perms are not.
func TestCustomRoleAtWorkspaceScope(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "custom-ws")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "custom-ws@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	roleID, err := e.CreateRole(ctx, orgID, nil, "ws-viewer", "read-only", "workspace",
		[]string{access.PermWsRead, access.PermEnvRead, access.PermConnRead})
	if err != nil {
		t.Fatal(err)
	}

	if err = e.BindRole(ctx, orgID, roleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	// Permitted by the custom role.
	for _, perm := range []string{access.PermWsRead, access.PermEnvRead, access.PermConnRead} {
		if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, perm) {
			t.Errorf("member should have %q via custom ws-viewer role", perm)
		}
	}
	// Not in the role.
	for _, perm := range []string{access.PermWsWrite, access.PermConnExecute, access.PermOrgRead} {
		if e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, perm) {
			t.Errorf("member should NOT have %q via ws-viewer role", perm)
		}
	}
}

// TestCreateRoleWithOrgPermForWorkspaceScopeFails verifies that creating a workspace-scope
// role with an org-only permission is rejected.
func TestCreateRoleWithOrgPermForWorkspaceScopeFails(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "ws-org-perm")
	ctx := context.Background()

	_, err := e.CreateRole(ctx, orgID, nil, "bad", "", "workspace", []string{access.PermOrgWrite})
	if err == nil {
		t.Error("expected error: org:write is not valid for workspace scope")
	}
}

// TestDeleteCustomRoleRevokesAccess verifies that deleting a custom role cascades to
// remove its bindings, revoking access.
func TestDeleteCustomRoleRevokesAccess(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "del-custom")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "del-custom@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	roleID, err := e.CreateRole(ctx, orgID, nil, "temp-role", "", "workspace", []string{access.PermWsWrite})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.BindRole(ctx, orgID, roleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsWrite) {
		t.Fatal("precondition: member should have ws:write before role deletion")
	}

	if err = e.DeleteRole(ctx, roleID, orgID); err != nil {
		t.Fatal(err)
	}

	if e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsWrite) {
		t.Error("member should NOT have ws:write after the role is deleted (binding cascaded away)")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Teams
// ─────────────────────────────────────────────────────────────────────────────

// TestAccountInMultipleTeamsUnionPermissions verifies that an account in two teams
// receives the union of both teams' permissions.
func TestAccountInMultipleTeamsUnionPermissions(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "multi-team")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "multi-team@example.com")

	team1, err := db.InsertTeam(context.Background(), orgID, "readers", "Readers")
	if err != nil {
		t.Fatal(err)
	}
	team2, err := db.InsertTeam(context.Background(), orgID, "writers", "Writers")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team1.ID, memberID); err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team2.ID, memberID); err != nil {
		t.Fatal(err)
	}

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	// team1 gets ws:read; team2 gets ws:write.
	readRole, err := e.CreateRole(ctx, orgID, nil, "ws-read-only", "", "workspace", []string{access.PermWsRead})
	if err != nil {
		t.Fatal(err)
	}
	writeRole, err := e.CreateRole(ctx, orgID, nil, "ws-write-only", "", "workspace", []string{access.PermWsWrite})
	if err != nil {
		t.Fatal(err)
	}

	if err = e.BindRole(ctx, orgID, readRole, "team", team1.ID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}
	if err = e.BindRole(ctx, orgID, writeRole, "team", team2.ID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	// Member is in both teams — should have union of permissions.
	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsRead) {
		t.Error("member should have ws:read via team1")
	}
	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsWrite) {
		t.Error("member should have ws:write via team2")
	}
}

// TestTeamBindingAtWorkspaceScope verifies a team role binding at workspace scope grants
// permissions to all team members.
func TestTeamBindingAtWorkspaceScope(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "team-ws")
	ctx := context.Background()

	alice := newMember(t, db, orgID, "alice@team-ws.com")
	bob := newMember(t, db, orgID, "bob@team-ws.com")
	charlie := newMember(t, db, orgID, "charlie@team-ws.com") // not in team

	team, err := db.InsertTeam(context.Background(), orgID, "frontend", "Frontend")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team.ID, alice); err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team.ID, bob); err != nil {
		t.Fatal(err)
	}

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	roleID, err := e.CreateRole(ctx, orgID, nil, "frontend-role", "", "workspace", []string{access.PermWsRead, access.PermEnvRead})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.BindRole(ctx, orgID, roleID, "team", team.ID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	if !e.Can(ctx, alice, orgID, "org", "workspace", ws.ID, access.PermWsRead) {
		t.Error("alice (team member) should have ws:read")
	}
	if !e.Can(ctx, bob, orgID, "org", "workspace", ws.ID, access.PermWsRead) {
		t.Error("bob (team member) should have ws:read")
	}
	if e.Can(ctx, charlie, orgID, "org", "workspace", ws.ID, access.PermWsRead) {
		t.Error("charlie (non-member) should NOT have ws:read via team binding")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Revocation
// ─────────────────────────────────────────────────────────────────────────────

// TestUnbindRoleTargeted verifies that revoking one role binding leaves
// other bindings for the same subject intact.
func TestUnbindRoleTargeted(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "revoke-tgt")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "revoke-tgt@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	roleReadID := createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-read-only", "workspace", []string{access.PermWsRead}, "account", memberID, "workspace", ws.ID, ownerID)
	roleWriteID := createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-write-only-revoke", "workspace", []string{access.PermWsWrite}, "account", memberID, "workspace", ws.ID, ownerID)

	// Find the ws:write binding ID.
	rbs := listRoleBindings(t, db, orgID, "workspace", ws.ID)
	var writeBindingID int64
	for _, rb := range rbs {
		if rb.RoleID == roleWriteID && rb.SubjectID == memberID {
			writeBindingID = rb.ID
			break
		}
	}
	if writeBindingID == 0 {
		t.Fatal("ws:write role binding not found")
	}

	if err = e.UnbindRole(ctx, writeBindingID, orgID); err != nil {
		t.Fatal(err)
	}

	// ws:write revoked.
	if e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsWrite) {
		t.Error("member should NOT have ws:write after revocation")
	}
	// ws:read still intact.
	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsRead) {
		t.Error("member should still have ws:read — only ws:write was revoked")
	}
	if roleReadID == 0 {
		t.Fatal("expected ws:read role ID to be set")
	}
}

// TestUnbindRoleWrongOrgNoOpForScopedBinding verifies that UnbindRole with the wrong org ID
// does not remove the binding (the WHERE clause includes org_id).
func TestUnbindRoleWrongOrgNoOpForScopedBinding(t *testing.T) {
	e, db := newTestEnforcer(t)
	ctx := context.Background()

	orgID, ownerID := seedOrg(t, db, e, "revoke-wrong-org")
	otherOrgID, _ := seedOrg(t, db, e, "revoke-other-org")

	memberID := newMember(t, db, orgID, "revoke-wrong@example.com")
	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-read-wrong-org", "workspace", []string{access.PermWsRead}, "account", memberID, "workspace", ws.ID, ownerID)

	rbs := listRoleBindings(t, db, orgID, "workspace", ws.ID)
	if len(rbs) == 0 {
		t.Fatal("expected a binding")
	}

	// Attempt to revoke using the wrong org ID — should be a no-op.
	if err = e.UnbindRole(ctx, rbs[0].ID, otherOrgID); err != nil {
		t.Fatal(err)
	}

	// Original binding should still be in place.
	if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsRead) {
		t.Error("binding should still exist after wrong-org revoke attempt")
	}
}

// TestUnbindRoleWrongOrgNoOp verifies that UnbindRole with the wrong org ID is a no-op.
func TestUnbindRoleWrongOrgNoOp(t *testing.T) {
	e, db := newTestEnforcer(t)
	ctx := context.Background()

	orgID, ownerID := seedOrg(t, db, e, "unbind-wrong-org")
	otherOrgID, _ := seedOrg(t, db, e, "unbind-other-org")

	memberID := newMember(t, db, orgID, "unbind-wrong@example.com")
	adminRoleID := findRoleID(t, db, orgID, "admin")

	if err := e.BindRole(ctx, orgID, adminRoleID, "account", memberID, "org", orgID, ownerID); err != nil {
		t.Fatal(err)
	}

	rbs := listRoleBindings(t, db, orgID, "org", orgID)
	var bindingID int64
	for _, b := range rbs {
		if b.SubjectType == "account" && b.SubjectID == memberID {
			bindingID = b.ID
			break
		}
	}
	if bindingID == 0 {
		t.Fatal("binding not found")
	}

	// Attempt to unbind using the wrong org.
	if err := e.UnbindRole(ctx, bindingID, otherOrgID); err != nil {
		t.Fatal(err)
	}

	if !e.Can(ctx, memberID, orgID, "org", "org", orgID, access.PermOrgInvite) {
		t.Error("binding should still exist after wrong-org unbind attempt")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Idempotency
// ─────────────────────────────────────────────────────────────────────────────

// TestBindRoleIdempotent verifies that binding the same role twice results in only
// one binding row (ON CONFLICT DO NOTHING).
func TestBindRoleIdempotent(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "idem-role")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "idem-role@example.com")
	adminRoleID := findRoleID(t, db, orgID, "admin")

	if err := e.BindRole(ctx, orgID, adminRoleID, "account", memberID, "org", orgID, ownerID); err != nil {
		t.Fatal(err)
	}
	// Second bind — should not error or duplicate.
	if err := e.BindRole(ctx, orgID, adminRoleID, "account", memberID, "org", orgID, ownerID); err != nil {
		t.Fatal(err)
	}

	rbs := listRoleBindings(t, db, orgID, "org", orgID)
	count := 0
	for _, b := range rbs {
		if b.SubjectType == "account" && b.SubjectID == memberID && b.RoleID == adminRoleID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 binding after two identical BindRole calls, got %d", count)
	}
}

// TestBindScopedRoleIdempotent verifies that binding the same scoped role twice
// results in only one binding row.
func TestBindScopedRoleIdempotent(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "idem-perm")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "idem-perm@example.com")
	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	roleID, err := e.CreateRole(ctx, orgID, &ws.ID, "idem-ws-read", "", "workspace", []string{access.PermWsRead})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.BindRole(ctx, orgID, roleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}
	if err = e.BindRole(ctx, orgID, roleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	rbs := listRoleBindings(t, db, orgID, "workspace", ws.ID)
	count := 0
	for _, rb := range rbs {
		if rb.RoleID == roleID && rb.SubjectID == memberID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 ws:read role binding after two identical binds, got %d", count)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GrantPermissions (batch)
// ─────────────────────────────────────────────────────────────────────────────

// TestMultiPermissionRoleAllEnforced verifies that a role containing multiple
// permissions enforces all of them.
func TestMultiPermissionRoleAllEnforced(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "batch-perm")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "batch-perm@example.com")
	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}

	perms := []string{access.PermWsRead, access.PermWsWrite, access.PermEnvRead}
	createRoleAndBind(t, e, db, orgID, &ws.ID, "multi-ws-role", "workspace", perms, "account", memberID, "workspace", ws.ID, ownerID)

	for _, p := range perms {
		if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, p) {
			t.Errorf("member should have %q after batch grant", p)
		}
	}

	// A permission not in the batch should not be granted.
	if e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, access.PermWsDelete) {
		t.Error("member should NOT have ws:delete — it was not in the batch")
	}

	rbs := listRoleBindings(t, db, orgID, "workspace", ws.ID)
	if len(rbs) != 1 {
		t.Errorf("expected 1 role binding, got %d", len(rbs))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Builtin role permission coverage
// ─────────────────────────────────────────────────────────────────────────────

// TestAdminRoleDoesNotHaveOwnerOnlyPermissions verifies that the admin builtin role
// does not include owner-exclusive permissions.
func TestAdminRoleDoesNotHaveOwnerOnlyPermissions(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "admin-no-owner")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "admin-no-owner@example.com")
	adminRoleID := findRoleID(t, db, orgID, "admin")

	if err := e.BindRole(ctx, orgID, adminRoleID, "account", memberID, "org", orgID, ownerID); err != nil {
		t.Fatal(err)
	}

	ownerOnly := []string{access.PermOrgTransferOwnership, access.PermOrgDelete}
	for _, p := range ownerOnly {
		if e.Can(ctx, memberID, orgID, "org", "org", orgID, p) {
			t.Errorf("admin should NOT have owner-only permission %q", p)
		}
	}
}

// TestWsMemberRoleBasicPermissions verifies the ws:member builtin role grants exactly its
// expected permissions within the workspace and nothing more.
func TestWsMemberRoleBasicPermissions(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "wsmember-basic")
	ctx := context.Background()

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SeedWorkspace(ctx, orgID, ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	memberID := newMember(t, db, orgID, "wsmember-basic@example.com")
	wsMemberRoleID := findWorkspaceRoleID(t, db, orgID, ws.ID, "ws:member")

	if err = e.BindRole(ctx, orgID, wsMemberRoleID, "account", memberID, "workspace", ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	// Permissions ws:member should have at the workspace level.
	allowed := []string{access.PermWsRead, access.PermEnvRead, access.PermConnRead, access.PermConnDQL}
	for _, p := range allowed {
		if !e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, p) {
			t.Errorf("ws:member role should have %q", p)
		}
	}

	// Permissions ws:member should NOT have.
	denied := []string{
		access.PermOrgWrite, access.PermOrgInvite, access.PermWsWrite, access.PermWsDelete,
		access.PermConnWrite, access.PermConnDelete, access.PermPolicyModify,
	}
	for _, p := range denied {
		if e.Can(ctx, memberID, orgID, "org", "workspace", ws.ID, p) {
			t.Errorf("ws:member role should NOT have %q", p)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SeedOrg idempotency
// ─────────────────────────────────────────────────────────────────────────────

// TestSeedOrgIdempotent verifies that calling SeedOrg twice on the same org does not
// error or duplicate roles (ON CONFLICT DO UPDATE).
func TestSeedOrgIdempotent(t *testing.T) {
	e, db := newTestEnforcer(t)
	ctx := context.Background()

	org, err := db.InsertOrg(context.Background(), "idem-org", "Idem Org")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := db.InsertAccount(context.Background(), "idem-seed@example.com", "Owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), org.ID, owner.ID); err != nil {
		t.Fatal(err)
	}

	if err = e.SeedOrg(ctx, org.ID, owner.ID); err != nil {
		t.Fatalf("first SeedOrg: %v", err)
	}
	if err = e.SeedOrg(ctx, org.ID, owner.ID); err != nil {
		t.Fatalf("second SeedOrg should be idempotent: %v", err)
	}

	roles, err := db.ListRoles(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	builtinCount := 0
	for _, r := range roles {
		if r.IsBuiltin {
			builtinCount++
		}
	}
	if builtinCount != 2 {
		t.Errorf("expected exactly 2 builtin org roles after double seed, got %d", builtinCount)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Ancestry cache invalidation after resource deletion
// ─────────────────────────────────────────────────────────────────────────────

// TestAncestoryCacheInvalidatedAfterDelete verifies that after a workspace is deleted
// and InvalidateAncestry is called, the enforcer no longer uses stale cached ancestry
// for a connection that was in that workspace.
func TestAncestoryCacheInvalidatedAfterDelete(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "anc-cache")
	ctx := context.Background()

	memberID := newMember(t, db, orgID, "anc-cache@example.com")

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "WS", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "C", "postgres", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	// Grant conn:execute on the workspace (flows to connection).
	createRoleAndBind(t, e, db, orgID, &ws.ID, "anc-cache-conn", "workspace", []string{access.PermConnExecute}, "account", memberID, "workspace", ws.ID, ownerID)

	// Prime the ancestry cache.
	if !e.Can(ctx, memberID, orgID, "org", "connection", conn.ID, access.PermConnExecute) {
		t.Fatal("precondition: member should have conn:execute before deletion")
	}

	// Delete the workspace and invalidate.
	if err = db.DeleteWorkspace(context.Background(), ws.ID); err != nil {
		t.Fatal(err)
	}
	e.InvalidateAncestry("connection", conn.ID)

	// After deletion and cache invalidation, the binding at workspace scope is gone
	// (workspace was deleted, cascading the role_bindings row).
	// The org-level ancestry still exists, but there is no org-level binding for this member.
	if e.Can(ctx, memberID, orgID, "org", "connection", conn.ID, access.PermConnExecute) {
		t.Error("member should NOT have conn:execute after workspace and its bindings were deleted")
	}
}
