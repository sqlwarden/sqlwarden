package database

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/access"
)

// newEnforcer creates an access.Enforcer backed by the test DB.
func newEnforcer(t *testing.T, db *DB) *access.Enforcer {
	t.Helper()
	e, err := access.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// newAccount creates a throw-away account and returns its ID.
func newAccount(t *testing.T, db *DB, email string) int64 {
	t.Helper()
	pw := "testpw"
	acc, err := db.InsertAccount(context.Background(), email, email, &pw)
	if err != nil {
		t.Fatal(err)
	}
	return acc.ID
}

func grantScopedRole(t *testing.T, db *DB, e *access.Enforcer, orgID int64, workspaceID *int64, roleName, scopeType string, permissions []string, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) {
	t.Helper()

	roleID, err := e.CreateRole(context.Background(), orgID, workspaceID, roleName, roleName+" description", scopeType, permissions)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.BindRole(context.Background(), orgID, roleID, subjectType, subjectID, resourceType, resourceID, grantedBy); err != nil {
		t.Fatal(err)
	}
}

// findOrgRoleID looks up an org-level builtin role by name.
func findOrgRoleID(t *testing.T, db *DB, orgID int64, name string) int64 {
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

// findWsRoleID looks up a workspace-scoped role by name.
func findWsRoleID(t *testing.T, db *DB, orgID, workspaceID int64, name string) int64 {
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

// seedWorkspace inserts a workspace, seeds its roles, and returns it.
func seedWorkspace(t *testing.T, db *DB, e *access.Enforcer, orgID, creatorID int64, name string) Workspace {
	t.Helper()
	ws, err := db.InsertWorkspace(context.Background(), &orgID, "org", orgID, name, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SeedWorkspace(context.Background(), orgID, ws.ID, creatorID); err != nil {
		t.Fatal(err)
	}
	return ws
}

func wsIDs(wss []Workspace) []int64 {
	ids := make([]int64, len(wss))
	for i, w := range wss {
		ids[i] = w.ID
	}
	return ids
}

func connIDs(conns []Connection) []int64 {
	ids := make([]int64, len(conns))
	for i, c := range conns {
		ids[i] = c.ID
	}
	return ids
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// ListAccessibleWorkspaces
// ---------------------------------------------------------------------------

func TestListAccessibleWorkspaces_NoBindings(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-none", "Org")
	ownerID := newAccount(t, db, "owner-ws-none@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	seedWorkspace(t, db, e, org.ID, ownerID, "Beta")

	userID := newAccount(t, db, "user-ws-none@example.com")
	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected 0 workspaces, got %d", len(wss))
	}
}

func TestListAccessibleWorkspaces_OrgRoleGrantsAll(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-orgrole", "Org")
	ownerID := newAccount(t, db, "owner-ws-orgrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Beta")

	userID := newAccount(t, db, "user-ws-orgrole@example.com")
	adminRoleID := findOrgRoleID(t, db, org.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), org.ID, adminRoleID, "account", userID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(wss))
	}
	ids := wsIDs(wss)
	if !contains(ids, ws1.ID) || !contains(ids, ws2.ID) {
		t.Fatalf("missing expected workspace IDs in %v", ids)
	}
}

func TestListAccessibleWorkspaces_WsRoleBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-dirws", "Org")
	ownerID := newAccount(t, db, "owner-ws-dirws@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Bound")
	_ = seedWorkspace(t, db, e, org.ID, ownerID, "Unbound")

	userID := newAccount(t, db, "user-ws-dirws@example.com")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, access.BuiltinWorkspaceMemberRole)
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", userID, "workspace", ws1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_DirectWsPermBinding(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-dirperm", "Org")
	ownerID := newAccount(t, db, "owner-ws-dirperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Bound")
	_ = seedWorkspace(t, db, e, org.ID, ownerID, "Unbound")

	userID := newAccount(t, db, "user-ws-dirperm@example.com")
	grantScopedRole(t, db, e, org.ID, &ws1.ID, "ws-read-bound", "workspace", []string{access.PermWsRead}, "account", userID, "workspace", ws1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1, got %v", wsIDs(wss))
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected ws1 to be directly accessible")
	}
}

func TestListAccessibleWorkspaces_OrgPermBinding(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-orgperm", "Org")
	ownerID := newAccount(t, db, "owner-ws-orgperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Beta")

	userID := newAccount(t, db, "user-ws-orgperm@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "org-ws-read-all", "org", []string{access.PermWsRead}, "account", userID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(wss))
	}
	ids := wsIDs(wss)
	if !contains(ids, ws1.ID) || !contains(ids, ws2.ID) {
		t.Fatalf("missing workspace IDs in %v", ids)
	}
}

