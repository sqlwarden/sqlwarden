package access_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

// newTestDB creates an isolated postgres database for a single test.
func newTestDB(t *testing.T) *database.DB {
	t.Helper()

	dbName := fmt.Sprintf("test_access_%d", atomic.AddUint64(&pgTestDBCounter, 1))
	pgTemplateCloneMu.Lock()
	_, err := pgAdminDB.ExecContext(context.Background(),
		fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", dbName, pgTemplateDBName))
	pgTemplateCloneMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	dsn := trimPostgresScheme(dsnWithDatabase("postgres://"+pgAdminDSN, dbName))
	db, err := database.New("postgres", dsn, nilLogger(), false)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)

	t.Cleanup(func() {
		db.Close()
		pgTemplateCloneMu.Lock()
		defer pgTemplateCloneMu.Unlock()
		_, _ = pgAdminDB.ExecContext(context.Background(),
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
			dbName)
		_, err := pgAdminDB.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		if err != nil {
			t.Error(err)
		}
	})

	return db
}

// newTestEnforcer returns an Enforcer backed by a fresh isolated schema.
func newTestEnforcer(t *testing.T) (*access.Enforcer, *database.DB) {
	t.Helper()
	db := newTestDB(t)
	e, err := access.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	return e, db
}

// seedOrg creates an org and runs SeedOrg; returns orgID and ownerAccountID.
func seedOrg(t *testing.T, db *database.DB, e *access.Enforcer, suffix string) (orgID, ownerID int64) {
	t.Helper()
	ctx := context.Background()

	suffix = strings.ReplaceAll(suffix, " ", "-")
	org, err := db.InsertOrg(context.Background(), "test-org-"+suffix, "Test Org "+suffix)
	if err != nil {
		t.Fatal(err)
	}

	owner, err := db.InsertAccount(context.Background(), "owner-"+suffix+"@example.com", "Owner "+suffix, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), org.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.SeedOrg(ctx, org.ID, owner.ID); err != nil {
		t.Fatalf("SeedOrg: %v", err)
	}
	return org.ID, owner.ID
}

func listRoleBindings(t *testing.T, db *database.DB, orgID int64, resourceType string, resourceID int64) []database.RoleBinding {
	t.Helper()

	var bindings []database.RoleBinding
	err := db.NewSelect().Model(&bindings).
		Where("org_id = ? AND resource_type = ? AND resource_id = ?", orgID, resourceType, resourceID).
		Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return bindings
}

func createRoleAndBind(t *testing.T, e *access.Enforcer, db *database.DB, orgID int64, workspaceID *int64, roleName, scopeType string, permissions []string, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) int64 {
	t.Helper()

	roleID, err := e.CreateRole(context.Background(), orgID, workspaceID, roleName, roleName+" description", scopeType, permissions)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.BindRole(context.Background(), orgID, roleID, subjectType, subjectID, resourceType, resourceID, grantedBy); err != nil {
		t.Fatal(err)
	}
	return roleID
}

// TestSeedOrgCreatesBuiltinRoles verifies that SeedOrg seeds the owner and admin builtin roles.
func TestSeedOrgCreatesBuiltinRoles(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "seed")

	roles, err := db.ListOrgRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]bool)
	for _, r := range roles {
		if r.IsBuiltin {
			byName[r.Name] = true
		}
	}
	for _, name := range []string{"owner", "admin"} {
		if !byName[name] {
			t.Errorf("expected builtin role %q to exist after SeedOrg", name)
		}
	}
	if byName["member"] {
		t.Error("member role should not exist at org level after SeedOrg")
	}
}

// TestCanOwnerHasOrgPermissions verifies that the owner can perform org-level operations.
func TestCanOwnerHasOrgPermissions(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "owner-perms")
	ctx := context.Background()

	perms := []string{"org:read", "org:write", "org:invite", "org:transfer_ownership", "policy:read", "policy:modify"}
	for _, p := range perms {
		if !e.Can(ctx, ownerID, orgID, "org", "org", orgID, p) {
			t.Errorf("owner should have permission %q", p)
		}
	}
}

