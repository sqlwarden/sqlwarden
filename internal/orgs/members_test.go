package orgs_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/platform/invalidate"
)

func TestRemoveMemberProtectsLastOwner(t *testing.T) {
	h := newHarness(t)
	err := h.svc.RemoveMember(context.Background(), account(ownerID), testOrg, ownerID)
	if !errors.Is(err, orgs.ErrLastOwner) {
		t.Fatalf("RemoveMember(last owner) = %v, want ErrLastOwner", err)
	}
	if len(h.store.removed) != 0 || len(h.revoker.calls) != 0 || len(h.events) != 0 {
		t.Fatal("blocked removal had side effects")
	}

	h.store.owners[coOwnerID] = true
	h.store.members[coOwnerID] = true
	if err := h.svc.RemoveMember(context.Background(), account(ownerID), testOrg, ownerID); err != nil {
		t.Fatalf("RemoveMember(owner with co-owner) = %v", err)
	}
}

func TestRemoveMemberRevokesSessionsAndAudits(t *testing.T) {
	h := newHarness(t)
	if err := h.svc.RemoveMember(context.Background(), account(writerID), testOrg, readerID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.store.removed, []int64{readerID}) || h.store.removedRevokedBy == nil || *h.store.removedRevokedBy != writerID {
		t.Fatalf("removed=%v revokedBy=%v", h.store.removed, h.store.removedRevokedBy)
	}
	if len(h.revoker.calls) != 1 || h.revoker.calls[0] != [2]string{"1", "8"} {
		t.Fatalf("revoke calls = %v", h.revoker.calls)
	}
	if len(h.cache.principals) != 1 || h.cache.principals[0] != [2]int64{testOrg, readerID} {
		t.Fatalf("principal invalidations = %v", h.cache.principals)
	}
	if got := h.actions(); len(got) != 1 || got[0] != orgs.ActionMemberRemoved || h.events[0].ResourceID != "8" {
		t.Fatalf("audit = %+v", h.events)
	}
}

func TestRemoveMemberPublishesCredentialRevocation(t *testing.T) {
	h := newHarness(t)
	var keys []string
	h.inval.Subscribe(invalidate.TopicCredentials, func(key string) { keys = append(keys, key) })
	if err := h.svc.RemoveMember(context.Background(), account(writerID), testOrg, readerID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, []string{"account:8"}) {
		t.Fatalf("published keys = %v", keys)
	}
}

func TestRemoveMemberPublishesNothingWhenStoreFails(t *testing.T) {
	h := newHarness(t)
	var keys []string
	h.inval.Subscribe(invalidate.TopicCredentials, func(key string) { keys = append(keys, key) })
	if err := h.svc.RemoveMember(context.Background(), account(ownerID), testOrg, ownerID); err == nil {
		t.Fatal("expected last owner error")
	}
	if len(keys) != 0 {
		t.Fatalf("published keys = %v", keys)
	}
}

func TestRemoveMemberSucceedsWhenRevocationFails(t *testing.T) {
	h := newHarness(t)
	h.revoker.err = errors.New("runtime unavailable")
	if err := h.svc.RemoveMember(context.Background(), account(writerID), testOrg, readerID); err != nil {
		t.Fatalf("RemoveMember with failing revoker = %v", err)
	}
	if len(h.store.removed) != 1 {
		t.Fatal("membership was not removed")
	}
}

func TestUpdateMemberRoleProtectsLastOwner(t *testing.T) {
	ctx := context.Background()
	for _, role := range []string{access.BuiltinOrgAdminRole, access.BuiltinOrgMemberRole} {
		h := newHarness(t)
		err := h.svc.UpdateMemberRole(ctx, account(ownerID), testOrg, ownerID, role)
		if !errors.Is(err, orgs.ErrLastOwner) {
			t.Fatalf("demote last owner to %s = %v, want ErrLastOwner", role, err)
		}
		if len(h.store.replaced) != 0 {
			t.Fatal("blocked demotion replaced a role")
		}
	}

	h := newHarness(t)
	if err := h.svc.UpdateMemberRole(ctx, account(ownerID), testOrg, ownerID, access.BuiltinOrgOwnerRole); err != nil {
		t.Fatalf("keeping the last owner an owner = %v", err)
	}
}

