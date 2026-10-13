package services

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/orgs"
)

type orgRevoker struct {
	execution.Revoker
	mu    sync.Mutex
	err   error
	calls [][2]string
}

func (r *orgRevoker) RevokeOrgAccount(_ context.Context, orgID, accountID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, [2]string{orgID, accountID})
	return 1, r.err
}

func seedOwnedOrg(t *testing.T, svc *Services, deps Deps, slug string) (database.Organization, database.Account) {
	t.Helper()
	ctx := context.Background()
	owner, err := deps.DB.InsertAccount(ctx, slug+"-owner@example.com", "Owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.InsertInstanceAdmin(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}
	admin := accountPrincipal(owner.ID)
	admin.InstanceAdmin = true
	org, err := svc.Orgs.Create(ctx, admin, orgs.CreateOrgInput{Name: slug})
	if err != nil {
		t.Fatal(err)
	}
	return org, owner
}

func builtinOrgRoleIDs(t *testing.T, deps Deps, orgID int64) map[string]int64 {
	t.Helper()
	roles, err := deps.DB.ListOrgRoles(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, role := range roles {
		if role.IsBuiltin {
			ids[role.Name] = role.ID
		}
	}
	return ids
}

func TestOrgStoreCreateOwnedOrgRollsBackWhenOwnerMembershipFails(t *testing.T) {
	deps := newTestDeps(t)
	store := orgStore{DB: deps.DB, enforcer: deps.Enforcer}
	ctx := context.Background()

	if _, err := store.CreateOwnedOrg(ctx, "rollback-org", "Rollback Org", 99999999); err == nil {
		t.Fatal("expected owner membership failure")
	}
	_, found, err := deps.DB.GetOrgBySlug(ctx, "rollback-org")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected organization insert to roll back")
	}
	var roles []database.Role
	if err := deps.DB.NewSelect().Model(&roles).Where("name IN (?)", []string{access.BuiltinOrgOwnerRole, access.BuiltinOrgAdminRole, access.BuiltinOrgMemberRole}).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected seeded roles to roll back, got %d", len(roles))
	}
}

func TestOrgStoreReplaceBuiltinRoleRollsBackWhenNewBindingFails(t *testing.T) {
	deps := newTestDeps(t)
	svc, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "role-replace")
	member, err := deps.DB.InsertAccount(ctx, "role-replace-member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ids := builtinOrgRoleIDs(t, deps, org.ID)
	adminRoleID := ids[access.BuiltinOrgAdminRole]
	if err := deps.Enforcer.BindRole(ctx, org.ID, adminRoleID, "account", member.ID, "org", org.ID, owner.ID); err != nil {
		t.Fatal(err)
	}

	store := orgStore{DB: deps.DB, enforcer: deps.Enforcer}
	replaced := []int64{ids[access.BuiltinOrgOwnerRole], adminRoleID}
	if err := store.ReplaceOrgMemberBuiltinRole(ctx, org.ID, member.ID, 99999999, replaced, owner.ID); err == nil {
		t.Fatal("expected invalid role binding failure")
	}
	hasAdmin, err := deps.DB.AccountHasRoleBinding(ctx, org.ID, adminRoleID, member.ID, "org", org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAdmin {
		t.Fatal("expected previous admin binding to remain after rollback")
	}
}

func TestOrgsUpdateMemberRoleReplacesBuiltinRole(t *testing.T) {
	deps := newTestDeps(t)
	svc, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "role-update")
	member, err := deps.DB.InsertAccount(ctx, "role-update-member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ids := builtinOrgRoleIDs(t, deps, org.ID)

	assert.Nil(t, svc.Orgs.UpdateMemberRole(ctx, accountPrincipal(owner.ID), org.ID, member.ID, access.BuiltinOrgAdminRole))
	assert.Nil(t, svc.Orgs.UpdateMemberRole(ctx, accountPrincipal(owner.ID), org.ID, member.ID, access.BuiltinOrgOwnerRole))

	for name, want := range map[string]bool{
		access.BuiltinOrgAdminRole: false,
		access.BuiltinOrgOwnerRole: true,
	} {
		has, err := deps.DB.AccountHasRoleBinding(ctx, org.ID, ids[name], member.ID, "org", org.ID)
		if err != nil {
			t.Fatal(err)
		}
		if has != want {
			t.Fatalf("binding to %s = %v, want %v", name, has, want)
		}
	}
}

func TestOrgsRemoveMemberRevokesOrgSessions(t *testing.T) {
	deps := newTestDeps(t)
	revoker := &orgRevoker{Revoker: deps.Runtime}
	deps.Revoker = revoker
	svc, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "remove-member")
	member, err := deps.DB.InsertAccount(ctx, "remove-member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, svc.Orgs.RemoveMember(ctx, accountPrincipal(owner.ID), org.ID, member.ID))

	want := [2]string{strconv.FormatInt(org.ID, 10), strconv.FormatInt(member.ID, 10)}
	if len(revoker.calls) != 1 || revoker.calls[0] != want {
		t.Fatalf("revoke calls = %v, want [%v]", revoker.calls, want)
	}
	isMember, err := deps.DB.IsOrgMember(ctx, org.ID, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if isMember {
		t.Fatal("member was not removed")
	}
	assert.Equal(t, sessionRevokeJobs(t, deps), 0)
}

func TestQueuedRevokerQueuesOrgAccountRetryOnFailure(t *testing.T) {
	deps := newTestDeps(t)
	revoker := &orgRevoker{Revoker: deps.Runtime, err: errors.New("runtime unavailable")}
	queued := queuedRevoker{revoker: revoker, queue: jobs.NewStore(deps.DB), logger: deps.Logger}

	revoked, err := queued.RevokeOrgAccount(context.Background(), "7", "9")
	assert.Nil(t, err)
	assert.Equal(t, revoked, 0)
	assert.Equal(t, sessionRevokeJobs(t, deps), 1)

	in := jobs.SessionRevokeInput{Kind: jobs.SessionRevokeOrgAccount, OrgID: "7", AccountID: "9"}
	if _, found, err := jobs.NewStore(deps.DB).ActiveBySingletonKey(context.Background(), in.UniqueKey()); err != nil || !found {
		t.Fatalf("retry job for %s found=%v err=%v", in.UniqueKey(), found, err)
	}
}

func sessionRevokeJobs(t *testing.T, deps Deps) int {
	t.Helper()
	n, err := deps.DB.NewSelect().TableExpr("jobs").Where("type = ?", jobs.TypeSessionRevoke).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}
