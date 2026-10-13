package rbac_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/response"
)

const (
	testWS      int64 = 9
	siblingWS   int64 = 11
	foreignWS   int64 = 12
	personalWS  int64 = 13
	missingWS   int64 = 14
	testEnv     int64 = 40
	siblingEnv  int64 = 41
	foreignEnv  int64 = 42
	testConn    int64 = 50
	siblingConn int64 = 51
	foreignConn int64 = 52

	envRoleID         int64 = 108
	connRoleID        int64 = 109
	siblingWSRoleID   int64 = 110
	wsBuiltinRoleID   int64 = 111
	wsPrivRoleID      int64 = 112
	wsUnboundRoleID   int64 = 113
	envBindingID      int64 = 205
	siblingWSBinding  int64 = 206
	siblingEnvBinding int64 = 207
	wsPrivBindingID   int64 = 208
)

func (s *fakeStore) ListWorkspaceRolesPage(_ context.Context, params database.ListRolesParams) (response.Paginated[database.Role], error) {
	return response.Paginated[database.Role]{Items: []database.Role{{OrgID: params.OrgID, WorkspaceID: params.WorkspaceID}}}, nil
}

func (s *fakeStore) ListWorkspacePoliciesPage(_ context.Context, params database.ListWorkspacePoliciesParams) (response.Paginated[database.WorkspacePolicyListItem], error) {
	return response.Paginated[database.WorkspacePolicyListItem]{Items: []database.WorkspacePolicyListItem{{ResourceID: params.WorkspaceID}}}, nil
}

func (s *fakeStore) GetWorkspace(_ context.Context, id int64) (database.Workspace, bool, error) {
	ws, ok := s.workspaces[id]
	return ws, ok, nil
}

func (s *fakeStore) GetEnvironment(_ context.Context, id int64) (database.Environment, bool, error) {
	env, ok := s.environments[id]
	return env, ok, nil
}

func (s *fakeStore) GetConnection(_ context.Context, id int64) (database.Connection, bool, error) {
	conn, ok := s.connections[id]
	return conn, ok, nil
}

func orgPtr(id int64) *int64 { return &id }

// newWorkspaceHarness adds workspaces testWS and siblingWS in testOrg,
// foreignWS in otherOrg, and a personal workspace, each with an environment
// and a connection, plus workspace roles and bindings.
func newWorkspaceHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	s := h.store
	for id, org := range map[int64]*int64{testWS: orgPtr(testOrg), siblingWS: orgPtr(testOrg), foreignWS: orgPtr(otherOrg)} {
		s.workspaces[id] = database.Workspace{ID: id, OrgID: org, OwnerType: "org"}
	}
	s.workspaces[personalWS] = database.Workspace{ID: personalWS, OwnerType: "account", OwnerID: actorID}
	s.environments[testEnv] = database.Environment{ID: testEnv, WorkspaceID: testWS}
	s.environments[siblingEnv] = database.Environment{ID: siblingEnv, WorkspaceID: siblingWS}
	s.environments[foreignEnv] = database.Environment{ID: foreignEnv, WorkspaceID: foreignWS}
	s.connections[testConn] = database.Connection{ID: testConn, WorkspaceID: testWS, EnvironmentID: testEnv}
	s.connections[siblingConn] = database.Connection{ID: siblingConn, WorkspaceID: siblingWS, EnvironmentID: siblingEnv}
	s.connections[foreignConn] = database.Connection{ID: foreignConn, WorkspaceID: foreignWS, EnvironmentID: foreignEnv}

	ws, sibling := testWS, siblingWS
	s.roles[envRoleID] = database.Role{ID: envRoleID, OrgID: testOrg, WorkspaceID: &ws, Name: "Env", ScopeType: "environment"}
	s.roles[connRoleID] = database.Role{ID: connRoleID, OrgID: testOrg, Name: "Conn", ScopeType: "connection"}
	s.roles[siblingWSRoleID] = database.Role{ID: siblingWSRoleID, OrgID: testOrg, WorkspaceID: &sibling, Name: "Sibling", ScopeType: "workspace"}
	s.roles[wsBuiltinRoleID] = database.Role{ID: wsBuiltinRoleID, OrgID: testOrg, WorkspaceID: &ws, Name: access.BuiltinWorkspaceAdminRole, ScopeType: "workspace", IsBuiltin: true}
	// The scope rules keep org permissions out of workspace roles; this
	// fixture stands in for such a role to exercise the privileged check.
	s.roles[wsPrivRoleID] = database.Role{ID: wsPrivRoleID, OrgID: testOrg, WorkspaceID: &ws, Name: "WsDeleter", ScopeType: "workspace", Permissions: []string{access.PermOrgDelete}}
	s.roles[wsUnboundRoleID] = database.Role{ID: wsUnboundRoleID, OrgID: testOrg, WorkspaceID: &ws, Name: "WsUnbound", ScopeType: "workspace"}

	s.bindings[envBindingID] = database.RoleBinding{ID: envBindingID, OrgID: testOrg, RoleID: envRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "environment", ResourceID: testEnv}
	s.bindings[siblingWSBinding] = database.RoleBinding{ID: siblingWSBinding, OrgID: testOrg, RoleID: siblingWSRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "workspace", ResourceID: siblingWS}
	s.bindings[siblingEnvBinding] = database.RoleBinding{ID: siblingEnvBinding, OrgID: testOrg, RoleID: envRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "environment", ResourceID: siblingEnv}
	s.bindings[wsPrivBindingID] = database.RoleBinding{ID: wsPrivBindingID, OrgID: testOrg, RoleID: wsPrivRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: "workspace", ResourceID: testWS}
	return h
}