func TestListAccessibleWorkspaces_OrgReadDoesNotGrantWorkspaceDiscovery(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-orgread-only", "Org")
	ownerID := newAccount(t, db, "owner-ws-orgread-only@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")

	userID := newAccount(t, db, "user-ws-orgread-only@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "org-read-only", "org", []string{access.PermOrgRead}, "account", userID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected no workspace discovery from org:read, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_OrgMembersBinding(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-org-members", "Org")
	ownerID := newAccount(t, db, "owner-ws-org-members@example.com")
	if err := db.AddOrgMember(context.Background(), org.ID, ownerID); err != nil {
		t.Fatal(err)
	}
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")

	userID := newAccount(t, db, "user-ws-org-members@example.com")
	if err := db.AddOrgMember(context.Background(), org.ID, userID); err != nil {
		t.Fatal(err)
	}
	grantScopedRole(t, db, e, org.ID, nil, "all-members-ws-read", "org", []string{access.PermWsRead}, access.SubjectTypeOrgMembers, org.ID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws.ID {
		t.Fatalf("expected org member to discover workspace %d, got %v", ws.ID, wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_TeamOrgBinding(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-team-org", "Org")
	ownerID := newAccount(t, db, "owner-ws-team-org@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Beta")

	team, _ := db.InsertTeam(context.Background(), org.ID, "devs-ws-org", "Devs")
	userID := newAccount(t, db, "user-ws-team-org@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	adminRoleID := findOrgRoleID(t, db, org.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), org.ID, adminRoleID, "team", team.ID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 2 {
		t.Fatalf("expected 2 workspaces via team, got %d", len(wss))
	}
	ids := wsIDs(wss)
	if !contains(ids, ws1.ID) || !contains(ids, ws2.ID) {
		t.Fatalf("missing workspace IDs in %v", ids)
	}
}

func TestListAccessibleWorkspaces_TeamWsBinding(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-team-ws", "Org")
	ownerID := newAccount(t, db, "owner-ws-team-ws@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Bound")
	_ = seedWorkspace(t, db, e, org.ID, ownerID, "Unbound")

	team, _ := db.InsertTeam(context.Background(), org.ID, "devs-ws-ws", "Devs")
	userID := newAccount(t, db, "user-ws-team-ws@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, access.BuiltinWorkspaceMemberRole)
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "team", team.ID, "workspace", ws1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_TeamNotMember(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-nonmember", "Org")
	ownerID := newAccount(t, db, "owner-ws-nonmember@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	team, _ := db.InsertTeam(context.Background(), org.ID, "devs-nonmember", "Devs")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, access.BuiltinWorkspaceMemberRole)
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "team", team.ID, "workspace", ws1.ID, ownerID)

	// userID is NOT added to the team
	userID := newAccount(t, db, "user-ws-nonmember@example.com")
	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected 0 workspaces for non-team-member, got %d", len(wss))
	}
}

func TestListAccessibleWorkspaces_WorkspaceMembersDirectPrincipal(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-members-direct", "Org")
	ownerID := newAccount(t, db, "owner-ws-members-direct@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Bound")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Unbound")
	userID := newAccount(t, db, "user-ws-members-direct@example.com")
	if err := db.AddOrgMember(context.Background(), org.ID, userID); err != nil {
		t.Fatal(err)
	}
	if err := db.AddWorkspaceMember(context.Background(), ws1.ID, userID, &ownerID); err != nil {
		t.Fatal(err)
	}

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only direct workspace member workspace, got %v; unbound=%d", wsIDs(wss), ws2.ID)
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected direct workspace member workspace to be accessible")
	}
}

func TestListAccessibleWorkspaces_WorkspaceMembersTeamPrincipal(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-members-team", "Org")
	ownerID := newAccount(t, db, "owner-ws-members-team@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Team Bound")
	userID := newAccount(t, db, "user-ws-members-team@example.com")
	if err := db.AddOrgMember(context.Background(), org.ID, userID); err != nil {
		t.Fatal(err)
	}
	team, err := db.InsertTeam(context.Background(), org.ID, "acc-ws-members-team", "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddTeamMember(context.Background(), team.ID, userID); err != nil {
		t.Fatal(err)
	}
	if err = db.AddWorkspaceTeam(context.Background(), ws.ID, team.ID, &ownerID); err != nil {
		t.Fatal(err)
	}

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws.ID {
		t.Fatalf("expected team-derived workspace member workspace, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_WorkspaceMembersRequiresOrgMembership(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-members-invalid", "Org")
	ownerID := newAccount(t, db, "owner-ws-members-invalid@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Invalid Direct")
	userID := newAccount(t, db, "user-ws-members-invalid@example.com")
	if err := db.AddWorkspaceMember(context.Background(), ws.ID, userID, &ownerID); err != nil {
		t.Fatal(err)
	}

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected invalid workspace membership to be ignored, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_WorkspaceMembersRevocation(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-members-revoke", "Org")
	ownerID := newAccount(t, db, "owner-ws-members-revoke@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Revoked")
	userID := newAccount(t, db, "user-ws-members-revoke@example.com")
	if err := db.AddOrgMember(context.Background(), org.ID, userID); err != nil {
		t.Fatal(err)
	}
	if err := db.AddWorkspaceMember(context.Background(), ws.ID, userID, &ownerID); err != nil {
		t.Fatal(err)
	}

	var binding struct {
		ID int64
	}
	if err := db.NewSelect().
		TableExpr("role_bindings AS rb").
		ColumnExpr("rb.id").
		Join("JOIN roles AS r ON r.id = rb.role_id").
		Where("rb.org_id = ? AND rb.subject_type = ? AND rb.subject_id = ? AND rb.resource_type = 'workspace' AND rb.resource_id = ? AND r.name = ?",
			org.ID, access.SubjectTypeWorkspaceMembers, ws.ID, ws.ID, access.BuiltinWorkspaceMemberRole).
		Scan(context.Background(), &binding); err != nil {
		t.Fatal(err)
	}
	if err := e.UnbindRole(context.Background(), binding.ID, org.ID); err != nil {
		t.Fatal(err)
	}

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected workspace_members policy revocation to remove visibility, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_CrossOrgIsolation(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	orgA, _ := db.InsertOrg(context.Background(), "acc-ws-orga", "Org A")
	orgB, _ := db.InsertOrg(context.Background(), "acc-ws-orgb", "Org B")
	ownerA := newAccount(t, db, "owner-ws-orga@example.com")
	ownerB := newAccount(t, db, "owner-ws-orgb@example.com")
	_ = e.SeedOrg(context.Background(), orgA.ID, ownerA)
	_ = e.SeedOrg(context.Background(), orgB.ID, ownerB)

	_ = seedWorkspace(t, db, e, orgA.ID, ownerA, "WS-A")
	wsB := seedWorkspace(t, db, e, orgB.ID, ownerB, "WS-B")

	userID := newAccount(t, db, "user-ws-xorg@example.com")
	adminRoleB := findOrgRoleID(t, db, orgB.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), orgB.ID, adminRoleB, "account", userID, "org", orgB.ID, ownerB)

	wssA, err := db.ListAccessibleWorkspaces(context.Background(), userID, orgA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wssA) != 0 {
		t.Fatalf("cross-org leak: expected 0 in org A, got %d", len(wssA))
	}

	wssB, err := db.ListAccessibleWorkspaces(context.Background(), userID, orgB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wssB) != 1 || wssB[0].ID != wsB.ID {
		t.Fatalf("expected wsB in org B, got %v", wsIDs(wssB))
	}
}

func TestListAccessibleWorkspaces_TwoUsersIsolation(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-2users", "Org")
	ownerID := newAccount(t, db, "owner-ws-2users@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "AliceWS")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "BobWS")

	aliceID := newAccount(t, db, "alice-ws-2users@example.com")
	bobID := newAccount(t, db, "bob-ws-2users@example.com")

	_ = e.BindRole(context.Background(), org.ID, findWsRoleID(t, db, org.ID, ws1.ID, access.BuiltinWorkspaceMemberRole), "account", aliceID, "workspace", ws1.ID, ownerID)
	_ = e.BindRole(context.Background(), org.ID, findWsRoleID(t, db, org.ID, ws2.ID, access.BuiltinWorkspaceMemberRole), "account", bobID, "workspace", ws2.ID, ownerID)

	aliceWss, _ := db.ListAccessibleWorkspaces(context.Background(), aliceID, org.ID)
	if len(aliceWss) != 1 || aliceWss[0].ID != ws1.ID {
		t.Fatalf("alice should only see ws1, got %v", wsIDs(aliceWss))
	}

	bobWss, _ := db.ListAccessibleWorkspaces(context.Background(), bobID, org.ID)
	if len(bobWss) != 1 || bobWss[0].ID != ws2.ID {
		t.Fatalf("bob should only see ws2, got %v", wsIDs(bobWss))
	}
}

func TestListAccessibleWorkspaces_OwnerSeesAll(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-owner", "Org")
	ownerID := newAccount(t, db, "owner-ws-owner@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Beta")
	ws3 := seedWorkspace(t, db, e, org.ID, ownerID, "Gamma")

	wss, err := db.ListAccessibleWorkspaces(context.Background(), ownerID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 3 {
		t.Fatalf("expected 3 workspaces for owner, got %d", len(wss))
	}
	ids := wsIDs(wss)
	for _, want := range []int64{ws1.ID, ws2.ID, ws3.ID} {
		if !contains(ids, want) {
			t.Fatalf("owner missing workspace %d in %v", want, ids)
		}
	}
}

func TestListAccessibleWorkspaces_EmptyOrg(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-empty", "Org")
	ownerID := newAccount(t, db, "owner-ws-empty@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	userID := newAccount(t, db, "user-ws-empty@example.com")
	adminRoleID := findOrgRoleID(t, db, org.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), org.ID, adminRoleID, "account", userID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected 0 workspaces in empty org, got %d", len(wss))
	}
}

// ---------------------------------------------------------------------------
// ListAccessibleConnections
// ---------------------------------------------------------------------------

func TestListAccessibleConnections_NoBindings(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-none", "Org")
	ownerID := newAccount(t, db, "owner-conn-none@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-none@example.com")
	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(conns))
	}
}

func TestListAccessibleConnections_OrgRoleGrantsAll(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-orgrole", "Org")
	ownerID := newAccount(t, db, "owner-conn-orgrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-orgrole@example.com")
	adminRoleID := findOrgRoleID(t, db, org.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), org.ID, adminRoleID, "account", userID, "org", org.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}
	ids := connIDs(conns)
	if !contains(ids, c1.ID) || !contains(ids, c2.ID) {
		t.Fatalf("missing connection IDs in %v", ids)
	}
}

func TestOrgConnectionRoleGrantsDiscoveryAcrossAllAncestors(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-org-conn-discovery", "Org")
	ownerID := newAccount(t, db, "owner-org-conn-discovery@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Beta")
	envA1, _ := db.InsertEnvironment(context.Background(), ws1.ID, "env-a1", "")
	envA2, _ := db.InsertEnvironment(context.Background(), ws1.ID, "env-a2", "")
	envB1, _ := db.InsertEnvironment(context.Background(), ws2.ID, "env-b1", "")
	connA1, _ := db.InsertConnection(context.Background(), ws1.ID, &envA1.ID, "conn-a1", "postgres", "enc", "open")
	connA2, _ := db.InsertConnection(context.Background(), ws1.ID, &envA2.ID, "conn-a2", "postgres", "enc", "open")
	connB1, _ := db.InsertConnection(context.Background(), ws2.ID, &envB1.ID, "conn-b1", "postgres", "enc", "open")
	connB2, _ := db.InsertConnection(context.Background(), ws2.ID, nil, "conn-b2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-org-conn-discovery@example.com")
	roleID, err := e.CreateRole(context.Background(), org.ID, nil, "org-conn-reader", "", "org", []string{access.PermConnRead})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.BindRole(context.Background(), org.ID, roleID, "account", userID, "org", org.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceIDs := wsIDs(wss)
	if len(workspaceIDs) != 2 || !contains(workspaceIDs, ws1.ID) || !contains(workspaceIDs, ws2.ID) {
		t.Fatalf("expected both workspaces visible via org conn role, got %v", workspaceIDs)
	}

	envsWs1, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	envIDsWs1 := envIDs(envsWs1)
	if len(envIDsWs1) != 3 || !contains(envIDsWs1, envA1.ID) || !contains(envIDsWs1, envA2.ID) {
		t.Fatalf("expected ws1 environments including Default visible via org conn role, got %v", envIDsWs1)
	}

	envsWs2, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws2.ID)
	if err != nil {
		t.Fatal(err)
	}
	envIDsWs2 := envIDs(envsWs2)
	if len(envIDsWs2) != 2 || !contains(envIDsWs2, envB1.ID) {
		t.Fatalf("expected ws2 environments including Default, got %v", envIDsWs2)
	}

	connsWs1, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	connIDsWs1 := connIDs(connsWs1)
	if len(connIDsWs1) != 2 || !contains(connIDsWs1, connA1.ID) || !contains(connIDsWs1, connA2.ID) {
		t.Fatalf("expected both ws1 connections visible via org conn role, got %v", connIDsWs1)
	}

	connsWs2, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws2.ID)
	if err != nil {
		t.Fatal(err)
	}
	connIDsWs2 := connIDs(connsWs2)
	if len(connIDsWs2) != 2 || !contains(connIDsWs2, connB1.ID) || !contains(connIDsWs2, connB2.ID) {
		t.Fatalf("expected both ws2 connections visible via org conn role, got %v", connIDsWs2)
	}
}

func TestListAccessibleConnections_WorkspaceRoleWithConnReadGrantsAllInWorkspace(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-wsrole", "Org")
	ownerID := newAccount(t, db, "owner-conn-wsrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsrole@example.com")
	grantScopedRole(t, db, e, org.ID, &ws.ID, "workspace-conn-read", "workspace", []string{access.PermConnRead}, "account", userID, "workspace", ws.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections via ws role, got %d", len(conns))
	}
	ids := connIDs(conns)
	if !contains(ids, c1.ID) || !contains(ids, c2.ID) {
		t.Fatalf("missing connection IDs in %v", ids)
	}
}

func TestListAccessibleConnections_WorkspaceMemberRoleDoesNotRevealConnections(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-wsmember-no-conns", "Org")
	ownerID := newAccount(t, db, "owner-conn-wsmember-no-conns@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsmember-no-conns@example.com")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, access.BuiltinWorkspaceMemberRole)
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", userID, "workspace", ws.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected no connection discovery from Workspace Member role, got %v", connIDs(conns))
	}
}

func TestListAccessibleConnections_DirectConnRoleBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-dirconn", "Org")
	ownerID := newAccount(t, db, "owner-conn-dirconn@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-dirconn@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "direct-conn-read", "connection", []string{access.PermConnRead}, "account", userID, "connection", c1.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].ID != c1.ID {
		t.Fatalf("expected only c1, got %v", connIDs(conns))
	}
}

