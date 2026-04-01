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
	adminRoleID := findOrgRoleID(t, db, org.ID, "admin")
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
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, "ws:member")
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
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-dirperm", "Org")
	ownerID := newAccount(t, db, "owner-ws-dirperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Bound")
	_ = seedWorkspace(t, db, e, org.ID, ownerID, "Unbound")

	userID := newAccount(t, db, "user-ws-dirperm@example.com")
	_ = e.GrantPermission(context.Background(), org.ID, access.PermWsRead, "account", userID, "workspace", ws1.ID, ownerID)

	wss, err := db.ListAccessibleWorkspaces(context.Background(), userID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].ID != ws1.ID {
		t.Fatalf("expected only ws1, got %v", wsIDs(wss))
	}
}

func TestListAccessibleWorkspaces_OrgPermBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-orgperm", "Org")
	ownerID := newAccount(t, db, "owner-ws-orgperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "Beta")

	userID := newAccount(t, db, "user-ws-orgperm@example.com")
	_ = e.GrantPermission(context.Background(), org.ID, access.PermOrgRead, "account", userID, "org", org.ID, ownerID)

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

func TestListAccessibleWorkspaces_TeamOrgBinding(t *testing.T) {
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

	adminRoleID := findOrgRoleID(t, db, org.ID, "admin")
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

	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, "ws:member")
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
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-nonmember", "Org")
	ownerID := newAccount(t, db, "owner-ws-nonmember@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "Alpha")
	team, _ := db.InsertTeam(context.Background(), org.ID, "devs-nonmember", "Devs")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, "ws:member")
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

func TestListAccessibleWorkspaces_CrossOrgIsolation(t *testing.T) {
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
	adminRoleB := findOrgRoleID(t, db, orgB.ID, "admin")
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
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-2users", "Org")
	ownerID := newAccount(t, db, "owner-ws-2users@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws1 := seedWorkspace(t, db, e, org.ID, ownerID, "AliceWS")
	ws2 := seedWorkspace(t, db, e, org.ID, ownerID, "BobWS")

	aliceID := newAccount(t, db, "alice-ws-2users@example.com")
	bobID := newAccount(t, db, "bob-ws-2users@example.com")

	_ = e.BindRole(context.Background(), org.ID, findWsRoleID(t, db, org.ID, ws1.ID, "ws:member"), "account", aliceID, "workspace", ws1.ID, ownerID)
	_ = e.BindRole(context.Background(), org.ID, findWsRoleID(t, db, org.ID, ws2.ID, "ws:member"), "account", bobID, "workspace", ws2.ID, ownerID)

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
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-ws-empty", "Org")
	ownerID := newAccount(t, db, "owner-ws-empty@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	userID := newAccount(t, db, "user-ws-empty@example.com")
	adminRoleID := findOrgRoleID(t, db, org.ID, "admin")
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
	db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

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
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-orgrole", "Org")
	ownerID := newAccount(t, db, "owner-conn-orgrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-orgrole@example.com")
	adminRoleID := findOrgRoleID(t, db, org.ID, "admin")
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

func TestListAccessibleConnections_WsRoleGrantsAllInWorkspace(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-wsrole", "Org")
	ownerID := newAccount(t, db, "owner-conn-wsrole@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsrole@example.com")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, "ws:member")
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", userID, "workspace", ws.ID, ownerID)

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

func TestListAccessibleConnections_DirectConnRoleBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-dirconn", "Org")
	ownerID := newAccount(t, db, "owner-conn-dirconn@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-dirconn@example.com")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, "ws:member")
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", userID, "connection", c1.ID, ownerID)

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
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-dirperm@example.com")
	_ = e.GrantPermission(context.Background(), org.ID, access.PermConnMetadata, "account", userID, "connection", c1.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].ID != c1.ID {
		t.Fatalf("expected only c1, got %v", connIDs(conns))
	}
}

func TestListAccessibleConnections_WsPermBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-wsperm", "Org")
	ownerID := newAccount(t, db, "owner-conn-wsperm@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsperm@example.com")
	_ = e.GrantPermission(context.Background(), org.ID, access.PermWsRead, "account", userID, "workspace", ws.ID, ownerID)

	conns, err := db.ListAccessibleConnections(context.Background(), userID, org.ID, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections via ws perm, got %d", len(conns))
	}
	ids := connIDs(conns)
	if !contains(ids, c1.ID) || !contains(ids, c2.ID) {
		t.Fatalf("missing connection IDs in %v", ids)
	}
}

func TestListAccessibleConnections_TeamOrgBinding(t *testing.T) {
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-team-org", "Org")
	ownerID := newAccount(t, db, "owner-conn-team-org@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	team, _ := db.InsertTeam(context.Background(), org.ID, "eng-conn-org", "Eng")
	userID := newAccount(t, db, "user-conn-team-org@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	adminRoleID := findOrgRoleID(t, db, org.ID, "admin")
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
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	team, _ := db.InsertTeam(context.Background(), org.ID, "eng-conn-conn", "Eng")
	userID := newAccount(t, db, "user-conn-team-conn@example.com")
	_ = db.AddTeamMember(context.Background(), team.ID, userID)

	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, "ws:member")
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "team", team.ID, "connection", c1.ID, ownerID)

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
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	_ = c1

	team, _ := db.InsertTeam(context.Background(), org.ID, "eng-nonmember", "Eng")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, "ws:member")
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
	db.InsertConnection(context.Background(), wsA.ID, nil, &orgA.ID, "org", orgA.ID, "db-a", "postgres", "enc", "open")
	cB, _ := db.InsertConnection(context.Background(), wsB.ID, nil, &orgB.ID, "org", orgB.ID, "db-b", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-xorg@example.com")
	adminRoleB := findOrgRoleID(t, db, orgB.ID, "admin")
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
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "alice-db", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "bob-db", "postgres", "enc", "open")

	aliceID := newAccount(t, db, "alice-conn-2users@example.com")
	bobID := newAccount(t, db, "bob-conn-2users@example.com")

	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws.ID, "ws:member")
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", aliceID, "connection", c1.ID, ownerID)
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", bobID, "connection", c2.ID, ownerID)

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
	c1, _ := db.InsertConnection(context.Background(), ws1.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	_, _ = db.InsertConnection(context.Background(), ws2.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")

	userID := newAccount(t, db, "user-conn-wsscope@example.com")
	wsMemberRoleID := findWsRoleID(t, db, org.ID, ws1.ID, "ws:member")
	_ = e.BindRole(context.Background(), org.ID, wsMemberRoleID, "account", userID, "workspace", ws1.ID, ownerID)

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
	db := newTestDB(t)
	e := newEnforcer(t, db)

	org, _ := db.InsertOrg(context.Background(), "acc-conn-owner", "Org")
	ownerID := newAccount(t, db, "owner-conn-owner@example.com")
	_ = e.SeedOrg(context.Background(), org.ID, ownerID)

	ws := seedWorkspace(t, db, e, org.ID, ownerID, "Main")
	c1, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db1", "postgres", "enc", "open")
	c2, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db2", "postgres", "enc", "open")
	c3, _ := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "db3", "postgres", "enc", "open")

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
