package access

import (
	"context"
	"errors"
	"testing"
)

type serviceStore struct {
	roles            map[int64]Role
	bindings         map[int64]RoleBinding
	accounts         map[int64]bool
	memberships      map[[2]int64]bool
	teams            map[int64]int64
	workspaces       map[int64]int64
	environments     map[int64]int64
	connections      map[int64]int64
	roleBindingCount int
}

func (s *serviceStore) Role(_ context.Context, orgID, roleID int64) (Role, bool, error) {
	role, ok := s.roles[roleID]
	return role, ok && role.OrgID == orgID, nil
}
func (s *serviceStore) RoleBinding(_ context.Context, orgID, bindingID int64) (RoleBinding, bool, error) {
	binding, ok := s.bindings[bindingID]
	return binding, ok && binding.OrgID == orgID, nil
}
func (s *serviceStore) CountRoleBindings(context.Context, int64, int64, string, int64) (int, error) {
	return s.roleBindingCount, nil
}
func (s *serviceStore) AccountExists(_ context.Context, accountID int64) (bool, error) {
	return s.accounts[accountID], nil
}
func (s *serviceStore) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return s.memberships[[2]int64{orgID, accountID}], nil
}
func (s *serviceStore) TeamOrg(_ context.Context, teamID int64) (int64, bool, error) {
	orgID, ok := s.teams[teamID]
	return orgID, ok, nil
}
func (s *serviceStore) WorkspaceOrg(_ context.Context, workspaceID int64) (int64, bool, error) {
	orgID, ok := s.workspaces[workspaceID]
	return orgID, ok, nil
}
func (s *serviceStore) EnvironmentWorkspace(_ context.Context, environmentID int64) (int64, bool, error) {
	workspaceID, ok := s.environments[environmentID]
	return workspaceID, ok, nil
}
func (s *serviceStore) ConnectionWorkspace(_ context.Context, connectionID int64) (int64, bool, error) {
	workspaceID, ok := s.connections[connectionID]
	return workspaceID, ok, nil
}

type roleManager struct {
	createCalls int
	bindCalls   int
	unbindCalls int
}

func (m *roleManager) CreateRole(context.Context, int64, *int64, string, string, string, []string) (int64, error) {
	m.createCalls++
	return 99, nil
}
func (*roleManager) UpdateRole(context.Context, int64, int64, string, string, []string) error {
	return nil
}
func (*roleManager) DeleteRole(context.Context, int64, int64) error { return nil }
func (m *roleManager) BindRole(context.Context, int64, int64, string, int64, string, int64, int64) error {
	m.bindCalls++
	return nil
}
func (m *roleManager) UnbindRole(context.Context, int64, int64) error {
	m.unbindCalls++
	return nil
}

type fixedPolicy bool

func (p fixedPolicy) Can(context.Context, int64, int64, string, string, int64, string) bool {
	return bool(p)
}
func (p fixedPolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	if p {
		return []string{PermWsRead}, nil
	}
	return nil, nil
}

func newServiceFixture() (*Service, *serviceStore, *roleManager) {
	store := &serviceStore{
		roles:    map[int64]Role{7: {ID: 7, OrgID: 1, ScopeType: resourceWorkspace}},
		bindings: map[int64]RoleBinding{}, accounts: map[int64]bool{2: true},
		memberships: map[[2]int64]bool{{1, 2}: true}, workspaces: map[int64]int64{10: 1, 20: 2},
		environments: map[int64]int64{30: 10, 40: 20}, connections: map[int64]int64{},
	}
	roles := &roleManager{}
	return NewService(store, roles, fixedPolicy(true)), store, roles
}

func TestGrantOrgPolicyRequiresExistingMembership(t *testing.T) {
	t.Parallel()
	service, store, roles := newServiceFixture()
	store.memberships[[2]int64{1, 2}] = false
	store.roles[7] = Role{ID: 7, OrgID: 1, ScopeType: resourceOrg}

	err := service.GrantOrgPolicy(context.Background(), GrantOrgPolicyInput{
		OrgID: 1, GrantorID: 3, RoleID: 7, SubjectType: SubjectTypeAccount, SubjectID: 2,
	})
	if !errors.Is(err, ErrSubjectNotFound) {
		t.Fatalf("error = %v, want ErrSubjectNotFound", err)
	}
	if roles.bindCalls != 0 {
		t.Fatalf("bind calls = %d, want 0", roles.bindCalls)
	}
}