type workspaceCase struct {
	name       string
	permission string
	call       func(h *harness, p access.Principal, org, ws int64) error
}

func workspaceCases() []workspaceCase {
	ctx := context.Background()
	return []workspaceCase{
		{"ListWorkspaceRoles", access.PermPolicyRead, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.ListWorkspaceRoles(ctx, p, org, ws, rbac.ListWorkspaceRolesInput{}))
		}},
		{"GetWorkspaceRole", access.PermPolicyRead, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.GetWorkspaceRole(ctx, p, org, ws, wsRoleID))
		}},
		{"CreateWorkspaceRole", access.PermPolicyModify, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.CreateWorkspaceRole(ctx, p, org, ws, rbac.CreateWorkspaceRoleInput{Name: "New", Permissions: []string{access.PermWsRead}}))
		}},
		{"UpdateWorkspaceRole", access.PermPolicyModify, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.UpdateWorkspaceRole(ctx, p, org, ws, wsRoleID, rbac.UpdateRoleInput{Name: "Renamed"}))
		}},
		{"DeleteWorkspaceRole", access.PermPolicyModify, func(h *harness, p access.Principal, org, ws int64) error {
			return h.svc.DeleteWorkspaceRole(ctx, p, org, ws, wsUnboundRoleID)
		}},
		{"ListWorkspacePolicies", access.PermPolicyRead, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.ListWorkspacePolicies(ctx, p, org, ws, database.ListWorkspacePoliciesParams{}))
		}},
		{"GetWorkspacePolicy", access.PermPolicyRead, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.GetWorkspacePolicy(ctx, p, org, ws, wsBindingID))
		}},
		{"GrantWorkspacePolicy", access.PermPolicyModify, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.GrantWorkspacePolicy(ctx, p, org, ws, rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID}))
		}},
		{"RevokeWorkspacePolicy", access.PermPolicyModify, func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.RevokeWorkspacePolicy(ctx, p, org, ws, wsBindingID))
		}},
		{"ListWorkspacePermissions", "", func(h *harness, p access.Principal, org, ws int64) error {
			return accesstest.Err(h.svc.ListWorkspacePermissions(ctx, p, org, ws))
		}},
	}
}