func TestListAccessibleConnections_DirectConnPermBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-dirperm", "Org")
	ownerID := newAccount(t, db, "owner-conn-dirperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-dirperm@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "conn-read-c1", "connection", []string{access.PermConnRead}, "account", userID, "connection", c1.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].ID != c1.ID {
		t.Fatalf("expected only c1, got %v", connIDs(conns))
	}

	ok, err := db.HasAccessibleConnection(context.Background(), userID, org.ID, ws.ID, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected c1 to be directly accessible")
	}
}

func TestListAccessibleConnections_WsReadDoesNotRevealConnections(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-wsperm", "Org")
	ownerID := newAccount(t, db, "owner-conn-wsperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsperm@example.com")
	grantScopedRole(t, db, e, org.ID, &ws.ID, "ws-read-access", "workspace", []string{access.PermWsRead}, "account", userID, "workspace", ws.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected no connection discovery via ws:read, got %v", connIDs(conns))
	}
}

func TestListAccessibleConnections_TeamOrgBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-team-org", "Org")
	ownerID := newAccount(t, db, "owner-conn-team-org@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	team, _ := db.InsertTeam(context.Background(), org.ID, "eng-conn-org", "Eng")
	userID := newAccount(t, db, "user-conn-team-org@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	adminRoleID := findOrgRoleID(t, db, org.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), org.ID, adminRoleID, "team", team.ID, "org", org.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections via team org binding, got %d", len(conns))
	}
	ids := connIDs(conns)
	if !contains(ids, c1.ID) || !contains(ids, c2.ID) {
		t.Fatalf("missing connection IDs in %v", ids)
	}
}

