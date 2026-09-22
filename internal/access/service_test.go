package access

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/audit"
)

// recordingAudit captures the audit intent a use case emitted.
type recordingAudit struct {
	events []audit.Event
	err    error
}

func (r *recordingAudit) Write(_ context.Context, event audit.Event) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, event)
	return nil
}

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
	service, store, roles, _ := newAuditedServiceFixture()
	return service, store, roles
}

func newAuditedServiceFixture() (*Service, *serviceStore, *roleManager, *recordingAudit) {
	store := &serviceStore{
		roles:    map[int64]Role{7: {ID: 7, OrgID: 1, ScopeType: resourceWorkspace}},
		bindings: map[int64]RoleBinding{}, accounts: map[int64]bool{2: true},
		memberships: map[[2]int64]bool{{1, 2}: true}, workspaces: map[int64]int64{10: 1, 20: 2},
		environments: map[int64]int64{30: 10, 40: 20}, connections: map[int64]int64{},
	}
	roles := &roleManager{}
	recorder := &recordingAudit{}
	return NewService(store, roles, fixedPolicy(true), recorder), store, roles, recorder
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

func TestGrantOrgPolicyEmitsAuditIntent(t *testing.T) {
	t.Parallel()
	service, store, _, recorder := newAuditedServiceFixture()
	store.roles[7] = Role{ID: 7, OrgID: 1, ScopeType: resourceOrg}

	err := service.GrantOrgPolicy(context.Background(), GrantOrgPolicyInput{
		OrgID: 1, GrantorID: 3, RoleID: 7, SubjectType: SubjectTypeAccount, SubjectID: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("emitted %d audit events, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Action != ActionPolicyGrant || event.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit action/outcome = %q/%q", event.Action, event.Outcome)
	}
	if event.AccountID == nil || *event.AccountID != 3 {
		t.Errorf("audit actor = %v, want 3", event.AccountID)
	}
	if event.OrgID == nil || *event.OrgID != 1 {
		t.Errorf("audit org = %v, want 1", event.OrgID)
	}
	if event.Metadata["role_id"] != "7" || event.Metadata["subject_id"] != "2" {
		t.Errorf("audit metadata = %v", event.Metadata)
	}
}

func TestRefusedGrantEmitsNoAuditIntent(t *testing.T) {
	t.Parallel()
	service, store, _, recorder := newAuditedServiceFixture()
	store.memberships[[2]int64{1, 2}] = false
	store.roles[7] = Role{ID: 7, OrgID: 1, ScopeType: resourceOrg}

	err := service.GrantOrgPolicy(context.Background(), GrantOrgPolicyInput{
		OrgID: 1, GrantorID: 3, RoleID: 7, SubjectType: SubjectTypeAccount, SubjectID: 2,
	})
	if !errors.Is(err, ErrSubjectNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrSubjectNotFound)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("emitted %d audit events for a refused grant, want 0", len(recorder.events))
	}
}

func TestUnauditableRoleCreationFailsTheUseCase(t *testing.T) {
	t.Parallel()
	service, _, _, recorder := newAuditedServiceFixture()
	recorder.err = errors.New("audit storage unavailable")

	_, err := service.CreateOrgRole(context.Background(), OrgRoleInput{
		OrgID: 1, Name: "auditor", ScopeType: scopeOrg, Permissions: []string{PermOrgRead},
	})
	if !errors.Is(err, recorder.err) {
		t.Fatalf("error = %v, want the audit failure", err)
	}
}

// TestRoleAdministrationAuditsTheAuthenticatedActor covers attribution across
// every role administration use case: a role change with no actor names nobody
// accountable, which is the part of the trail an investigation depends on.
func TestRoleAdministrationAuditsTheAuthenticatedActor(t *testing.T) {
	t.Parallel()

	administer := map[string]func(*Service) error{
		"create org role": func(s *Service) error {
			_, err := s.CreateOrgRole(context.Background(), OrgRoleInput{
				OrgID: 1, ActorID: 3, Name: "auditor", ScopeType: scopeOrg, Permissions: []string{PermOrgRead},
			})
			return err
		},
		"create workspace role": func(s *Service) error {
			_, err := s.CreateWorkspaceRole(context.Background(), WorkspaceRoleInput{
				OrgID: 1, WorkspaceID: 10, ActorID: 3, Name: "reader",
				ScopeType: resourceWorkspace, Permissions: []string{PermWsRead},
			})
			return err
		},
		"update org role": func(s *Service) error {
			return s.UpdateOrgRole(context.Background(), UpdateOrgRoleInput{
				OrgID: 1, RoleID: 8, ActorID: 3, Name: "auditor", Permissions: []string{PermOrgRead},
			})
		},
		"update workspace role": func(s *Service) error {
			return s.UpdateWorkspaceRole(context.Background(), UpdateWorkspaceRoleInput{
				OrgID: 1, WorkspaceID: 10, RoleID: 7, ActorID: 3, Name: "reader",
				Permissions: []string{PermWsRead},
			})
		},
		"delete org role": func(s *Service) error {
			return s.DeleteOrgRole(context.Background(), DeleteOrgRoleInput{OrgID: 1, RoleID: 8, ActorID: 3})
		},
		"delete workspace role": func(s *Service) error {
			return s.DeleteWorkspaceRole(context.Background(), DeleteWorkspaceRoleInput{
				OrgID: 1, WorkspaceID: 10, RoleID: 7, ActorID: 3,
			})
		},
	}

	for name, run := range administer {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			service, store, _, recorder := newAuditedServiceFixture()
			workspaceID := int64(10)
			store.roles[7] = Role{ID: 7, OrgID: 1, WorkspaceID: &workspaceID, ScopeType: resourceWorkspace}
			store.roles[8] = Role{ID: 8, OrgID: 1, ScopeType: scopeOrg}

			if err := run(service); err != nil {
				t.Fatal(err)
			}
			if len(recorder.events) != 1 {
				t.Fatalf("emitted %d audit events, want 1", len(recorder.events))
			}
			if actor := recorder.events[0].AccountID; actor == nil || *actor != 3 {
				t.Fatalf("audit actor = %v, want the authenticated account 3", actor)
			}
		})
	}
}