func TestWorkspaceDenyMatrix(t *testing.T) {
	for _, tc := range workspaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			accesstest.AssertForbidden(t, tc.call(h, account(nonMemberID), testOrg, testWS), "non-member")
			accesstest.AssertForbidden(t, tc.call(h, account(actorID), otherOrg, foreignWS), "foreign org")
			accesstest.AssertNotFound(t, tc.call(h, account(actorID), testOrg, missingWS), "missing workspace")
			accesstest.AssertNotFound(t, tc.call(h, account(actorID), testOrg, foreignWS), "workspace of another org")
			accesstest.AssertNotFound(t, tc.call(h, account(actorID), testOrg, personalWS), "personal workspace")

			// Organization-level grants do not stand in for workspace grants.
			h.policy.set(readerID, access.AllPermissions()...)
			if tc.permission != "" {
				h.policy.setWorkspace(readerID, allBut(tc.permission)...)
				accesstest.AssertForbidden(t, tc.call(h, account(readerID), testOrg, testWS), "member without "+tc.permission)
				h.policy.set(readerID)
				h.policy.setWorkspace(readerID, tc.permission)
			} else {
				h.policy.set(readerID)
				h.policy.setWorkspace(readerID)
			}
			if err := tc.call(h, account(readerID), testOrg, testWS); err != nil {
				t.Fatalf("member holding only %q on the workspace: %v", tc.permission, err)
			}
		})
	}
}

func TestWorkspaceRolesOfOtherScopesAreNotFound(t *testing.T) {
	ctx := context.Background()
	p := account(actorID)
	for name, roleID := range map[string]int64{
		"organization role":     customRoleID,
		"org-defined conn role": connRoleID,
		"sibling workspace":     siblingWSRoleID,
		"foreign org":           foreignRoleID,
		"missing":               999,
	} {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetWorkspaceRole(ctx, p, testOrg, testWS, roleID)), "get")
			accesstest.AssertNotFound(t, accesstest.Err(h.svc.UpdateWorkspaceRole(ctx, p, testOrg, testWS, roleID, rbac.UpdateRoleInput{Name: "X"})), "update")
			accesstest.AssertNotFound(t, h.svc.DeleteWorkspaceRole(ctx, p, testOrg, testWS, roleID), "delete")
			if len(h.store.updated) != 0 || len(h.store.deleted) != 0 || len(h.events) != 0 {
				t.Fatalf("role outside workspace changed: updated=%v deleted=%v", h.store.updated, h.store.deleted)
			}
		})
	}
}

func TestWorkspaceBuiltinRolesAreImmutable(t *testing.T) {
	h := newWorkspaceHarness(t)
	ctx := context.Background()
	_, err := h.svc.UpdateWorkspaceRole(ctx, account(actorID), testOrg, testWS, wsBuiltinRoleID, rbac.UpdateRoleInput{Name: "Renamed"})
	if !errors.Is(err, rbac.ErrBuiltinRole) {
		t.Fatalf("UpdateWorkspaceRole builtin error = %v, want ErrBuiltinRole", err)
	}
	if err := h.svc.DeleteWorkspaceRole(ctx, account(actorID), testOrg, testWS, wsBuiltinRoleID); !errors.Is(err, rbac.ErrBuiltinRole) {
		t.Fatalf("DeleteWorkspaceRole builtin error = %v, want ErrBuiltinRole", err)
	}
	if len(h.store.updated) != 0 || len(h.store.deleted) != 0 || len(h.events) != 0 {
		t.Fatalf("builtin role changed")
	}
}

