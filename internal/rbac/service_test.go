package rbac_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/response"
)

const (
	testOrg    int64 = 1
	otherOrg   int64 = 2
	missingOrg int64 = 3

	actorID     int64 = 5
	readerID    int64 = 7
	nonMemberID int64 = 9
	foreignID   int64 = 10

	teamID        int64 = 30
	foreignTeamID int64 = 31

	ownerRoleID   int64 = 100
	adminRoleID   int64 = 101
	customRoleID  int64 = 103
	wsRoleID      int64 = 104
	privRoleID    int64 = 105
	foreignRoleID int64 = 106
	unboundRoleID int64 = 107

	customBindingID  int64 = 200
	ownerBindingID   int64 = 201
	wsBindingID      int64 = 202
	privBindingID    int64 = 203
	foreignBindingID int64 = 204
)

type bindCall struct {
	roleID, subjectID, resourceID, grantedBy int64
	subjectType, resourceType                string
}

type fakeStore struct {
	roles    map[int64]database.Role
	bindings map[int64]database.RoleBinding
	members  map[int64]bool
	nextRole int64

	workspaces   map[int64]database.Workspace
	environments map[int64]database.Environment
	connections  map[int64]database.Connection

	created   []string
	updated   []int64
	deleted   []int64
	bound     []bindCall
	unbound   []int64
	createErr error
	updateErr error
}

func newFakeStore() *fakeStore {
	wsID := testWS
	return &fakeStore{
		roles: map[int64]database.Role{
			ownerRoleID:   {ID: ownerRoleID, OrgID: testOrg, Name: access.BuiltinOrgOwnerRole, ScopeType: "org", IsBuiltin: true},
			adminRoleID:   {ID: adminRoleID, OrgID: testOrg, Name: access.BuiltinOrgAdminRole, ScopeType: "org", IsBuiltin: true},
			customRoleID:  {ID: customRoleID, OrgID: testOrg, Name: "Custom", ScopeType: "org", Permissions: []string{access.PermPolicyRead}},
			wsRoleID:      {ID: wsRoleID, OrgID: testOrg, Name: "Workspace", ScopeType: "workspace", WorkspaceID: &wsID},
			privRoleID:    {ID: privRoleID, OrgID: testOrg, Name: "Deleter", ScopeType: "org", Permissions: []string{access.PermOrgDelete}},
			foreignRoleID: {ID: foreignRoleID, OrgID: otherOrg, Name: "Foreign", ScopeType: "org"},
			unboundRoleID: {ID: unboundRoleID, OrgID: testOrg, Name: "Unbound", ScopeType: "org"},
		},
		bindings: map[int64]database.RoleBinding{
			customBindingID:  {ID: customBindingID, OrgID: testOrg, RoleID: customRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "org", ResourceID: testOrg},
			ownerBindingID:   {ID: ownerBindingID, OrgID: testOrg, RoleID: ownerRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: actorID, ResourceType: "org", ResourceID: testOrg},
			wsBindingID:      {ID: wsBindingID, OrgID: testOrg, RoleID: wsRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "workspace", ResourceID: wsID},
			privBindingID:    {ID: privBindingID, OrgID: testOrg, RoleID: privRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "org", ResourceID: testOrg},
			foreignBindingID: {ID: foreignBindingID, OrgID: otherOrg, RoleID: foreignRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: foreignID, ResourceType: "org", ResourceID: otherOrg},
		},
		members:      map[int64]bool{actorID: true, readerID: true},
		nextRole:     500,
		workspaces:   map[int64]database.Workspace{},
		environments: map[int64]database.Environment{},
		connections:  map[int64]database.Connection{},
	}
}

func (s *fakeStore) ListRolesPage(_ context.Context, params database.ListRolesParams) (response.Paginated[database.Role], error) {
	return response.Paginated[database.Role]{Items: []database.Role{{OrgID: params.OrgID}}}, nil
}

func (s *fakeStore) GetRole(_ context.Context, id, orgID int64) (database.Role, bool, error) {
	role, ok := s.roles[id]
	if !ok || role.OrgID != orgID {
		return database.Role{}, false, nil
	}
	return role, true, nil
}

func (s *fakeStore) CreateRole(_ context.Context, orgID int64, workspaceID *int64, name, _, scopeType string, permissions []string) (int64, error) {
	if s.createErr != nil {
		return 0, s.createErr
	}
	s.nextRole++
	s.roles[s.nextRole] = database.Role{ID: s.nextRole, OrgID: orgID, WorkspaceID: workspaceID, Name: name, ScopeType: scopeType, Permissions: permissions}
	s.created = append(s.created, name)
	return s.nextRole, nil
}