func TestListAccessibleConnections_TeamConnBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-team-conn", "Org")
	ownerID := newAccount(t, db, "owner-conn-team-conn@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")

	team, _ := db.InsertTeam(context.Background(), org.ID, "eng-conn-conn", "Eng")
	userID := newAccount(t, db, "user-conn-team-conn@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	grantScopedRole(t, db, e, org.ID, nil, "team-conn-read", "connection", []string{access.PermConnRead}, "team", team.ID, "connection", c1.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].ID != c1.ID {
		t.Fatalf("expected only c1, got %v", connIDs(conns))
	}
}

func TestListAccessibleConnections_TeamNotMember(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-nonmember", "Org")
	ownerID := newAccount(t, db, "owner-conn-nonmember@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	_ = c1

	team, _ := db.InsertTeam(context.Background(), org.ID, "eng-nonmember", "Eng")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, access.BuiltinWorkspaceMemberRole)
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "team", team.ID, "workspace", ws.ID, ownerID)

	// userID is NOT in the team
	userID := newAccount(t, db, "user-conn-nonmember@example.com")
	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 0 {
		t.Fatalf("non-team-member should see 0 connections, got %d", len(conns))
	}
}

func TestListAccessibleConnections_CrossOrgIsolation(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	orgA, _ := db.InsertOrg(context.Background(), "acc-conn-orga", "Org A")
	orgB, _ := db.InsertOrg(context.Background(), "acc-conn-orgb", "Org B")
	ownerA := newAccount(t, db, "owner-conn-orga@example.com")
	ownerB := newAccount(t, db, "owner-conn-orgb@example.com")
	_ = e.SeedOrg(context.Background(), orgA.ID, ownerA)
	_ = e.SeedOrg(context.Background(), orgB.ID, ownerB)

	wsA := seedWorkspace(t, db, e, orgA.ID, ownerA, "Main")
	wsB := seedWorkspace(t, db, e, orgB.ID, ownerB, "Main")
	db.InsertConnection(context.Background(), wsA.ID, nil, "db-a", "postgres", "enc", "open")
	cB, _ := db.InsertConnection(context.Background(), wsB.ID, nil, "db-b", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-xorg@example.com")
	adminRoleB := findOrgRoleID(t, db, orgB.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), orgB.ID, adminRoleB, "account", userID, "org", orgB.ID, ownerB)

	connsA, err := db.ListAccessibleConnections(context.Background(), userID, orgA.ID, wsA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connsA) != 0 {
		t.Fatalf("cross-org leak: expected 0 connections in org A, got %d", len(connsA))
	}

	connsB, err := db.ListAccessibleConnections(context.Background(), userID, orgB.ID, wsB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connsB) != 1 || connsB[0].ID != cB.ID {
		t.Fatalf("expected cB in org B, got %v", connIDs(connsB))
	}
}