func TestUpdateWorkspaceRolePrivilegedPermissions(t *testing.T) {
	ctx := context.Background()
	h := newWorkspaceHarness(t)
	h.policy.set(readerID)
	h.policy.setWorkspace(readerID, access.PermPolicyModify)

	_, err := h.svc.UpdateWorkspaceRole(ctx, account(readerID), testOrg, testWS, wsRoleID, rbac.UpdateRoleInput{Name: "Workspace", Permissions: []string{access.PermOrgDelete}})
	var validation *rbac.ValidationError
	if !errors.As(err, &validation) || validation.Fields["permissions"] == "" {
		t.Fatalf("adding org:delete error = %v, want permissions validation", err)
	}

	in := rbac.UpdateRoleInput{Name: "WsDeleter", Permissions: []string{access.PermWsRead}}
	_, err = h.svc.UpdateWorkspaceRole(ctx, account(readerID), testOrg, testWS, wsPrivRoleID, in)
	if !errors.Is(err, rbac.ErrPrivilegedPolicy) {
		t.Fatalf("removing org:delete error = %v, want ErrPrivilegedPolicy", err)
	}
	if len(h.store.updated) != 0 || len(h.events) != 0 {
		t.Fatalf("role changed despite refusal: updated=%v", h.store.updated)
	}

	h.policy.set(readerID, access.PermOrgDelete)
	if _, err := h.svc.UpdateWorkspaceRole(ctx, account(readerID), testOrg, testWS, wsPrivRoleID, in); err != nil {
		t.Fatalf("removing org:delete while holding it: %v", err)
	}
	if !slices.Equal(h.actions(), []string{rbac.ActionRoleUpdated}) || h.events[0].Metadata["workspace_id"] != "9" {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestCreateWorkspaceRoleValidatesScopeBeforeWriting(t *testing.T) {
	cases := map[string]struct {
		in    rbac.CreateWorkspaceRoleInput
		field string
		msg   string
	}{
		"name":           {rbac.CreateWorkspaceRoleInput{Permissions: []string{access.PermWsRead}}, "name", "Name is required."},
		"org scope":      {rbac.CreateWorkspaceRoleInput{Name: "R", ScopeType: "org"}, "scope_type", "Scope type must be workspace, environment, or connection."},
		"org permission": {rbac.CreateWorkspaceRoleInput{Name: "R", Permissions: []string{access.PermOrgDelete}}, "permissions", "Permission org:delete is not valid for workspace scope."},
		"env scope":      {rbac.CreateWorkspaceRoleInput{Name: "R", ScopeType: "environment", Permissions: []string{access.PermWsDelete}}, "permissions", "Permission ws:delete is not valid for environment scope."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			_, err := h.svc.CreateWorkspaceRole(context.Background(), account(actorID), testOrg, testWS, tc.in)
			var validation *rbac.ValidationError
			if !errors.As(err, &validation) || validation.Fields[tc.field] != tc.msg {
				t.Fatalf("error = %v, want %s=%q", err, tc.field, tc.msg)
			}
			if len(h.store.created) != 0 {
				t.Fatalf("store called despite invalid input")
			}
		})
	}
}

func TestCreateWorkspaceRoleOwnedByWorkspaceAndAudited(t *testing.T) {
	h := newWorkspaceHarness(t)
	role, err := h.svc.CreateWorkspaceRole(context.Background(), account(actorID), testOrg, testWS, rbac.CreateWorkspaceRoleInput{
		Name: "Readers", Permissions: []string{access.PermWsRead},
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceRole: %v", err)
	}
	if role.ScopeType != "workspace" || role.WorkspaceID == nil || *role.WorkspaceID != testWS {
		t.Fatalf("role = %+v", role)
	}
	if len(h.events) != 1 || h.events[0].Action != rbac.ActionRoleCreated ||
		h.events[0].Metadata["workspace_id"] != "9" || h.events[0].Metadata["permissions"] != access.PermWsRead {
		t.Fatalf("events = %+v", h.events)
	}

	h.store.createErr = errors.New("UNIQUE constraint failed: roles.org_id, roles.name")
	_, err = h.svc.CreateWorkspaceRole(context.Background(), account(actorID), testOrg, testWS, rbac.CreateWorkspaceRoleInput{Name: "Readers"})
	var validation *rbac.ValidationError
	if !errors.As(err, &validation) || validation.Fields["name"] != "A role with this name already exists in this workspace." {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestDeleteWorkspaceRoleInUseAndAudit(t *testing.T) {
	h := newWorkspaceHarness(t)
	ctx := context.Background()
	if err := h.svc.DeleteWorkspaceRole(ctx, account(actorID), testOrg, testWS, wsRoleID); !errors.Is(err, rbac.ErrRoleInUse) {
		t.Fatalf("bound role error = %v, want ErrRoleInUse", err)
	}
	if err := h.svc.DeleteWorkspaceRole(ctx, account(actorID), testOrg, testWS, wsUnboundRoleID); err != nil {
		t.Fatalf("DeleteWorkspaceRole: %v", err)
	}
	if !slices.Equal(h.actions(), []string{rbac.ActionRoleDeleted}) || h.events[0].Metadata["workspace_id"] != "9" {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestGrantWorkspacePolicyResourceMustBelongToWorkspace(t *testing.T) {
	ws := testWS
	cases := map[string]struct {
		role         int64
		resourceType string
		resourceID   int64
		wantID       int64
		ok           bool
	}{
		"workspace":           {wsRoleID, "", 0, testWS, true},
		"own environment":     {envRoleID, "environment", testEnv, testEnv, true},
		"own connection":      {connRoleID, "connection", testConn, testConn, true},
		"sibling environment": {envRoleID, "environment", siblingEnv, 0, false},
		"sibling connection":  {connRoleID, "connection", siblingConn, 0, false},
		"foreign environment": {envRoleID, "environment", foreignEnv, 0, false},
		"foreign connection":  {connRoleID, "connection", foreignConn, 0, false},
		"missing environment": {envRoleID, "environment", 999, 0, false},
		"missing connection":  {connRoleID, "connection", 999, 0, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			got, err := h.svc.GrantWorkspacePolicy(context.Background(), account(actorID), testOrg, ws, rbac.GrantWorkspacePolicyInput{
				RoleID: tc.role, SubjectType: access.SubjectTypeAccount, SubjectID: readerID, ResourceType: tc.resourceType, ResourceID: tc.resourceID,
			})
			if !tc.ok {
				accesstest.AssertNotFound(t, err, "grant")
				if len(h.store.bound) != 0 || len(h.events) != 0 {
					t.Fatalf("bound outside workspace: %+v", h.store.bound)
				}
				return
			}
			if err != nil || got != tc.wantID {
				t.Fatalf("grant = %d, %v; want %d", got, err, tc.wantID)
			}
			resourceType := tc.resourceType
			if resourceType == "" {
				resourceType = "workspace"
			}
			want := bindCall{roleID: tc.role, subjectType: access.SubjectTypeAccount, subjectID: readerID, resourceType: resourceType, resourceID: tc.wantID, grantedBy: actorID}
			if !slices.Equal(h.store.bound, []bindCall{want}) {
				t.Fatalf("bound = %+v, want %+v", h.store.bound, want)
			}
			e := h.events[0]
			if e.Action != rbac.ActionPolicyGranted || e.Metadata["workspace_id"] != "9" || e.Metadata["resource_type"] != resourceType {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestGrantWorkspacePolicyRoleChecks(t *testing.T) {
	ctx := context.Background()
	scopeMismatch := "Role scope must match resource type."
	cases := map[string]struct {
		in       rbac.GrantWorkspacePolicyInput
		notFound bool
		field    string
		msg      string
	}{
		"org role on workspace":      {in: rbac.GrantWorkspacePolicyInput{RoleID: customRoleID}, field: "role_id", msg: scopeMismatch},
		"env role on workspace":      {in: rbac.GrantWorkspacePolicyInput{RoleID: envRoleID}, field: "role_id", msg: scopeMismatch},
		"workspace role on env":      {in: rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, ResourceType: "environment", ResourceID: testEnv}, field: "role_id", msg: scopeMismatch},
		"sibling workspace role":     {in: rbac.GrantWorkspacePolicyInput{RoleID: siblingWSRoleID}, notFound: true},
		"foreign org role":           {in: rbac.GrantWorkspacePolicyInput{RoleID: foreignRoleID}, notFound: true},
		"missing role":               {in: rbac.GrantWorkspacePolicyInput{RoleID: 999}, notFound: true},
		"invalid resource type":      {in: rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, ResourceType: "org"}, field: "resource_type", msg: "Resource type must be workspace, environment, or connection."},
		"missing env resource":       {in: rbac.GrantWorkspacePolicyInput{RoleID: envRoleID, ResourceType: "environment"}, field: "resource_id", msg: "Resource is required for non-workspace resources."},
		"missing role id":            {in: rbac.GrantWorkspacePolicyInput{}, field: "role_id", msg: "Role is required."},
		"invalid subject type":       {in: rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, SubjectType: "role"}, field: "subject_type", msg: "Subject type must be account, team, org_members, or workspace_members."},
		"workspace members of other": {in: rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, SubjectType: access.SubjectTypeWorkspaceMembers, SubjectID: siblingWS}, notFound: true},
		"foreign team":               {in: rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, SubjectType: access.SubjectTypeTeam, SubjectID: foreignTeamID}, notFound: true},
		"non-member account":         {in: rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: nonMemberID}, notFound: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			in := tc.in
			if in.SubjectType == "" {
				in.SubjectType, in.SubjectID = access.SubjectTypeAccount, readerID
			}
			_, err := h.svc.GrantWorkspacePolicy(ctx, account(actorID), testOrg, testWS, in)
			if tc.notFound {
				accesstest.AssertNotFound(t, err, name)
			} else {
				var validation *rbac.ValidationError
				if !errors.As(err, &validation) || validation.Fields[tc.field] != tc.msg {
					t.Fatalf("error = %v, want %s=%q", err, tc.field, tc.msg)
				}
			}
			if len(h.store.bound) != 0 {
				t.Fatalf("bound = %+v", h.store.bound)
			}
		})
	}
}

func TestGrantWorkspacePolicySubjects(t *testing.T) {
	for name, in := range map[string]rbac.GrantWorkspacePolicyInput{
		"workspace members": {RoleID: wsRoleID, SubjectType: access.SubjectTypeWorkspaceMembers, SubjectID: testWS},
		"org members":       {RoleID: wsRoleID, SubjectType: access.SubjectTypeOrgMembers, SubjectID: testOrg},
		"team":              {RoleID: wsRoleID, SubjectType: access.SubjectTypeTeam, SubjectID: teamID},
	} {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			if _, err := h.svc.GrantWorkspacePolicy(context.Background(), account(actorID), testOrg, testWS, in); err != nil {
				t.Fatalf("grant: %v", err)
			}
			if len(h.store.bound) != 1 {
				t.Fatalf("bound = %+v", h.store.bound)
			}
		})
	}
}

func TestWorkspacePrivilegedPolicyRequiresActorToHoldPermissions(t *testing.T) {
	ctx := context.Background()
	h := newWorkspaceHarness(t)
	h.policy.setWorkspace(readerID, access.PermPolicyModify)
	h.policy.set(readerID, access.PermPolicyModify, access.PermOrgTransferOwnership)

	_, err := h.svc.GrantWorkspacePolicy(ctx, account(readerID), testOrg, testWS, rbac.GrantWorkspacePolicyInput{RoleID: wsPrivRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: actorID})
	var refusal *rbac.RefusalError
	if !errors.Is(err, rbac.ErrPrivilegedPolicy) || !errors.As(err, &refusal) || refusal.Role.ID != wsPrivRoleID {
		t.Fatalf("grant error = %v, want privileged refusal", err)
	}
	_, err = h.svc.RevokeWorkspacePolicy(ctx, account(readerID), testOrg, testWS, wsPrivBindingID)
	if !errors.Is(err, rbac.ErrPrivilegedPolicy) || !errors.As(err, &refusal) || refusal.Binding.ID != wsPrivBindingID {
		t.Fatalf("revoke error = %v, want privileged refusal", err)
	}
	if len(h.store.bound) != 0 || len(h.store.unbound) != 0 || len(h.events) != 0 {
		t.Fatalf("privileged change applied")
	}

	h.policy.set(readerID, access.PermPolicyModify, access.PermOrgDelete)
	if _, err := h.svc.GrantWorkspacePolicy(ctx, account(readerID), testOrg, testWS, rbac.GrantWorkspacePolicyInput{RoleID: wsPrivRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: actorID}); err != nil {
		t.Fatalf("grant with privilege: %v", err)
	}
	if _, err := h.svc.RevokeWorkspacePolicy(ctx, account(readerID), testOrg, testWS, wsPrivBindingID); err != nil {
		t.Fatalf("revoke with privilege: %v", err)
	}
}

func TestWorkspaceBindingsOutsideWorkspaceAreNotFound(t *testing.T) {
	ctx := context.Background()
	p := account(actorID)
	for name, bindingID := range map[string]int64{
		"org binding":         customBindingID,
		"sibling workspace":   siblingWSBinding,
		"sibling environment": siblingEnvBinding,
		"foreign org":         foreignBindingID,
		"missing":             999,
	} {
		t.Run(name, func(t *testing.T) {
			h := newWorkspaceHarness(t)
			accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetWorkspacePolicy(ctx, p, testOrg, testWS, bindingID)), "get")
			accesstest.AssertNotFound(t, accesstest.Err(h.svc.RevokeWorkspacePolicy(ctx, p, testOrg, testWS, bindingID)), "revoke")
			if len(h.store.unbound) != 0 {
				t.Fatalf("binding outside workspace revoked")
			}
		})
	}
}