func (s *fakeStore) UpdateRole(_ context.Context, roleID, orgID int64, name, _ string, permissions []string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	role, ok := s.roles[roleID]
	if !ok || role.OrgID != orgID {
		return access.ErrRoleNotFound
	}
	if role.IsBuiltin {
		return access.ErrBuiltinRole
	}
	role.Name, role.Permissions = name, permissions
	s.roles[roleID] = role
	s.updated = append(s.updated, roleID)
	return nil
}

func (s *fakeStore) DeleteRole(_ context.Context, roleID, orgID int64) error {
	role, ok := s.roles[roleID]
	if !ok || role.OrgID != orgID {
		return access.ErrRoleNotFound
	}
	if role.IsBuiltin {
		return access.ErrBuiltinRole
	}
	count := 0
	for _, b := range s.bindings {
		if b.RoleID == roleID {
			count++
		}
	}
	if count > 0 {
		return access.RoleInUseError{BindingCount: count}
	}
	delete(s.roles, roleID)
	s.deleted = append(s.deleted, roleID)
	return nil
}

func (s *fakeStore) ListOrgPoliciesPage(_ context.Context, params database.ListOrgPoliciesParams) (response.Paginated[database.WorkspacePolicyListItem], error) {
	return response.Paginated[database.WorkspacePolicyListItem]{Items: []database.WorkspacePolicyListItem{{ResourceID: params.OrgID}}}, nil
}

func (s *fakeStore) GetRoleBinding(_ context.Context, id, orgID int64) (database.RoleBinding, bool, error) {
	b, ok := s.bindings[id]
	if !ok || b.OrgID != orgID {
		return database.RoleBinding{}, false, nil
	}
	return b, true, nil
}

func (s *fakeStore) GetPolicyBindingItem(_ context.Context, orgID int64, b database.RoleBinding) (database.WorkspacePolicyListItem, error) {
	return database.WorkspacePolicyListItem{BindingID: b.ID, ResourceID: orgID}, nil
}

func (s *fakeStore) CountRoleBindings(_ context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error) {
	count := 0
	for _, b := range s.bindings {
		if b.OrgID == orgID && b.RoleID == roleID && b.ResourceType == resourceType && b.ResourceID == resourceID {
			count++
		}
	}
	return count, nil
}

func (s *fakeStore) BindRole(_ context.Context, _, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error {
	s.bound = append(s.bound, bindCall{roleID: roleID, subjectType: subjectType, subjectID: subjectID, resourceType: resourceType, resourceID: resourceID, grantedBy: grantedBy})
	return nil
}

func (s *fakeStore) UnbindRole(_ context.Context, bindingID, _ int64) error {
	delete(s.bindings, bindingID)
	s.unbound = append(s.unbound, bindingID)
	return nil
}

func (s *fakeStore) GetAccount(_ context.Context, id int64) (database.Account, bool, error) {
	switch id {
	case actorID, readerID, nonMemberID, foreignID:
		return database.Account{ID: id}, true, nil
	}
	return database.Account{}, false, nil
}

func (s *fakeStore) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return orgID == testOrg && s.members[accountID], nil
}

func (s *fakeStore) GetTeamByID(_ context.Context, id int64) (database.Team, bool, error) {
	switch id {
	case teamID:
		return database.Team{ID: id, OrgID: testOrg}, true, nil
	case foreignTeamID:
		return database.Team{ID: id, OrgID: otherOrg}, true, nil
	}
	return database.Team{}, false, nil
}

type fakeDirectory struct{ store *fakeStore }

func (d fakeDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == testOrg, nil
}

func (d fakeDirectory) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	// missingOrg reports membership so the Requirer reaches its org lookup.
	if orgID == missingOrg {
		return true, nil
	}
	return d.store.IsOrgMember(ctx, orgID, accountID)
}

func (fakeDirectory) IsEffectiveWorkspaceMember(context.Context, int64, int64, int64) (bool, error) {
	return false, nil
}

func (d fakeDirectory) GetWorkspace(_ context.Context, id int64) (access.Workspace, bool, error) {
	ws, ok := d.store.workspaces[id]
	if !ok {
		return access.Workspace{}, false, nil
	}
	return access.Workspace{ID: ws.ID, OrgID: ws.OrgID, OwnerType: ws.OwnerType}, true, nil
}

func (fakeDirectory) GetConnection(context.Context, int64) (access.Connection, bool, error) {
	return access.Connection{}, false, nil
}