// TestCanWsMemberLacksAdminPermissions verifies that a ws:member cannot perform admin operations.
func TestCanWsMemberLacksAdminPermissions(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "member-perms")
	ctx := context.Background()

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SeedWorkspace(ctx, orgID, ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	member, err := db.InsertAccount(context.Background(), "member-perms@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	// Bind ws:member role at workspace level.
	roles, err := db.ListWorkspaceRoles(context.Background(), orgID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Name == "ws:member" && r.IsBuiltin {
			if err = e.BindRole(ctx, orgID, r.ID, "account", member.ID, "workspace", ws.ID, ownerID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	// ws:member should NOT have org-level admin permissions.
	restricted := []string{"org:write", "org:invite", "org:transfer_ownership", "ws:delete"}
	for _, p := range restricted {
		if e.Can(ctx, member.ID, orgID, "org", "workspace", ws.ID, p) {
			t.Errorf("ws:member should NOT have permission %q", p)
		}
	}

	// ws:member should have ws:read via workspace-level binding.
	if !e.Can(ctx, member.ID, orgID, "org", "workspace", ws.ID, "ws:read") {
		t.Error("ws:member should have ws:read at workspace level")
	}
}

// TestCanSpaceOwnerShortCircuits verifies ownerType=="space" always grants permission.
func TestCanSpaceOwnerShortCircuits(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "space-sc")
	ctx := context.Background()

	// Even a made-up permission is granted for ownerType=="space".
	if !e.Can(ctx, ownerID, orgID, "space", "workspace", 999, "org:transfer_ownership") {
		t.Error("ownerType=space should short-circuit to true")
	}
}

// TestCanWorkspaceInheritsOrgRole verifies that an org-level role binding grants workspace permissions.
func TestCanWorkspaceInheritsOrgRole(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-inherit")
	ctx := context.Background()

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "MyWS", "")
	if err != nil {
		t.Fatal(err)
	}

	// Owner's org-level role should grant ws:write at workspace scope.
	if !e.Can(ctx, ownerID, orgID, "org", "workspace", ws.ID, "ws:write") {
		t.Error("owner should inherit ws:write at workspace scope via org role")
	}
}

// TestCanViaTeamMembership verifies that team-based role bindings grant permissions.
func TestCanViaTeamMembership(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "team-rbac")
	ctx := context.Background()

	member, err := db.InsertAccount(context.Background(), "team-member@example.com", "Team Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	team, err := db.InsertTeam(context.Background(), orgID, "devs", "Developers")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	// Bind admin role to the team.
	roles, err := db.ListRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Name == "admin" && r.IsBuiltin {
			if err = e.BindRole(ctx, orgID, r.ID, "team", team.ID, "org", orgID, ownerID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	// Member is in the team, so should get admin's permissions.
	if !e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("team member should have org:invite via team admin role")
	}
	// But still not owner-only permissions.
	if e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:transfer_ownership") {
		t.Error("team member should NOT have org:transfer_ownership")
	}
}

// TestCanScopedRoleBinding verifies that a scoped custom role grants access.
func TestCanScopedRoleBinding(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "direct-perm")
	ctx := context.Background()

	member, err := db.InsertAccount(context.Background(), "direct-perm@example.com", "Direct", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, "DirectWS", "")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-conn-execute", "workspace", []string{"conn:execute"}, "account", member.ID, "workspace", ws.ID, ownerID)

	if !e.Can(ctx, member.ID, orgID, "org", "workspace", ws.ID, "conn:execute") {
		t.Error("member should have conn:execute via scoped role binding")
	}
	// Should not have other permissions.
	if e.Can(ctx, member.ID, orgID, "org", "workspace", ws.ID, "ws:write") {
		t.Error("member should NOT have ws:write without a binding")
	}
}

// TestCreateRoleValidScope verifies CreateRole accepts valid scope+permissions.
func TestCreateRoleValidScope(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "role-valid")
	ctx := context.Background()

	id, err := e.CreateRole(ctx, orgID, nil, "viewer", "Read-only viewer", "workspace", []string{"ws:read", "env:read"})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero role ID")
	}
}

// TestCreateRoleInvalidPermissionForScope verifies CreateRole rejects out-of-scope permissions.
func TestCreateRoleInvalidPermissionForScope(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "role-invalid")
	ctx := context.Background()

	_, err := e.CreateRole(ctx, orgID, nil, "bad-role", "", "connection", []string{"org:write"})
	if err == nil {
		t.Error("expected error for out-of-scope permission, got nil")
	}
}