func TestGrantWorkspacePolicyRejectsCrossWorkspaceResource(t *testing.T) {
	t.Parallel()
	service, _, roles := newServiceFixture()
	_, err := service.GrantWorkspacePolicy(context.Background(), GrantWorkspacePolicyInput{
		OrgID: 1, WorkspaceID: 10, GrantorID: 3, RoleID: 7,
		SubjectType: SubjectTypeAccount, SubjectID: 2,
		ResourceType: resourceEnvironment, ResourceID: 40,
	})
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("error = %v, want ErrResourceNotFound", err)
	}
	if roles.bindCalls != 0 {
		t.Fatalf("bind calls = %d, want 0", roles.bindCalls)
	}
}

func TestWorkspacePolicyBindingRejectsAnotherWorkspace(t *testing.T) {
	t.Parallel()
	service, store, _ := newServiceFixture()
	store.bindings[8] = RoleBinding{
		ID: 8, OrgID: 1, RoleID: 7, ResourceType: resourceWorkspace, ResourceID: 20,
	}

	_, err := service.WorkspacePolicyBinding(context.Background(), 1, 10, 8)
	if !errors.Is(err, ErrRoleBindingNotFound) {
		t.Fatalf("error = %v, want ErrRoleBindingNotFound", err)
	}
}

func TestCreateRoleValidatesOwnerScopeBeforeWriting(t *testing.T) {
	t.Parallel()
	service, _, roles := newServiceFixture()
	_, err := service.CreateOrgRole(context.Background(), OrgRoleInput{OrgID: 1, ScopeType: resourceWorkspace})
	if !errors.Is(err, ErrInvalidRoleScope) {
		t.Fatalf("error = %v, want ErrInvalidRoleScope", err)
	}
	if roles.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", roles.createCalls)
	}
}

func TestEffectivePermissionsEnforcesMembershipAndTenantBoundary(t *testing.T) {
	t.Parallel()
	service, store, _ := newServiceFixture()

	store.memberships[[2]int64{1, 2}] = false
	_, _, err := service.EffectivePermissions(context.Background(), EffectivePermissionsInput{
		AccountID: 2, OrgID: 1, ResourceType: resourceWorkspace, ResourceID: 10,
	})
	if !errors.Is(err, ErrSubjectNotFound) {
		t.Fatalf("non-member error = %v, want ErrSubjectNotFound", err)
	}

	store.memberships[[2]int64{1, 2}] = true
	_, _, err = service.EffectivePermissions(context.Background(), EffectivePermissionsInput{
		AccountID: 2, OrgID: 1, ResourceType: resourceWorkspace, ResourceID: 20,
	})
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("cross-tenant error = %v, want ErrResourceNotFound", err)
	}
}

func TestRevokeOrgPolicyProtectsLastOwner(t *testing.T) {
	t.Parallel()
	service, store, roles := newServiceFixture()
	store.roles[7] = Role{ID: 7, OrgID: 1, Name: BuiltinOrgOwnerRole, ScopeType: resourceOrg, IsBuiltin: true}
	store.bindings[8] = RoleBinding{ID: 8, OrgID: 1, RoleID: 7, ResourceType: resourceOrg, ResourceID: 1}
	store.roleBindingCount = 1

	_, err := service.RevokeOrgPolicy(context.Background(), RevokeOrgPolicyInput{OrgID: 1, GrantorID: 3, BindingID: 8})
	if !errors.Is(err, ErrLastOwnerPolicy) {
		t.Fatalf("error = %v, want ErrLastOwnerPolicy", err)
	}
	if roles.unbindCalls != 0 {
		t.Fatalf("unbind calls = %d, want 0", roles.unbindCalls)
	}
}