// fakePolicy grants each account exactly the organization permissions listed
// for it, and separately the permissions listed for it on workspace testWS.
type fakePolicy struct {
	grants   map[int64]map[string]bool
	wsGrants map[int64]map[string]bool
}

func (p *fakePolicy) Can(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if ownerType != "org" {
		return false
	}
	switch {
	case resourceType == "org" && resourceID == orgID:
		return p.grants[accountID][permission]
	case resourceType == "workspace" && resourceID == testWS:
		return p.wsGrants[accountID][permission]
	}
	return false
}

// EffectivePermissions echoes the resolved resource so tests can see what
// the service asked about.
func (p *fakePolicy) EffectivePermissions(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	return []string{fmt.Sprintf("%d/%d/%s/%s/%d", accountID, orgID, ownerType, resourceType, resourceID)}, nil
}

func (p *fakePolicy) setWorkspace(accountID int64, permissions ...string) {
	granted := map[string]bool{}
	for _, permission := range permissions {
		granted[permission] = true
	}
	p.wsGrants[accountID] = granted
}

func (p *fakePolicy) set(accountID int64, permissions ...string) {
	granted := map[string]bool{}
	for _, permission := range permissions {
		granted[permission] = true
	}
	p.grants[accountID] = granted
}

type harness struct {
	svc      *rbac.Service
	store    *fakeStore
	policy   *fakePolicy
	events   []audit.Event
	auditErr error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{store: newFakeStore(), policy: &fakePolicy{grants: map[int64]map[string]bool{}, wsGrants: map[int64]map[string]bool{}}}
	h.policy.set(actorID, access.AllPermissions()...)
	h.policy.setWorkspace(actorID, access.AllPermissions()...)
	h.svc = rbac.New(rbac.Deps{
		Store:    h.store,
		Requirer: access.NewRequirer(fakeDirectory{store: h.store}, h.policy),
		Policy:   h.policy,
		Audit: audit.WriterFunc(func(_ context.Context, e audit.Event) error {
			if h.auditErr != nil {
				return h.auditErr
			}
			h.events = append(h.events, e)
			return nil
		}),
	})
	return h
}

func (h *harness) actions() []string {
	out := make([]string, len(h.events))
	for i, e := range h.events {
		out[i] = e.Action
	}
	return out
}

func account(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

func allBut(excluded string) []string {
	out := []string{}
	for _, permission := range access.AllPermissions() {
		if permission != excluded {
			out = append(out, permission)
		}
	}
	return out
}

type methodCase struct {
	name       string
	permission string
	call       func(h *harness, p access.Principal, org int64) error
}

func methodCases() []methodCase {
	ctx := context.Background()
	return []methodCase{
		{"ListRoles", access.PermPolicyRead, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListRoles(ctx, p, org, rbac.ListRolesInput{}))
		}},
		{"GetRole", access.PermPolicyRead, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.GetRole(ctx, p, org, customRoleID))
		}},
		{"CreateRole", access.PermPolicyModify, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.CreateRole(ctx, p, org, rbac.CreateRoleInput{Name: "New", Permissions: []string{access.PermOrgRead}}))
		}},
		{"UpdateRole", access.PermPolicyModify, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.UpdateRole(ctx, p, org, customRoleID, rbac.UpdateRoleInput{Name: "Renamed"}))
		}},
		{"DeleteRole", access.PermPolicyModify, func(h *harness, p access.Principal, org int64) error {
			return h.svc.DeleteRole(ctx, p, org, unboundRoleID)
		}},
		{"ListPolicies", access.PermPolicyRead, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListPolicies(ctx, p, org, database.ListOrgPoliciesParams{}))
		}},
		{"GetPolicy", access.PermPolicyRead, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.GetPolicy(ctx, p, org, customBindingID))
		}},
		{"GrantPolicy", access.PermPolicyModify, func(h *harness, p access.Principal, org int64) error {
			return h.svc.GrantPolicy(ctx, p, org, rbac.GrantPolicyInput{RoleID: customRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID})
		}},
		{"RevokePolicy", access.PermPolicyModify, func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.RevokePolicy(ctx, p, org, customBindingID))
		}},
		{"ListPermissions", "", func(h *harness, p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListPermissions(ctx, p, org))
		}},
	}
}