func TestUpdateMemberRoleRequiresPrivilegedPermissionsToGrantOwner(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	accesstest.AssertForbidden(t, h.svc.UpdateMemberRole(ctx, account(transferID), testOrg, readerID, access.BuiltinOrgOwnerRole), "grant owner without org:delete")
	if len(h.store.replaced) != 0 {
		t.Fatal("denied grant replaced a role")
	}

	if err := h.svc.UpdateMemberRole(ctx, account(ownerID), testOrg, readerID, access.BuiltinOrgOwnerRole); err != nil {
		t.Fatalf("owner grants owner = %v", err)
	}
	if err := h.svc.UpdateMemberRole(ctx, account(transferID), testOrg, readerID, access.BuiltinOrgAdminRole); err != nil {
		t.Fatalf("grant admin with org:transfer_ownership = %v", err)
	}
}

func TestUpdateMemberRoleReplacesEveryBuiltinRole(t *testing.T) {
	h := newHarness(t)
	if err := h.svc.UpdateMemberRole(context.Background(), account(ownerID), testOrg, readerID, access.BuiltinOrgAdminRole); err != nil {
		t.Fatal(err)
	}
	if len(h.store.replaced) != 1 {
		t.Fatalf("replace calls = %v", h.store.replaced)
	}
	call := h.store.replaced[0]
	if call.accountID != readerID || call.roleID != adminRoleID {
		t.Fatalf("replace call = %+v", call)
	}
	if !slices.Equal(call.replacedRoleIDs, []int64{ownerRoleID, adminRoleID, memberRoleID}) {
		t.Fatalf("replaced roles = %v, want only the builtin org roles", call.replacedRoleIDs)
	}
	if len(h.cache.orgs) != 1 || len(h.cache.principals) != 1 {
		t.Fatalf("invalidations orgs=%v principals=%v", h.cache.orgs, h.cache.principals)
	}
	if got := h.actions(); len(got) != 1 || got[0] != orgs.ActionMemberRoleChanged {
		t.Fatalf("audit = %v", got)
	}
}

func TestUpdateMemberRoleFailureLeavesNoPartialEffects(t *testing.T) {
	h := newHarness(t)
	h.store.replaceErr = errors.New("insert failed")
	if err := h.svc.UpdateMemberRole(context.Background(), account(ownerID), testOrg, readerID, access.BuiltinOrgAdminRole); err == nil {
		t.Fatal("expected replacement failure")
	}
	if len(h.cache.orgs)+len(h.cache.principals)+len(h.events) != 0 {
		t.Fatal("failed replacement invalidated caches or audited")
	}
}

func TestUpdateMemberRoleValidation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	var v *orgs.ValidationError
	if err := h.svc.UpdateMemberRole(ctx, account(ownerID), testOrg, readerID, ""); !errors.As(err, &v) || v.Fields["role"] != "Role is required." {
		t.Fatalf("empty role = %v", err)
	}
	if err := h.svc.UpdateMemberRole(ctx, account(ownerID), testOrg, readerID, "Custom"); !errors.As(err, &v) || v.Fields["role"] != "Role must be Owner, Administrator, or Baseline Access." {
		t.Fatalf("custom role = %v", err)
	}
}

func TestMemberTargetsOutsideTheOrgAreNotFound(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetMember(ctx, account(readerID), testOrg, nonMemberID)), "GetMember")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.ListMemberTeams(ctx, account(readerID), testOrg, nonMemberID, database.ListTeamsParams{})), "ListMemberTeams")
	accesstest.AssertNotFound(t, h.svc.UpdateMemberRole(ctx, account(ownerID), testOrg, nonMemberID, access.BuiltinOrgAdminRole), "UpdateMemberRole")
}

func TestListMembersScopesToOrgAndValidatesRole(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	page, err := h.svc.ListMembers(ctx, account(readerID), testOrg, database.ListOrgMembersParams{OrgID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if page.Items[0].OrgID != testOrg {
		t.Fatalf("listed org %d, want %d", page.Items[0].OrgID, testOrg)
	}
	var v *orgs.ValidationError
	_, err = h.svc.ListMembers(ctx, account(readerID), testOrg, database.ListOrgMembersParams{Role: "Custom"})
	if !errors.As(err, &v) {
		t.Fatalf("invalid role filter = %v", err)
	}
}