// TestRevokeWorkspacePolicyAuditsTheAuthenticatedActor covers the revoke path,
// which removes access and so is the change most worth attributing.
func TestRevokeWorkspacePolicyAuditsTheAuthenticatedActor(t *testing.T) {
	t.Parallel()
	service, store, _, recorder := newAuditedServiceFixture()
	workspaceID := int64(10)
	store.roles[7] = Role{ID: 7, OrgID: 1, WorkspaceID: &workspaceID, ScopeType: resourceWorkspace}
	store.bindings[5] = RoleBinding{
		ID: 5, OrgID: 1, RoleID: 7, ResourceType: resourceWorkspace, ResourceID: workspaceID,
		SubjectType: SubjectTypeAccount, SubjectID: 2,
	}

	_, err := service.RevokeWorkspacePolicy(context.Background(), RevokeWorkspacePolicyInput{
		OrgID: 1, WorkspaceID: workspaceID, BindingID: 5, ActorID: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("emitted %d audit events, want 1", len(recorder.events))
	}
	if actor := recorder.events[0].AccountID; actor == nil || *actor != 3 {
		t.Fatalf("audit actor = %v, want the authenticated account 3", actor)
	}
}

// TestUnattributedRoleAdministrationOmitsAnActor covers an input with no
// authenticated account: the trail records no actor rather than account zero,
// so a missing actor cannot be mistaken for a real one.
func TestUnattributedRoleAdministrationOmitsAnActor(t *testing.T) {
	t.Parallel()
	service, _, _, recorder := newAuditedServiceFixture()

	_, err := service.CreateOrgRole(context.Background(), OrgRoleInput{
		OrgID: 1, Name: "auditor", ScopeType: scopeOrg, Permissions: []string{PermOrgRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorder.events[0].AccountID != nil {
		t.Fatalf("audit actor = %v, want no actor", recorder.events[0].AccountID)
	}
}