func TestDenyMatrix(t *testing.T) {
	for _, tc := range methodCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			accesstest.AssertForbidden(t, tc.call(h, account(nonMemberID), testOrg), "non-member")
			accesstest.AssertNotFound(t, tc.call(h, account(actorID), missingOrg), "missing org")
			accesstest.AssertForbidden(t, tc.call(h, account(actorID), otherOrg), "foreign org")

			if tc.permission != "" {
				h.policy.set(readerID, allBut(tc.permission)...)
				accesstest.AssertForbidden(t, tc.call(h, account(readerID), testOrg), "member without "+tc.permission)
				h.policy.set(readerID, tc.permission)
			} else {
				h.policy.set(readerID)
			}
			if err := tc.call(h, account(readerID), testOrg); err != nil {
				t.Fatalf("member holding only %q: %v", tc.permission, err)
			}
		})
	}
}

func TestBuiltinRolesAreImmutable(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, err := h.svc.UpdateRole(ctx, account(actorID), testOrg, adminRoleID, rbac.UpdateRoleInput{Name: "Renamed"})
	if !errors.Is(err, rbac.ErrBuiltinRole) {
		t.Fatalf("UpdateRole builtin error = %v, want ErrBuiltinRole", err)
	}
	if err := h.svc.DeleteRole(ctx, account(actorID), testOrg, adminRoleID); !errors.Is(err, rbac.ErrBuiltinRole) {
		t.Fatalf("DeleteRole builtin error = %v, want ErrBuiltinRole", err)
	}
	if len(h.store.updated) != 0 || len(h.store.deleted) != 0 || len(h.events) != 0 {
		t.Fatalf("builtin role changed: updated=%v deleted=%v events=%v", h.store.updated, h.store.deleted, h.actions())
	}
}

func TestCreateRoleValidatesScopeBeforeWriting(t *testing.T) {
	ws := int64(9)
	cases := map[string]struct {
		in    rbac.CreateRoleInput
		field string
		msg   string
	}{
		"name":        {rbac.CreateRoleInput{Permissions: []string{access.PermOrgRead}}, "name", "Name is required."},
		"scope":       {rbac.CreateRoleInput{Name: "R", ScopeType: "workspace"}, "scope_type", "Organization roles must have scope_type=org."},
		"workspace":   {rbac.CreateRoleInput{Name: "R", WorkspaceID: &ws}, "workspace_id", "Organization roles cannot set workspace_id."},
		"unknown":     {rbac.CreateRoleInput{Name: "R", Permissions: []string{"nope:nope"}}, "permissions", "Permission nope:nope is not valid for scope org."},
		"first error": {rbac.CreateRoleInput{Name: "R", Permissions: []string{"a:a", "b:b"}}, "permissions", "Permission a:a is not valid for scope org."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			_, err := h.svc.CreateRole(context.Background(), account(actorID), testOrg, tc.in)
			var validation *rbac.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want ValidationError", err)
			}
			if got := validation.Fields[tc.field]; got != tc.msg {
				t.Fatalf("field %s = %q, want %q", tc.field, got, tc.msg)
			}
			if len(h.store.created) != 0 {
				t.Fatalf("store called despite invalid input: %v", h.store.created)
			}
		})
	}
}