// TestDeleteRoleRemovesBindings verifies DeleteRole returns error for builtin roles.
func TestDeleteBuiltinRoleRejected(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, _ := seedOrg(t, db, e, "del-builtin")
	ctx := context.Background()

	roles, err := db.ListRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	var builtinID int64
	for _, r := range roles {
		if r.IsBuiltin {
			builtinID = r.ID
			break
		}
	}
	if builtinID == 0 {
		t.Skip("no builtin roles")
	}

	if err = e.DeleteRole(ctx, builtinID, orgID); err == nil {
		t.Error("expected error when deleting builtin role")
	}
}

// TestCacheInvalidationOnRoleChange verifies that InvalidateOrgPolicy causes re-evaluation.
func TestCacheInvalidationOnRoleChange(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "cache-inv")
	ctx := context.Background()

	member, err := db.InsertAccount(context.Background(), "cache-member@example.com", "CacheMember", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	// No binding yet — should be false.
	if e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("member should not have org:invite before binding")
	}

	// Bind admin role (which includes org:invite).
	roles, err := db.ListRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Name == "admin" && r.IsBuiltin {
			if err = e.BindRole(ctx, orgID, r.ID, "account", member.ID, "org", orgID, ownerID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	// After BindRole (which calls InvalidateOrgPolicy), Can should re-evaluate.
	if !e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("member should have org:invite after admin role binding")
	}
}

// TestPrincipalCacheInvalidation verifies InvalidatePrincipals clears team memberships.
func TestPrincipalCacheInvalidation(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "principal-cache")
	ctx := context.Background()

	member, err := db.InsertAccount(context.Background(), "principal-member@example.com", "PMember", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	team, err := db.InsertTeam(context.Background(), orgID, "eng", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	// Bind admin role to team.
	roles, err := db.ListRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		if r.Name == "admin" && r.IsBuiltin {
			if err = e.BindRole(ctx, orgID, r.ID, "team", team.ID, "org", orgID, ownerID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	// First call caches principal list.
	if !e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("should have org:invite via team")
	}

	// Remove from team in DB and invalidate principals.
	if err = db.RemoveTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	e.InvalidatePrincipals(orgID, member.ID)

	// After invalidation, should no longer have the permission.
	if e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("should NOT have org:invite after team removal + cache invalidation")
	}
}

// TestUnbindRole verifies that UnbindRole removes a specific binding.
func TestUnbindRole(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "unbind")
	ctx := context.Background()

	member, err := db.InsertAccount(context.Background(), "unbind-member@example.com", "UnbindMember", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(context.Background(), orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	roles, err := db.ListRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}

	var adminRoleID int64
	for _, r := range roles {
		if r.Name == "admin" && r.IsBuiltin {
			adminRoleID = r.ID
			break
		}
	}

	if err = e.BindRole(ctx, orgID, adminRoleID, "account", member.ID, "org", orgID, ownerID); err != nil {
		t.Fatal(err)
	}
	if !e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("should have org:invite after binding")
	}

	// Get the binding ID.
	bindings := listRoleBindings(t, db, orgID, "org", orgID)
	var bindingID int64
	for _, b := range bindings {
		if b.SubjectType == "account" && b.SubjectID == member.ID {
			bindingID = b.ID
			break
		}
	}
	if bindingID == 0 {
		t.Fatal("binding not found")
	}

	if err = e.UnbindRole(ctx, bindingID, orgID); err != nil {
		t.Fatal(err)
	}
	if e.Can(ctx, member.ID, orgID, "org", "org", orgID, "org:invite") {
		t.Error("should NOT have org:invite after unbinding")
	}
}