func TestListAccessibleConnections_TwoUsersIsolation(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-2users", "Org")
	ownerID := newAccount(t, db, "owner-conn-2users@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "alice-db", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, "bob-db", "postgres", "enc", "open")

	aliceID := newAccount(t, db, "alice-conn-2users@example.com")
	bobID := newAccount(t, db, "bob-conn-2users@example.com")

	grantScopedRole(t, db, e, org.ID, nil, "alice-conn-read", "connection", []string{access.PermConnRead}, "account", aliceID, "connection", c1.ID, ownerID)
	grantScopedRole(t, db, e, org.ID, nil, "bob-conn-read", "connection", []string{access.PermConnRead}, "account", bobID, "connection", c2.ID, ownerID)

	aliceConns, _ := db.ListAccessibleConnections(context.Background(), aliceID, org.ID, ws.ID)
	if len(aliceConns) != 1 || aliceConns[0].ID != c1.ID {
		t.Fatalf("alice should only see c1, got %v", connIDs(aliceConns))
	}

	bobConns, _ := db.ListAccessibleConnections(context.Background(), bobID, org.ID, ws.ID)
	if len(bobConns) != 1 || bobConns[0].ID != c2.ID {
		t.Fatalf("bob should only see c2, got %v", connIDs(bobConns))
	}
}

func TestListAccessibleConnections_WsBindingDoesNotLeakToOtherWs(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-wsscope", "Org")
	ownerID := newAccount(t, db, "owner-conn-wsscope@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "WS1")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "WS2")
	c1, _ := db.InsertConnection(context.Background(), ws1.ID, nil, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws2.ID, nil, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsscope@example.com")
	grantScopedRole(t, db, e, org.ID, &ws1.ID, "workspace-conn-read-ws1", "workspace", []string{access.PermConnRead}, "account", userID, "workspace", ws1.ID, ownerID)

	connsWs2, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connsWs2) != 0 {
		t.Fatalf("ws1 binding should not leak to ws2, got %d connections", len(connsWs2))
	}

	connsWs1, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(connsWs1) != 1 || connsWs1[0].ID != c1.ID {
		t.Fatalf("expected c1 in ws1, got %v", connIDs(connsWs1))
	}
}

func TestListAccessibleConnections_OwnerSeesAll(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-owner", "Org")
	ownerID := newAccount(t, db, "owner-conn-owner@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db2", "postgres", "enc", "open")
	c3, _ := db.InsertConnection(context.Background(), ws.ID, nil, "db3", "postgres", "enc", "open")

	conns, err := db.ListAccessibleConnections(context.Background(), ownerID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 3 {
		t.Fatalf("expected 3 connections for owner, got %d", len(conns))
	}
	ids := connIDs(conns)
	for _, want := range []int64{c1.ID, c2.ID, c3.ID} {
		if !contains(ids, want) {
			t.Fatalf("owner missing connection %d in %v", want, ids)
		}
	}
}

// ---------------------------------------------------------------------------
// ListAccessibleConnections — environment-scope bindings
// ---------------------------------------------------------------------------

func envIDs(envs []Environment) []int64 {
	ids := make([]int64, len(envs))
	for i, e := range envs {
		ids[i] = e.ID
	}
	return ids
}

func TestListAccessibleConnections_EnvPermBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-envperm", "Org")
	ownerID := newAccount(t, db, "owner-conn-envperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	env, _ := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")

	// Tagged connection (in env) and untagged connection.
	tagged, _ := db.InsertConnection(context.Background(), ws.ID, &env.ID, "tagged", "postgres", "enc", "open")
	untagged, _ := db.InsertConnection(context.Background(), ws.ID, nil, "untagged", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-envperm@example.com")
	// Grant permission at environment scope only.
	grantScopedRole(t, db, e, org.ID, nil, "env-conn-execute", "environment", []string{"conn:execute"}, "account", userID, "environment", env.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := connIDs(conns)
	if !contains(ids, tagged.ID) {
		t.Errorf("expected tagged connection to be accessible via env-scope binding")
	}
	if contains(ids, untagged.ID) {
		t.Errorf("untagged connection should NOT be accessible via env-scope binding")
	}
}

func TestListAccessibleConnections_EnvReadDoesNotRevealConnections(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-envread", "Org")
	ownerID := newAccount(t, db, "owner-conn-envread@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	env, _ := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	tagged, _ := db.InsertConnection(context.Background(), ws.ID, &env.ID, "tagged", "postgres", "enc", "open")
	_ = tagged

	userID := newAccount(t, db, "user-conn-envread@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "env-read-only", "environment", []string{access.PermEnvRead}, "account", userID, "environment", env.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected no connection discovery from env:read, got %v", connIDs(conns))
	}
}

func TestListAccessibleConnections_EnvRoleBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-envrole", "Org")
	ownerID := newAccount(t, db, "owner-conn-envrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	envA, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-a", "")
	envB, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-b", "")

	connA, _ := db.InsertConnection(context.Background(), ws.ID, &envA.ID, "conn-a", "postgres", "enc", "open")
	connB, _ := db.InsertConnection(context.Background(), ws.ID, &envB.ID, "conn-b", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-envrole@example.com")
	// Custom env-scope role with conn:execute.
	roleID, _ := e.CreateRole(context.Background(), org.ID, nil, "env-viewer", "", "environment", []string{"conn:execute", "conn:read"})
	_ = e.BindRole(context.Background(), org.ID, roleID, "account", userID, "environment", envA.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := connIDs(conns)
	if !contains(ids, connA.ID) {
		t.Errorf("connA should be accessible via env A role binding")
	}
	if contains(ids, connB.ID) {
		t.Errorf("connB should NOT be accessible — binding is only on env A")
	}
}

// ---------------------------------------------------------------------------
// ListAccessibleEnvironments
// ---------------------------------------------------------------------------

func TestListAccessibleEnvironments_NoBindings(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-none", "Org")
	ownerID := newAccount(t, db, "owner-env-none@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	db.InsertEnvironment(context.Background(), ws.ID, "prod", "")

	userID := newAccount(t, db, "user-env-none@example.com")
	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected 0 environments, got %d", len(envs))
	}
}

func TestListAccessibleEnvironments_OrgRoleGrantsAll(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-orgrole", "Org")
	ownerID := newAccount(t, db, "owner-env-orgrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	env1, _ := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	env2, _ := db.InsertEnvironment(context.Background(), ws.ID, "prod", "")

	userID := newAccount(t, db, "user-env-orgrole@example.com")
	adminRoleID := findOrgRoleID(t, db, org.ID, access.BuiltinOrgAdminRole)
	_ = e.BindRole(context.Background(), org.ID, adminRoleID, "account", userID, "org", org.ID, ownerID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 3 {
		t.Fatalf("expected 3 environments including Default, got %d", len(envs))
	}
	ids := envIDs(envs)
	if !contains(ids, env1.ID) || !contains(ids, env2.ID) {
		t.Fatalf("missing expected environment IDs in %v", ids)
	}
}

func TestListAccessibleEnvironments_WorkspaceRoleWithEnvReadGrantsAll(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-wsrole", "Org")
	ownerID := newAccount(t, db, "owner-env-wsrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	env1, _ := db.InsertEnvironment(context.Background(), ws.ID, "alpha", "")
	env2, _ := db.InsertEnvironment(context.Background(), ws.ID, "beta", "")

	userID := newAccount(t, db, "user-env-wsrole@example.com")
	grantScopedRole(t, db, e, org.ID, &ws.ID, "workspace-env-read", "workspace", []string{access.PermEnvRead}, "account", userID, "workspace", ws.ID, ownerID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := envIDs(envs)
	if !contains(ids, env1.ID) || !contains(ids, env2.ID) {
		t.Fatalf("workspace binding should expose all environments; got %v", ids)
	}
}

func TestListAccessibleEnvironments_WorkspaceMemberRoleDoesNotRevealEnvironments(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-wsmember-no-envs", "Org")
	ownerID := newAccount(t, db, "owner-env-wsmember-no-envs@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	db.InsertEnvironment(context.Background(), ws.ID, "alpha", "")

	userID := newAccount(t, db, "user-env-wsmember-no-envs@example.com")
	wsMemberID := findWsRoleID(t, db, org.ID, ws.ID, access.BuiltinWorkspaceMemberRole)
	_ = e.BindRole(context.Background(), org.ID, wsMemberID, "account", userID, "workspace", ws.ID, ownerID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected no environment discovery from Workspace Member role, got %v", envIDs(envs))
	}
}

func TestListAccessibleEnvironments_WsReadDoesNotRevealEnvironments(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-wsread", "Org")
	ownerID := newAccount(t, db, "owner-env-wsread@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	db.InsertEnvironment(context.Background(), ws.ID, "alpha", "")

	userID := newAccount(t, db, "user-env-wsread@example.com")
	grantScopedRole(t, db, e, org.ID, &ws.ID, "ws-read-only", "workspace", []string{access.PermWsRead}, "account", userID, "workspace", ws.ID, ownerID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected no environment discovery from ws:read, got %v", envIDs(envs))
	}
}

func TestListAccessibleEnvironments_DirectEnvBinding(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-direct", "Org")
	ownerID := newAccount(t, db, "owner-env-direct@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	envA, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-a", "")
	envB, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-b", "")

	userID := newAccount(t, db, "user-env-direct@example.com")
	// Grant access only to env A.
	grantScopedRole(t, db, e, org.ID, nil, "env-read-a", "environment", []string{"env:read"}, "account", userID, "environment", envA.ID, ownerID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := envIDs(envs)
	if !contains(ids, envA.ID) {
		t.Errorf("env A should be accessible via direct env binding")
	}
	if contains(ids, envB.ID) {
		t.Errorf("env B should NOT be accessible — no binding for it")
	}

	ok, err := db.HasAccessibleEnvironment(context.Background(), userID, org.ID, ws.ID, envA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected envA to be directly accessible")
	}
}

func TestListAccessibleEnvironments_IsolatedAcrossOrgs(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org1, _ := db.InsertOrg(context.Background(), "acc-env-iso1", "Org1")
	ownerID1 := newAccount(t, db, "owner-env-iso1@example.com")
	_ = e.SeedOrg(context.Background(), org1.ID, ownerID1)
	ws1 := seedWorkspace(t, db, e, org1.ID, ownerID1, "WS1")
	env1, _ := db.InsertEnvironment(context.Background(), ws1.ID, "prod", "")

	org2, _ := db.InsertOrg(context.Background(), "acc-env-iso2", "Org2")
	ownerID2 := newAccount(t, db, "owner-env-iso2@example.com")
	_ = e.SeedOrg(context.Background(), org2.ID, ownerID2)
	ws2 := seedWorkspace(t, db, e, org2.ID, ownerID2, "WS2")
	env2, _ := db.InsertEnvironment(context.Background(), ws2.ID, "prod", "")

	// ownerID1 has org1 bindings; should not see env2.
	envs1, _ := db.ListAccessibleEnvironments(context.Background(), ownerID1, org1.ID, ws1.ID)
	ids1 := envIDs(envs1)
	if !contains(ids1, env1.ID) {
		t.Errorf("owner1 should see env1")
	}

	envs2, _ := db.ListAccessibleEnvironments(context.Background(), ownerID1, org2.ID, ws2.ID)
	ids2 := envIDs(envs2)
	if contains(ids2, env2.ID) {
		t.Errorf("owner1 should NOT see env2 in org2")
	}
}

func TestListAccessibleWorkspaces_EnvPermissionPropagatesVisibility(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-env-prop", "Org")
	ownerID := newAccount(t, db, "owner-ws-env-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "EnvVisible")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Hidden")
	env1, _ := db.InsertEnvironment(context.Background(), ws1.ID, "env-a", "")
	_, _ = db.InsertEnvironment(context.Background(), ws2.ID, "env-b", "")

	userID := newAccount(t, db, "user-ws-env-prop@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "env-read-1", "environment", []string{access.PermEnvRead}, "account", userID, "environment", env1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1 from env binding, got %v", wsIDs(wss))
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected workspace visibility to propagate from environment access")
	}
}

func TestListAccessibleWorkspaces_ConnPermissionPropagatesVisibility(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-conn-prop", "Org")
	ownerID := newAccount(t, db, "owner-ws-conn-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "ConnVisible")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Hidden")
	env1, _ := db.InsertEnvironment(context.Background(), ws1.ID, "env-a", "")
	conn1, _ := db.InsertConnection(context.Background(), ws1.ID, &env1.ID, "conn-a", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws2.ID, nil, "conn-b", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-ws-conn-prop@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "conn-read-1", "connection", []string{access.PermConnRead}, "account", userID, "connection", conn1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1 from conn binding, got %v", wsIDs(wss))
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected workspace visibility to propagate from connection access")
	}
}

func TestListAccessibleWorkspaces_OrgPolicyPermissionPropagatesVisibility(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-org-policy-prop", "Org")
	ownerID := newAccount(t, db, "owner-ws-org-policy-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "PolicyVisibleA")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "PolicyVisibleB")

	userID := newAccount(t, db, "user-ws-org-policy-prop@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "org-policy-read", "org", []string{access.PermPolicyRead}, "account", userID, "org", org.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := wsIDs(wss)
	if len(ids) != 2 || !contains(ids, ws1.ID) || !contains(ids, ws2.ID) {
		t.Fatalf("expected both workspaces from org policy binding, got %v", ids)
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected workspace visibility to propagate from org policy access")
	}
}

func TestListAccessibleWorkspaces_WorkspacePolicyPermissionPropagatesVisibility(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-policy-prop", "Org")
	ownerID := newAccount(t, db, "owner-ws-policy-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "PolicyVisible")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Hidden")

	userID := newAccount(t, db, "user-ws-policy-prop@example.com")
	grantScopedRole(t, db, e, org.ID, &ws1.ID, "workspace-policy-read", "workspace", []string{access.PermPolicyRead}, "account", userID, "workspace", ws1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1 from workspace policy binding, got %v", wsIDs(wss))
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected workspace visibility to propagate from workspace policy access")
	}

	ok, err = db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("workspace policy access should not expose sibling workspaces")
	}
}

func TestListAccessibleWorkspaces_ConnRolePropagatesVisibilityForTeam(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-team-conn-prop", "Org")
	ownerID := newAccount(t, db, "owner-ws-team-conn-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Visible")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Hidden")
	env1, _ := db.InsertEnvironment(context.Background(), ws1.ID, "env-a", "")
	conn1, _ := db.InsertConnection(context.Background(), ws1.ID, &env1.ID, "conn-a", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws2.ID, nil, "conn-b", "postgres", "enc", "open")

	team, _ := db.InsertTeam(context.Background(), org.ID, "ops-team-conn-prop", "Ops")
	userID := newAccount(t, db, "user-ws-team-conn-prop@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	roleID, err := e.CreateRole(context.Background(), org.ID, &ws1.ID, "conn-reader-prop", "", "connection", []string{access.PermConnRead})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.BindRole(context.Background(), org.ID, roleID, "team", team.ID, "connection", conn1.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1 from team conn role, got %v", wsIDs(wss))
	}

	ok, err := db.HasAccessibleWorkspace(context.Background(), userID, org.ID, ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected team connection role to propagate workspace visibility")
	}
}

func TestListAccessibleWorkspaces_IgnoresInvalidConnectionScopedEnvPermission(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-invalid-conn-env", "Org")
	ownerID := newAccount(t, db, "owner-ws-invalid-conn-env@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	env, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-a", "")
	conn, _ := db.InsertConnection(context.Background(), ws.ID, &env.ID, "conn-a", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-ws-invalid-conn-env@example.com")
	binding := insertTestScopedRoleBinding(t, db, org.ID, &ws.ID, "invalid-conn-env", "connection", []string{access.PermEnvRead}, "account", userID, "connection", conn.ID)
	_ = binding

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Fatalf("expected invalid connection-scoped env permission to be ignored for workspace discovery, got %v", wsIDs(wss))
	}
}

func TestListAccessibleEnvironments_ConnPermissionPropagatesVisibility(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-conn-prop", "Org")
	ownerID := newAccount(t, db, "owner-env-conn-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	envA, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-a", "")
	envB, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-b", "")
	connA, _ := db.InsertConnection(context.Background(), ws.ID, &envA.ID, "conn-a", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, &envB.ID, "conn-b", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-env-conn-prop@example.com")
	grantScopedRole(t, db, e, org.ID, nil, "conn-read-a", "connection", []string{access.PermConnRead}, "account", userID, "connection", connA.ID, ownerID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].ID != envA.ID {
		t.Fatalf("expected only envA from conn binding, got %v", envIDs(envs))
	}

	ok, err := db.HasAccessibleEnvironment(context.Background(), userID, org.ID, ws.ID, envA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected environment visibility to propagate from connection access")
	}
}

func TestListAccessibleEnvironments_ConnRolePropagatesVisibilityForTeam(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-team-conn-prop", "Org")
	ownerID := newAccount(t, db, "owner-env-team-conn-prop@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	envA, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-a", "")
	envB, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-b", "")
	connA, _ := db.InsertConnection(context.Background(), ws.ID, &envA.ID, "conn-a", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, &envB.ID, "conn-b", "postgres", "enc", "open")

	team, _ := db.InsertTeam(context.Background(), org.ID, "ops-env-team-prop", "Ops")
	userID := newAccount(t, db, "user-env-team-conn-prop@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	roleID, err := e.CreateRole(context.Background(), org.ID, &ws.ID, "conn-reader-env-prop", "", "connection", []string{access.PermConnRead})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.BindRole(context.Background(), org.ID, roleID, "team", team.ID, "connection", connA.ID, ownerID); err != nil {
		t.Fatal(err)
	}

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].ID != envA.ID {
		t.Fatalf("expected only envA from team conn role, got %v", envIDs(envs))
	}

	ok, err := db.HasAccessibleEnvironment(context.Background(), userID, org.ID, ws.ID, envA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected team connection role to propagate environment visibility")
	}
}

func TestListAccessibleEnvironments_IgnoresInvalidConnectionScopedEnvPermission(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-env-invalid-conn-env", "Org")
	ownerID := newAccount(t, db, "owner-env-invalid-conn-env@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)
	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	envA, _ := db.InsertEnvironment(context.Background(), ws.ID, "env-a", "")
	connA, _ := db.InsertConnection(context.Background(), ws.ID, &envA.ID, "conn-a", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-env-invalid-conn-env@example.com")
	_ = insertTestScopedRoleBinding(t, db, org.ID, &ws.ID, "invalid-conn-env", "connection", []string{access.PermEnvRead}, "account", userID, "connection", connA.ID)

	envs, err := db.ListAccessibleEnvironments(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected invalid connection-scoped env permission to be ignored for environment discovery, got %v", envIDs(envs))
	}
}

func TestListAccessibleConnections_IgnoresInvalidWorkspaceScopedEnvPermission(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-invalid-ws-env", "Org")
	ownerID := newAccount(t, db, "owner-conn-invalid-ws-env@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	env, _ := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	_, _ = db.InsertConnection(context.Background(), ws.ID, &env.ID, "db1", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-invalid-ws-env@example.com")
	_ = insertTestScopedRoleBinding(t, db, org.ID, &ws.ID, "invalid-ws-env", "workspace", []string{access.PermEnvRead}, "account", userID, "workspace", ws.ID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 0 {
		t.Fatalf("expected invalid workspace-scoped env permission to be ignored for connection discovery, got %v", connIDs(conns))
	}
}