func TestCreateRoleDefaultsToOrgScopeAndAudits(t *testing.T) {
	h := newHarness(t)
	role, err := h.svc.CreateRole(context.Background(), account(actorID), testOrg, rbac.CreateRoleInput{
		Name: "Readers", Permissions: []string{access.PermOrgRead, access.PermPolicyRead},
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if role.ScopeType != "org" || role.WorkspaceID != nil || role.Name != "Readers" {
		t.Fatalf("role = %+v", role)
	}
	if len(h.events) != 1 || h.events[0].Action != rbac.ActionRoleCreated {
		t.Fatalf("events = %v", h.actions())
	}
	e := h.events[0]
	if e.Resource != "role" || e.Metadata["permissions"] != access.PermOrgRead+","+access.PermPolicyRead {
		t.Fatalf("event = %+v", e)
	}
}

func TestStorePermissionErrorsBecomeValidation(t *testing.T) {
	for _, storeErr := range []error{access.ErrInvalidScopePermission, access.ErrUnknownPermission} {
		h := newHarness(t)
		h.store.createErr = storeErr
		h.store.updateErr = storeErr
		const want = "Permissions include a permission that is not valid for this scope."
		_, err := h.svc.CreateRole(context.Background(), account(actorID), testOrg, rbac.CreateRoleInput{Name: "R"})
		var validation *rbac.ValidationError
		if !errors.As(err, &validation) || validation.Fields["permissions"] != want {
			t.Fatalf("CreateRole %v: error = %v", storeErr, err)
		}
		_, err = h.svc.UpdateRole(context.Background(), account(actorID), testOrg, customRoleID, rbac.UpdateRoleInput{Name: "R"})
		if !errors.As(err, &validation) || validation.Fields["permissions"] != want {
			t.Fatalf("UpdateRole %v: error = %v", storeErr, err)
		}
	}
}

func TestUpdateRoleValidation(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.UpdateRole(context.Background(), account(actorID), testOrg, customRoleID, rbac.UpdateRoleInput{})
	var validation *rbac.ValidationError
	if !errors.As(err, &validation) || validation.Fields["name"] != "Name is required." {
		t.Fatalf("error = %v", err)
	}
	h.store.updateErr = errors.New("UNIQUE constraint failed: roles.org_id, roles.name")
	_, err = h.svc.UpdateRole(context.Background(), account(actorID), testOrg, customRoleID, rbac.UpdateRoleInput{Name: "Dup"})
	if !errors.As(err, &validation) || validation.Fields["name"] != "A role with this name already exists in this organization." {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestDeleteRoleInUseIsRefused(t *testing.T) {
	h := newHarness(t)
	err := h.svc.DeleteRole(context.Background(), account(actorID), testOrg, customRoleID)
	if !errors.Is(err, rbac.ErrRoleInUse) {
		t.Fatalf("error = %v, want ErrRoleInUse", err)
	}
	var inUse access.RoleInUseError
	if !errors.As(err, &inUse) || inUse.BindingCount != 1 {
		t.Fatalf("error = %#v, want binding count 1", err)
	}
	if len(h.store.deleted) != 0 || len(h.events) != 0 {
		t.Fatalf("role deleted despite bindings")
	}
}

func TestDeleteRoleAudits(t *testing.T) {
	h := newHarness(t)
	if err := h.svc.DeleteRole(context.Background(), account(actorID), testOrg, unboundRoleID); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if !slices.Equal(h.actions(), []string{rbac.ActionRoleDeleted}) || h.events[0].ResourceID != "107" {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestForeignRolesAndBindingsAreNotFound(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := account(actorID)
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetRole(ctx, p, testOrg, foreignRoleID)), "GetRole foreign")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.UpdateRole(ctx, p, testOrg, foreignRoleID, rbac.UpdateRoleInput{Name: "X"})), "UpdateRole foreign")
	accesstest.AssertNotFound(t, h.svc.DeleteRole(ctx, p, testOrg, foreignRoleID), "DeleteRole foreign")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetPolicy(ctx, p, testOrg, foreignBindingID)), "GetPolicy foreign")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetPolicy(ctx, p, testOrg, wsBindingID)), "GetPolicy workspace binding")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.RevokePolicy(ctx, p, testOrg, foreignBindingID)), "RevokePolicy foreign")
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.RevokePolicy(ctx, p, testOrg, wsBindingID)), "RevokePolicy workspace binding")
	err := h.svc.GrantPolicy(ctx, p, testOrg, rbac.GrantPolicyInput{RoleID: foreignRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID})
	accesstest.AssertNotFound(t, err, "GrantPolicy foreign role")
	if len(h.store.bound) != 0 || len(h.store.unbound) != 0 {
		t.Fatalf("foreign records changed")
	}
}

func TestGrantPolicySubjectMustExistInOrg(t *testing.T) {
	cases := map[string]struct {
		subjectType string
		subjectID   int64
		ok          bool
	}{
		"member account":         {access.SubjectTypeAccount, readerID, true},
		"non-member account":     {access.SubjectTypeAccount, nonMemberID, false},
		"unknown account":        {access.SubjectTypeAccount, 999, false},
		"org team":               {access.SubjectTypeTeam, teamID, true},
		"foreign team":           {access.SubjectTypeTeam, foreignTeamID, false},
		"unknown team":           {access.SubjectTypeTeam, 999, false},
		"org members":            {access.SubjectTypeOrgMembers, testOrg, true},
		"foreign org members":    {access.SubjectTypeOrgMembers, otherOrg, false},
		"foreign member account": {access.SubjectTypeAccount, foreignID, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			err := h.svc.GrantPolicy(context.Background(), account(actorID), testOrg, rbac.GrantPolicyInput{RoleID: customRoleID, SubjectType: tc.subjectType, SubjectID: tc.subjectID})
			if tc.ok {
				if err != nil {
					t.Fatalf("GrantPolicy: %v", err)
				}
				want := bindCall{roleID: customRoleID, subjectType: tc.subjectType, subjectID: tc.subjectID, resourceType: "org", resourceID: testOrg, grantedBy: actorID}
				if !slices.Equal(h.store.bound, []bindCall{want}) {
					t.Fatalf("bound = %+v", h.store.bound)
				}
				return
			}
			accesstest.AssertNotFound(t, err, "GrantPolicy")
			if len(h.store.bound) != 0 {
				t.Fatalf("bound = %+v", h.store.bound)
			}
		})
	}
}

