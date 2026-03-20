package access

import (
	"log/slog"
	"os"
	"testing"

	"github.com/sqlwarden/internal/database"
)

func newTestEnforcer(t *testing.T) *Enforcer {
	t.Helper()

	dir := t.TempDir()
	dsn := dir + "/test.db"

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	db, err := database.New("sqlite", dsn, logger, false)
	if err != nil {
		t.Fatal(err)
	}

	err = db.MigrateUp()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	enf, err := New(db.DB)
	if err != nil {
		t.Fatal(err)
	}

	return enf
}

func TestSeedOrgAndOwnerAccess(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	member := "member-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	// Owner should have full access.
	if !enf.Can(owner, slug, "*", "manage") {
		t.Error("owner should be able to manage *")
	}
	if !enf.Can(owner, slug, "workspace:ws1", "connect") {
		t.Error("owner should be able to connect to workspace:ws1")
	}

	// Member without any grants should have no access.
	if enf.CanOnConnection(member, slug, "conn1", "ws1", "connect") {
		t.Error("member should not have connection access without grants")
	}
}

func TestWorkspaceGrant(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	err = enf.GrantWorkspaceAccess(user, slug, "ws1", "query")
	if err != nil {
		t.Fatal(err)
	}

	// User should be able to query via workspace.
	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "query") {
		t.Error("user should be able to query via workspace grant")
	}

	// connect <= query, so connect should also be allowed.
	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "connect") {
		t.Error("user should be able to connect (implied by query)")
	}

	// execute > query, so execute should be denied.
	if enf.CanOnConnection(user, slug, "conn1", "ws1", "execute") {
		t.Error("user should not be able to execute with only query grant")
	}
}

func TestConnectionOverride(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	err = enf.GrantWorkspaceAccess(user, slug, "ws1", "query")
	if err != nil {
		t.Fatal(err)
	}

	err = enf.GrantConnectionOverride(user, slug, "conn1", "execute")
	if err != nil {
		t.Fatal(err)
	}

	// With connection override, execute should be allowed on conn1.
	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "execute") {
		t.Error("user should be able to execute with connection override")
	}

	// conn2 has no override, so execute should be denied.
	if enf.CanOnConnection(user, slug, "conn2", "ws1", "execute") {
		t.Error("user should not be able to execute on conn2 without override")
	}
}

func TestRevokeConnectionOverride(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	err = enf.GrantWorkspaceAccess(user, slug, "ws1", "query")
	if err != nil {
		t.Fatal(err)
	}

	err = enf.GrantConnectionOverride(user, slug, "conn1", "execute")
	if err != nil {
		t.Fatal(err)
	}

	err = enf.RevokeConnectionOverride(user, slug, "conn1")
	if err != nil {
		t.Fatal(err)
	}

	// After revoking override, should fall back to workspace query.
	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "query") {
		t.Error("user should still have workspace query after revoking override")
	}

	if enf.CanOnConnection(user, slug, "conn1", "ws1", "execute") {
		t.Error("user should not have execute after revoking override")
	}
}

func TestTeamAdditiveUnion(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	// Team gets workspace query.
	err = enf.GrantWorkspaceAccess("team:t1", slug, "ws1", "query")
	if err != nil {
		t.Fatal(err)
	}

	// User gets direct connection execute.
	err = enf.GrantConnectionOverride(user, slug, "conn1", "execute")
	if err != nil {
		t.Fatal(err)
	}

	// Add user to team.
	err = enf.AddTeamMember(user, "t1", slug)
	if err != nil {
		t.Fatal(err)
	}

	// User should have execute on conn1 via union.
	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "execute") {
		t.Error("user should have execute via union of team + direct")
	}

	// Remove from team.
	err = enf.RemoveTeamMember(user, "t1", slug)
	if err != nil {
		t.Fatal(err)
	}

	// Should still have execute from direct grant.
	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "execute") {
		t.Error("user should still have execute from direct grant after leaving team")
	}
}

func TestCustomRole(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	err = enf.SeedCustomRole(slug, "role1", "ws1", []string{"query"})
	if err != nil {
		t.Fatal(err)
	}

	err = enf.AssignCustomRole(user, slug, "role1")
	if err != nil {
		t.Fatal(err)
	}

	if !enf.CanOnConnection(user, slug, "conn1", "ws1", "query") {
		t.Error("user should have query via custom role")
	}

	err = enf.DeleteCustomRole(slug, "role1")
	if err != nil {
		t.Fatal(err)
	}

	if enf.CanOnConnection(user, slug, "conn1", "ws1", "query") {
		t.Error("user should not have query after custom role deleted")
	}
}

func TestSetOrgRole(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	// Assign user as admin.
	err = enf.SetOrgRole(user, "admin", slug)
	if err != nil {
		t.Fatal(err)
	}

	if !enf.Can(user, slug, "teams", "manage") {
		t.Error("admin should be able to manage teams")
	}

	// Change to a member role (no policies defined for member).
	err = enf.SetOrgRole(user, "member", slug)
	if err != nil {
		t.Fatal(err)
	}

	if enf.Can(user, slug, "teams", "manage") {
		t.Error("member should not be able to manage teams")
	}
}

func TestRemoveOrgMember(t *testing.T) {
	enf := newTestEnforcer(t)

	slug := "acme"
	owner := "owner-1"
	user := "user-1"

	err := enf.SeedOrgPolicies(slug, owner)
	if err != nil {
		t.Fatal(err)
	}

	err = enf.SetOrgRole(user, "admin", slug)
	if err != nil {
		t.Fatal(err)
	}

	if !enf.Can(user, slug, "teams", "manage") {
		t.Error("admin should be able to manage teams")
	}

	err = enf.RemoveOrgMember(user, slug)
	if err != nil {
		t.Fatal(err)
	}

	if enf.Can(user, slug, "teams", "manage") {
		t.Error("removed member should not be able to manage teams")
	}
}