func TestRevokeWorkspacePolicyOnEnvironmentAudits(t *testing.T) {
	h := newWorkspaceHarness(t)
	b, err := h.svc.RevokeWorkspacePolicy(context.Background(), account(actorID), testOrg, testWS, envBindingID)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if b.ID != envBindingID || !slices.Equal(h.store.unbound, []int64{envBindingID}) {
		t.Fatalf("revoked = %+v, unbound = %v", b, h.store.unbound)
	}
	e := h.events[0]
	if e.Action != rbac.ActionPolicyRevoked || e.ResourceID != "205" || e.Metadata["resource_type"] != "environment" || e.Metadata["resource_id"] != "40" || e.Metadata["workspace_id"] != "9" {
		t.Fatalf("event = %+v", e)
	}
}

func TestWorkspaceAuditFailureDoesNotFailChange(t *testing.T) {
	h := newWorkspaceHarness(t)
	h.auditErr = errors.New("audit down")
	if _, err := h.svc.GrantWorkspacePolicy(context.Background(), account(actorID), testOrg, testWS, rbac.GrantWorkspacePolicyInput{RoleID: wsRoleID, SubjectType: access.SubjectTypeAccount, SubjectID: readerID}); err != nil {
		t.Fatalf("grant with failing audit: %v", err)
	}
}