func TestGrantPolicyValidation(t *testing.T) {
	h := newHarness(t)
	err := h.svc.GrantPolicy(context.Background(), account(actorID), testOrg, rbac.GrantPolicyInput{SubjectType: "role"})
	var validation *rbac.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v", err)
	}
	want := map[string]string{
		"role_id":      "Role is required.",
		"subject_type": "Subject type must be account, team, or org_members.",
		"subject_id":   "Subject is required.",
	}
	for field, msg := range want {
		if validation.Fields[field] != msg {
			t.Fatalf("field %s = %q, want %q", field, validation.Fields[field], msg)
		}
	}

	err = h.svc.GrantPolicy(context.Background(), account(actorID), testOrg, rbac.GrantPolicyInput{RoleID: wsRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID})
	if !errors.As(err, &validation) || validation.Fields["role_id"] != "Role scope must match resource type." {
		t.Fatalf("workspace role error = %v", err)
	}
	if len(h.store.bound) != 0 {
		t.Fatalf("bound = %+v", h.store.bound)
	}
}

func TestPrivilegedGrantRequiresActorToHoldPermissions(t *testing.T) {
	for _, roleID := range []int64{ownerRoleID, privRoleID} {
		h := newHarness(t)
		h.policy.set(readerID, access.PermPolicyModify, access.PermOrgTransferOwnership)
		err := h.svc.GrantPolicy(context.Background(), account(readerID), testOrg, rbac.GrantPolicyInput{RoleID: roleID, SubjectType: access.SubjectTypeAccount, SubjectID: actorID})
		var refusal *rbac.RefusalError
		if !errors.Is(err, rbac.ErrPrivilegedPolicy) || !errors.As(err, &refusal) || refusal.Role.ID != roleID {
			t.Fatalf("role %d: error = %v, want privileged refusal", roleID, err)
		}
		if len(h.store.bound) != 0 || len(h.events) != 0 {
			t.Fatalf("role %d bound despite refusal", roleID)
		}

		h.policy.set(readerID, access.PermPolicyModify, access.PermOrgDelete, access.PermOrgTransferOwnership)
		if err := h.svc.GrantPolicy(context.Background(), account(readerID), testOrg, rbac.GrantPolicyInput{RoleID: roleID, SubjectType: access.SubjectTypeAccount, SubjectID: actorID}); err != nil {
			t.Fatalf("role %d with privileges: %v", roleID, err)
		}
	}
}

func TestPrivilegedRevokeRequiresActorToHoldPermissions(t *testing.T) {
	h := newHarness(t)
	h.policy.set(readerID, access.PermPolicyModify)
	_, err := h.svc.RevokePolicy(context.Background(), account(readerID), testOrg, privBindingID)
	var refusal *rbac.RefusalError
	if !errors.Is(err, rbac.ErrPrivilegedPolicy) || !errors.As(err, &refusal) || refusal.Binding.ID != privBindingID || refusal.Role.ID != privRoleID {
		t.Fatalf("error = %v, want privileged refusal", err)
	}
	if len(h.store.unbound) != 0 {
		t.Fatalf("unbound despite refusal")
	}
	h.policy.set(readerID, access.PermPolicyModify, access.PermOrgDelete)
	if _, err := h.svc.RevokePolicy(context.Background(), account(readerID), testOrg, privBindingID); err != nil {
		t.Fatalf("RevokePolicy with privilege: %v", err)
	}
}

func TestUpdateRoleAddingPrivilegedPermissionRequiresActorToHoldIt(t *testing.T) {
	for _, permission := range []string{access.PermOrgDelete, access.PermOrgTransferOwnership} {
		t.Run(permission, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			h.policy.set(readerID, access.PermPolicyModify)
			in := rbac.UpdateRoleInput{Name: "Custom", Permissions: []string{access.PermPolicyRead, permission}}
			_, err := h.svc.UpdateRole(ctx, account(readerID), testOrg, customRoleID, in)
			var refusal *rbac.RefusalError
			if !errors.Is(err, rbac.ErrPrivilegedPolicy) || !errors.As(err, &refusal) || refusal.Role.ID != customRoleID {
				t.Fatalf("error = %v, want privileged refusal", err)
			}
			if len(h.store.updated) != 0 || len(h.events) != 0 ||
				!slices.Equal(h.store.roles[customRoleID].Permissions, []string{access.PermPolicyRead}) {
				t.Fatalf("role changed despite refusal: updated=%v perms=%v events=%v", h.store.updated, h.store.roles[customRoleID].Permissions, h.actions())
			}

			h.policy.set(readerID, access.PermPolicyModify, permission)
			if _, err := h.svc.UpdateRole(ctx, account(readerID), testOrg, customRoleID, in); err != nil {
				t.Fatalf("UpdateRole holding %s: %v", permission, err)
			}
			if !slices.Equal(h.store.roles[customRoleID].Permissions, in.Permissions) {
				t.Fatalf("permissions = %v", h.store.roles[customRoleID].Permissions)
			}
		})
	}
}