// TestCanEnvironmentScopeCoversTaggedConnections verifies that a permission binding at environment
// scope is inherited by connections tagged to that environment.
func TestCanEnvironmentScopeCoversTaggedConnections(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "env-conn-inherit")
	ctx := context.Background()

	member, err := db.InsertAccount(ctx, "env-conn@example.com", "EnvConn", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(ctx, orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	ws, err := db.InsertWorkspace(ctx, &orgID, "org", orgID, "EnvWS", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SeedWorkspace(ctx, orgID, ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	// Create an environment in the workspace.
	env, err := db.InsertEnvironment(ctx, ws.ID, "staging", "")
	if err != nil {
		t.Fatal(err)
	}

	// Create a connection tagged to that environment.
	taggedConn, err := db.InsertConnection(ctx, ws.ID, &env.ID, "tagged-db", "sqlite", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	// Create a connection NOT tagged to any environment.
	untaggedConn, err := db.InsertConnection(ctx, ws.ID, nil, "untagged-db", "sqlite", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, nil, "env-conn-execute", "environment", []string{"conn:execute"}, "account", member.ID, "environment", env.ID, ownerID)

	// Member should have conn:execute on the tagged connection (inherits via environment).
	if !e.Can(ctx, member.ID, orgID, "org", "connection", taggedConn.ID, "conn:execute") {
		t.Error("member should have conn:execute on connection tagged to the environment")
	}

	// Member should NOT have conn:execute on the untagged connection.
	if e.Can(ctx, member.ID, orgID, "org", "connection", untaggedConn.ID, "conn:execute") {
		t.Error("member should NOT have conn:execute on connection not tagged to the environment")
	}
}

// TestCanEnvironmentScopeIsolatesAcrossEnvironments verifies that a binding on env A does not
// grant access to connections tagged to env B within the same workspace.
func TestCanEnvironmentScopeIsolatesAcrossEnvironments(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "env-isolation")
	ctx := context.Background()

	member, err := db.InsertAccount(ctx, "env-iso@example.com", "EnvIso", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(ctx, orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	ws, err := db.InsertWorkspace(ctx, &orgID, "org", orgID, "IsoWS", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SeedWorkspace(ctx, orgID, ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	envA, err := db.InsertEnvironment(ctx, ws.ID, "env-a", "")
	if err != nil {
		t.Fatal(err)
	}
	envB, err := db.InsertEnvironment(ctx, ws.ID, "env-b", "")
	if err != nil {
		t.Fatal(err)
	}

	connA, err := db.InsertConnection(ctx, ws.ID, &envA.ID, "conn-a", "sqlite", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}
	connB, err := db.InsertConnection(ctx, ws.ID, &envB.ID, "conn-b", "sqlite", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, nil, "env-a-conn-execute", "environment", []string{"conn:execute"}, "account", member.ID, "environment", envA.ID, ownerID)

	// Should have access to conn in env A.
	if !e.Can(ctx, member.ID, orgID, "org", "connection", connA.ID, "conn:execute") {
		t.Error("member should have conn:execute on connection in env A")
	}
	// Should NOT have access to conn in env B.
	if e.Can(ctx, member.ID, orgID, "org", "connection", connB.ID, "conn:execute") {
		t.Error("member should NOT have conn:execute on connection in env B")
	}
}

// TestCanWorkspaceScopeStillCoversEnvTaggedConnections verifies that a workspace-scope binding
// continues to cover connections tagged to environments (workspace scope is always in ancestry).
func TestCanWorkspaceScopeStillCoversEnvTaggedConnections(t *testing.T) {
	e, db := newTestEnforcer(t)
	orgID, ownerID := seedOrg(t, db, e, "ws-covers-env-conn")
	ctx := context.Background()

	member, err := db.InsertAccount(ctx, "ws-env-cov@example.com", "WsEnvCov", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddOrgMember(ctx, orgID, member.ID); err != nil {
		t.Fatal(err)
	}

	ws, err := db.InsertWorkspace(ctx, &orgID, "org", orgID, "CovWS", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SeedWorkspace(ctx, orgID, ws.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	env, err := db.InsertEnvironment(ctx, ws.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	conn, err := db.InsertConnection(ctx, ws.ID, &env.ID, "prod-db", "sqlite", "enc", "open")
	if err != nil {
		t.Fatal(err)
	}

	createRoleAndBind(t, e, db, orgID, &ws.ID, "ws-conn-execute-tagged", "workspace", []string{"conn:execute"}, "account", member.ID, "workspace", ws.ID, ownerID)

	// Should have access even though connection is env-tagged, because workspace is still in ancestry.
	if !e.Can(ctx, member.ID, orgID, "org", "connection", conn.ID, "conn:execute") {
		t.Error("member should have conn:execute on env-tagged connection via workspace-scope binding")
	}
}