func TestListWorkspaceScopesToWorkspace(t *testing.T) {
	h := newWorkspaceHarness(t)
	ctx := context.Background()
	roles, err := h.svc.ListWorkspaceRoles(ctx, account(actorID), testOrg, testWS, rbac.ListWorkspaceRolesInput{})
	if err != nil || len(roles.Items) != 1 || roles.Items[0].WorkspaceID == nil || *roles.Items[0].WorkspaceID != testWS {
		t.Fatalf("roles = %+v, %v", roles, err)
	}
	var validation *rbac.ValidationError
	if _, err := h.svc.ListWorkspaceRoles(ctx, account(actorID), testOrg, testWS, rbac.ListWorkspaceRolesInput{Builtin: "maybe"}); !errors.As(err, &validation) || validation.Fields["builtin"] == "" {
		t.Fatalf("builtin error = %v", err)
	}
	policies, err := h.svc.ListWorkspacePolicies(ctx, account(actorID), testOrg, testWS, database.ListWorkspacePoliciesParams{OrgID: otherOrg, WorkspaceID: foreignWS})
	if err != nil || len(policies.Items) != 1 || policies.Items[0].ResourceID != testWS {
		t.Fatalf("policies = %+v, %v", policies, err)
	}
	catalog, err := h.svc.ListWorkspacePermissions(ctx, account(actorID), testOrg, testWS)
	if err != nil || !slices.Equal(catalog.Permissions, access.ScopePermissions["workspace"]) || len(catalog.PermissionDetails) != len(catalog.Permissions) {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
}