func TestUpdateRoleKeepingPrivilegedPermissionIsNotRechecked(t *testing.T) {
	h := newHarness(t)
	h.policy.set(readerID, access.PermPolicyModify)
	in := rbac.UpdateRoleInput{Name: "Renamed", Permissions: []string{access.PermOrgDelete, access.PermOrgRead}}
	if _, err := h.svc.UpdateRole(context.Background(), account(readerID), testOrg, privRoleID, in); err != nil {
		t.Fatalf("UpdateRole keeping org:delete: %v", err)
	}
	if !slices.Equal(h.store.updated, []int64{privRoleID}) {
		t.Fatalf("updated = %v", h.store.updated)
	}
}

func TestUpdateRoleRemovingPrivilegedPermissionRequiresActorToHoldIt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.policy.set(readerID, access.PermPolicyModify, access.PermOrgTransferOwnership)
	in := rbac.UpdateRoleInput{Name: "Deleter", Permissions: []string{access.PermOrgRead}}
	_, err := h.svc.UpdateRole(ctx, account(readerID), testOrg, privRoleID, in)
	if !errors.Is(err, rbac.ErrPrivilegedPolicy) {
		t.Fatalf("error = %v, want ErrPrivilegedPolicy", err)
	}
	if len(h.store.updated) != 0 || !slices.Equal(h.store.roles[privRoleID].Permissions, []string{access.PermOrgDelete}) {
		t.Fatalf("role changed despite refusal: %v", h.store.roles[privRoleID].Permissions)
	}

	h.policy.set(readerID, access.PermPolicyModify, access.PermOrgDelete)
	if _, err := h.svc.UpdateRole(ctx, account(readerID), testOrg, privRoleID, in); err != nil {
		t.Fatalf("UpdateRole holding org:delete: %v", err)
	}
}

func TestUpdateRoleValidatesScopeBeforePrivilegeCheck(t *testing.T) {
	h := newHarness(t)
	h.policy.set(readerID, access.PermPolicyModify)
	_, err := h.svc.UpdateRole(context.Background(), account(readerID), testOrg, customRoleID, rbac.UpdateRoleInput{Name: "Custom", Permissions: []string{access.PermOrgDelete, "bogus:perm"}})
	var validation *rbac.ValidationError
	if !errors.As(err, &validation) || validation.Fields["permissions"] != "Permissions include a permission that is not valid for this scope." {
		t.Fatalf("error = %v, want permissions validation", err)
	}
	if len(h.store.updated) != 0 {
		t.Fatalf("updated = %v", h.store.updated)
	}
}

func TestUpdateRoleAudits(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.UpdateRole(context.Background(), account(actorID), testOrg, customRoleID, rbac.UpdateRoleInput{Name: "Renamed"}); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if !slices.Equal(h.actions(), []string{rbac.ActionRoleUpdated}) || h.events[0].Resource != "role" || h.events[0].ResourceID != "103" || len(h.events[0].Metadata) != 0 {
		t.Fatalf("events = %+v", h.events)
	}

	h.auditErr = errors.New("audit down")
	if _, err := h.svc.UpdateRole(context.Background(), account(actorID), testOrg, customRoleID, rbac.UpdateRoleInput{Name: "Again"}); err != nil {
		t.Fatalf("UpdateRole with failing audit: %v", err)
	}
}

func TestRevokeLastOwnerIsRefused(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, err := h.svc.RevokePolicy(ctx, account(actorID), testOrg, ownerBindingID)
	var refusal *rbac.RefusalError
	if !errors.Is(err, rbac.ErrLastOwner) || !errors.As(err, &refusal) || refusal.Binding.SubjectID != actorID {
		t.Fatalf("error = %v, want ErrLastOwner", err)
	}
	if len(h.store.unbound) != 0 || len(h.events) != 0 {
		t.Fatalf("last owner revoked")
	}

	h.store.bindings[300] = database.RoleBinding{ID: 300, OrgID: testOrg, RoleID: ownerRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "org", ResourceID: testOrg}
	if _, err := h.svc.RevokePolicy(ctx, account(actorID), testOrg, ownerBindingID); err != nil {
		t.Fatalf("revoke with a second owner: %v", err)
	}
}

