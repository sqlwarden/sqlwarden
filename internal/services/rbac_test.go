package services

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/rbac"
)

func seedRBACOrg(t *testing.T) (*Services, Deps, database.Organization, database.Account, database.Account) {
	t.Helper()
	deps := newTestDeps(t)
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	org, owner := seedOwnedOrg(t, svc, deps, "rbac-org")
	member, err := deps.DB.InsertAccount(context.Background(), "rbac-member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	return svc, deps, org, owner, member
}

func orgBindings(t *testing.T, deps Deps, orgID, roleID, subjectID int64) []database.RoleBinding {
	t.Helper()
	var bindings []database.RoleBinding
	err := deps.DB.NewSelect().Model(&bindings).
		Where("org_id = ? AND role_id = ? AND subject_type = ? AND subject_id = ? AND resource_type = 'org'", orgID, roleID, access.SubjectTypeAccount, subjectID).
		Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return bindings
}

func TestRBACGrantPolicyIsIdempotent(t *testing.T) {
	svc, deps, org, owner, member := seedRBACOrg(t)
	ctx := context.Background()
	role, err := svc.RBAC.CreateRole(ctx, accountPrincipal(owner.ID), org.ID, rbac.CreateRoleInput{Name: "Readers", Permissions: []string{access.PermPolicyRead}})
	if err != nil {
		t.Fatal(err)
	}
	in := rbac.GrantPolicyInput{RoleID: role.ID, SubjectType: access.SubjectTypeAccount, SubjectID: member.ID}
	for range 2 {
		if err := svc.RBAC.GrantPolicy(ctx, accountPrincipal(owner.ID), org.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	assert.Equal(t, len(orgBindings(t, deps, org.ID, role.ID, member.ID)), 1)
}

func TestRBACExpiredBindingGrantsNothing(t *testing.T) {
	svc, deps, org, owner, member := seedRBACOrg(t)
	ctx := context.Background()
	role, err := svc.RBAC.CreateRole(ctx, accountPrincipal(owner.ID), org.ID, rbac.CreateRoleInput{Name: "Policy readers", Permissions: []string{access.PermPolicyRead}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RBAC.GrantPolicy(ctx, accountPrincipal(owner.ID), org.ID, rbac.GrantPolicyInput{RoleID: role.ID, SubjectType: access.SubjectTypeAccount, SubjectID: member.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.RBAC.ListRoles(ctx, accountPrincipal(member.ID), org.ID, rbac.ListRolesInput{})
	assert.Nil(t, err)

	bindings := orgBindings(t, deps, org.ID, role.ID, member.ID)
	assert.Equal(t, len(bindings), 1)
	if _, err := deps.DB.NewUpdate().Model((*database.RoleBinding)(nil)).
		Set("expires_at = ?", time.Now().Add(-time.Minute)).
		Where("id = ?", bindings[0].ID).
		Exec(ctx); err != nil {
		t.Fatal(err)
	}
	deps.Enforcer.InvalidateOrgPolicy(org.ID)

	_, err = svc.RBAC.ListRoles(ctx, accountPrincipal(member.ID), org.ID, rbac.ListRolesInput{})
	assert.ErrorIs(t, err, access.ErrForbidden)
}

func TestRBACDeleteRoleInUseIsRefused(t *testing.T) {
	svc, _, org, owner, member := seedRBACOrg(t)
	ctx := context.Background()
	role, err := svc.RBAC.CreateRole(ctx, accountPrincipal(owner.ID), org.ID, rbac.CreateRoleInput{Name: "Bound", Permissions: []string{access.PermOrgRead}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RBAC.GrantPolicy(ctx, accountPrincipal(owner.ID), org.ID, rbac.GrantPolicyInput{RoleID: role.ID, SubjectType: access.SubjectTypeAccount, SubjectID: member.ID}); err != nil {
		t.Fatal(err)
	}
	err = svc.RBAC.DeleteRole(ctx, accountPrincipal(owner.ID), org.ID, role.ID)
	var inUse access.RoleInUseError
	if !errors.As(err, &inUse) || inUse.BindingCount != 1 {
		t.Fatalf("DeleteRole error = %v, want role in use with one binding", err)
	}
	_, err = svc.RBAC.GetRole(ctx, accountPrincipal(owner.ID), org.ID, role.ID)
	assert.Nil(t, err)
}

func TestRBACRevokeLastOwnerIsRefused(t *testing.T) {
	svc, deps, org, owner, _ := seedRBACOrg(t)
	ctx := context.Background()
	ownerRoleID := builtinOrgRoleIDs(t, deps, org.ID)[access.BuiltinOrgOwnerRole]
	bindings := orgBindings(t, deps, org.ID, ownerRoleID, owner.ID)
	assert.Equal(t, len(bindings), 1)

	_, err := svc.RBAC.RevokePolicy(ctx, accountPrincipal(owner.ID), org.ID, bindings[0].ID)
	assert.ErrorIs(t, err, rbac.ErrLastOwner)
	assert.Equal(t, len(orgBindings(t, deps, org.ID, ownerRoleID, owner.ID)), 1)
}

func TestRBACWorkspacePolicyBindsOnlyInsideWorkspace(t *testing.T) {
	svc, deps, org, owner, member := seedRBACOrg(t)
	ctx := context.Background()
	p := accountPrincipal(owner.ID)
	ws, err := svc.Catalog.CreateWorkspace(ctx, p, org.ID, catalog.CreateWorkspaceInput{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Catalog.CreateWorkspace(ctx, p, org.ID, catalog.CreateWorkspaceInput{Name: "Finance"})
	if err != nil {
		t.Fatal(err)
	}
	ownEnv, err := deps.DB.InsertEnvironment(ctx, ws.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	otherEnv, err := deps.DB.InsertEnvironment(ctx, other.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	role, err := svc.RBAC.CreateWorkspaceRole(ctx, p, org.ID, ws.ID, rbac.CreateWorkspaceRoleInput{Name: "Env readers", ScopeType: "environment", Permissions: []string{access.PermEnvRead}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.RBAC.GrantWorkspacePolicy(ctx, p, org.ID, ws.ID, rbac.GrantWorkspacePolicyInput{RoleID: role.ID, SubjectType: access.SubjectTypeAccount, SubjectID: member.ID, ResourceType: "environment", ResourceID: otherEnv.ID})
	assert.ErrorIs(t, err, access.ErrNotFound)

	in := rbac.GrantWorkspacePolicyInput{RoleID: role.ID, SubjectType: access.SubjectTypeAccount, SubjectID: member.ID, ResourceType: "environment", ResourceID: ownEnv.ID}
	for range 2 {
		if _, err := svc.RBAC.GrantWorkspacePolicy(ctx, p, org.ID, ws.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	var bindings []database.RoleBinding
	if err := deps.DB.NewSelect().Model(&bindings).Where("role_id = ?", role.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(bindings), 1)
	assert.Equal(t, bindings[0].ResourceID, ownEnv.ID)

	_, err = svc.RBAC.GetWorkspacePolicy(ctx, p, org.ID, other.ID, bindings[0].ID)
	assert.ErrorIs(t, err, access.ErrNotFound)

	effective, err := svc.RBAC.EffectivePermissions(ctx, accountPrincipal(member.ID), org.ID, rbac.ResourceRef{Type: "environment", ID: strconv.FormatInt(ownEnv.ID, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(effective.Permissions, access.PermEnvRead) {
		t.Fatalf("effective permissions = %v, want %s", effective.Permissions, access.PermEnvRead)
	}
}

func TestRBACBuiltinRoleIsImmutable(t *testing.T) {
	svc, deps, org, owner, _ := seedRBACOrg(t)
	ctx := context.Background()
	adminRoleID := builtinOrgRoleIDs(t, deps, org.ID)[access.BuiltinOrgAdminRole]
	_, err := svc.RBAC.UpdateRole(ctx, accountPrincipal(owner.ID), org.ID, adminRoleID, rbac.UpdateRoleInput{Name: "Renamed"})
	assert.ErrorIs(t, err, rbac.ErrBuiltinRole)
	assert.ErrorIs(t, svc.RBAC.DeleteRole(ctx, accountPrincipal(owner.ID), org.ID, adminRoleID), rbac.ErrBuiltinRole)
}