func TestPolicyAuditEvents(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.GrantPolicy(ctx, account(actorID), testOrg, rbac.GrantPolicyInput{RoleID: customRoleID, SubjectType: access.SubjectTypeTeam, SubjectID: teamID}); err != nil {
		t.Fatalf("GrantPolicy: %v", err)
	}
	b, err := h.svc.RevokePolicy(ctx, account(actorID), testOrg, customBindingID)
	if err != nil {
		t.Fatalf("RevokePolicy: %v", err)
	}
	if b.ID != customBindingID || b.SubjectID != readerID {
		t.Fatalf("revoked binding = %+v", b)
	}
	if !slices.Equal(h.actions(), []string{rbac.ActionPolicyGranted, rbac.ActionPolicyRevoked}) {
		t.Fatalf("events = %v", h.actions())
	}
	granted := h.events[0]
	if granted.Resource != "role" || granted.ResourceID != "103" || granted.Metadata["subject_type"] != access.SubjectTypeTeam ||
		granted.Metadata["subject_id"] != "30" || granted.Metadata["resource_type"] != "org" || granted.Metadata["resource_id"] != "1" {
		t.Fatalf("granted = %+v", granted)
	}
	revoked := h.events[1]
	if revoked.Resource != "role_binding" || revoked.ResourceID != "200" || revoked.Metadata["role_id"] != "103" ||
		revoked.Metadata["subject_id"] != "7" || revoked.Actor.SubjectID == nil || *revoked.Actor.SubjectID != actorID {
		t.Fatalf("revoked = %+v", revoked)
	}
}

func TestAuditFailureDoesNotFailChange(t *testing.T) {
	h := newHarness(t)
	h.auditErr = errors.New("audit down")
	if err := h.svc.GrantPolicy(context.Background(), account(actorID), testOrg, rbac.GrantPolicyInput{RoleID: customRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID}); err != nil {
		t.Fatalf("GrantPolicy with failing audit: %v", err)
	}
	if len(h.store.bound) != 1 {
		t.Fatalf("bound = %+v", h.store.bound)
	}
}

func TestListRolesValidatesFilters(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	var validation *rbac.ValidationError
	_, err := h.svc.ListRoles(ctx, account(actorID), testOrg, rbac.ListRolesInput{Builtin: "yes"})
	if !errors.As(err, &validation) || validation.Fields["builtin"] != "Built-in flag must be true or false." {
		t.Fatalf("builtin error = %v", err)
	}
	_, err = h.svc.ListRoles(ctx, account(actorID), testOrg, rbac.ListRolesInput{Scope: "team"})
	if !errors.As(err, &validation) || validation.Fields["scope"] != "Scope must be all, org, or workspace." {
		t.Fatalf("scope error = %v", err)
	}
}

func TestListPermissionsReturnsCatalog(t *testing.T) {
	h := newHarness(t)
	catalog, err := h.svc.ListPermissions(context.Background(), account(actorID), testOrg)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if !slices.Equal(catalog.Permissions, access.AllPermissions()) || len(catalog.ScopeMap) == 0 || len(catalog.ResourceDetails) == 0 {
		t.Fatalf("catalog = %+v", catalog)
	}
}

func TestPrivilegedPermissions(t *testing.T) {
	cases := []struct {
		name string
		role database.Role
		want []string
	}{
		{"builtin owner", database.Role{Name: access.BuiltinOrgOwnerRole, IsBuiltin: true, ScopeType: "org"}, []string{access.PermOrgDelete, access.PermOrgTransferOwnership}},
		{"custom transfer", database.Role{Name: "Transfer", ScopeType: "org", Permissions: []string{access.PermOrgRead, access.PermOrgTransferOwnership}}, []string{access.PermOrgTransferOwnership}},
		{"builtin admin", database.Role{Name: access.BuiltinOrgAdminRole, IsBuiltin: true, ScopeType: "org"}, []string{}},
		{"custom named owner", database.Role{Name: access.BuiltinOrgOwnerRole, ScopeType: "org"}, []string{}},
	}
	for _, tc := range cases {
		if got := rbac.PrivilegedPermissions(tc.role); !slices.Equal(got, tc.want) {
			t.Errorf("%s: PrivilegedPermissions = %v, want %v", tc.name, got, tc.want)
		}
	}
}
