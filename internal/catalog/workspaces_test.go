package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// orgFakeStore adds the organization lookup the Requirer's Org check needs.
type orgFakeStore struct{ *fakeStore }

func (s orgFakeStore) GetOrg(_ context.Context, id int64) (database.Organization, bool, error) {
	if id != 1 {
		return database.Organization{}, false, nil
	}
	return database.Organization{ID: 1, Slug: "acme", Name: "Acme"}, true, nil
}

type fakeWorkspaceStore struct {
	store *fakeStore

	accessible       map[int64]bool
	accessibleEnvs   map[int64]bool
	created          []string
	createErr        error
	updated          map[int64]string
	deleted          []int64
	envConnections   map[int64][]int64
	deleteEnvErr     error
	deletedEnvs      []int64
	insertEnvErr     error
	members          map[[2]int64]bool
	teams            map[int64]database.Team
	teamMembers      map[int64][]int64
	workspaceTeams   map[[2]int64]bool
	addMemberCreator *int64
	memberParams     database.ListWorkspaceMembersParams
}

func newFakeWorkspaceStore(store *fakeStore) *fakeWorkspaceStore {
	return &fakeWorkspaceStore{
		store:          store,
		accessible:     map[int64]bool{},
		accessibleEnvs: map[int64]bool{},
		updated:        map[int64]string{},
		envConnections: map[int64][]int64{},
		members:        map[[2]int64]bool{},
		teams:          map[int64]database.Team{},
		teamMembers:    map[int64][]int64{},
		workspaceTeams: map[[2]int64]bool{},
	}
}

func (f *fakeWorkspaceStore) ListAccessibleWorkspaces(_ context.Context, _, orgID int64) ([]database.Workspace, error) {
	var out []database.Workspace
	for id, ws := range f.store.workspaces {
		if f.accessible[id] && ws.OrgID != nil && *ws.OrgID == orgID {
			out = append(out, ws)
		}
	}
	return out, nil
}
func (f *fakeWorkspaceStore) HasAccessibleWorkspace(_ context.Context, _, _, workspaceID int64) (bool, error) {
	return f.accessible[workspaceID], nil
}
func (f *fakeWorkspaceStore) PopulateWorkspaceCounts(_ context.Context, workspaces []database.Workspace) error {
	for i := range workspaces {
		workspaces[i].EnvironmentCount = 1
	}
	return nil
}
func (f *fakeWorkspaceStore) CreateOwnedWorkspace(_ context.Context, orgID, _ int64, name, description string) (database.Workspace, error) {
	if f.createErr != nil {
		return database.Workspace{}, f.createErr
	}
	f.created = append(f.created, name)
	f.store.nextID++
	ws := database.Workspace{ID: f.store.nextID, OrgID: &orgID, OwnerType: "org", OwnerID: orgID, Name: name, Description: description}
	f.store.workspaces[ws.ID] = ws
	return ws, nil
}
func (f *fakeWorkspaceStore) UpdateWorkspace(_ context.Context, id int64, name, _ string) error {
	f.updated[id] = name
	return nil
}
func (f *fakeWorkspaceStore) DeleteWorkspace(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	delete(f.store.workspaces, id)
	return nil
}
func (f *fakeWorkspaceStore) ListAccessibleEnvironments(_ context.Context, _, _, workspaceID int64) ([]database.Environment, error) {
	var out []database.Environment
	for id, env := range f.store.environments {
		if f.accessibleEnvs[id] && env.WorkspaceID == workspaceID {
			out = append(out, env)
		}
	}
	return out, nil
}
func (f *fakeWorkspaceStore) HasAccessibleEnvironment(_ context.Context, _, _, _, environmentID int64) (bool, error) {
	return f.accessibleEnvs[environmentID], nil
}
func (f *fakeWorkspaceStore) InsertEnvironment(_ context.Context, workspaceID int64, name, _ string) (database.Environment, error) {
	if f.insertEnvErr != nil {
		return database.Environment{}, f.insertEnvErr
	}
	f.store.nextID++
	env := database.Environment{ID: f.store.nextID, WorkspaceID: workspaceID, Name: name}
	f.store.environments[env.ID] = env
	return env, nil
}
func (f *fakeWorkspaceStore) UpdateEnvironment(_ context.Context, id int64, name, _ string) error {
	env := f.store.environments[id]
	env.Name = name
	f.store.environments[id] = env
	return nil
}
func (f *fakeWorkspaceStore) ListConnectionIDsByEnvironment(_ context.Context, environmentID int64) ([]int64, error) {
	return f.envConnections[environmentID], nil
}
func (f *fakeWorkspaceStore) ListAccessibleConnections(_ context.Context, _, _, workspaceID int64) ([]database.Connection, error) {
	var out []database.Connection
	for _, conn := range f.store.connections {
		if conn.WorkspaceID == workspaceID {
			out = append(out, conn)
		}
	}
	return out, nil
}
func (f *fakeWorkspaceStore) DeleteEnvironment(_ context.Context, id int64) error {
	if f.deleteEnvErr != nil {
		return f.deleteEnvErr
	}
	f.deletedEnvs = append(f.deletedEnvs, id)
	delete(f.store.environments, id)
	return nil
}
func (f *fakeWorkspaceStore) ListWorkspaceMembersPage(_ context.Context, params database.ListWorkspaceMembersParams) (response.Paginated[database.WorkspaceMemberListItem], error) {
	f.memberParams = params
	return response.Paginated[database.WorkspaceMemberListItem]{Items: []database.WorkspaceMemberListItem{}, Page: params.Page, PageSize: params.PageSize}, nil
}
func (f *fakeWorkspaceStore) ListWorkspaceEffectiveMembersPage(_ context.Context, _ int64, params database.ListWorkspaceMembersParams) (response.Paginated[database.WorkspaceEffectiveMemberListItem], error) {
	return response.Paginated[database.WorkspaceEffectiveMemberListItem]{Items: []database.WorkspaceEffectiveMemberListItem{}, Page: params.Page, PageSize: params.PageSize}, nil
}
func (f *fakeWorkspaceStore) AddWorkspaceMember(_ context.Context, workspaceID, accountID int64, createdBy *int64) error {
	f.members[[2]int64{workspaceID, accountID}] = true
	f.addMemberCreator = createdBy
	return nil
}
func (f *fakeWorkspaceStore) RemoveWorkspaceMember(_ context.Context, workspaceID, accountID int64) error {
	delete(f.members, [2]int64{workspaceID, accountID})
	return nil
}
func (f *fakeWorkspaceStore) ListWorkspaceTeamsPage(_ context.Context, params database.ListWorkspaceTeamsParams) (response.Paginated[database.WorkspaceTeamListItem], error) {
	return response.Paginated[database.WorkspaceTeamListItem]{Items: []database.WorkspaceTeamListItem{}, Page: params.Page, PageSize: params.PageSize}, nil
}
func (f *fakeWorkspaceStore) GetTeamByID(_ context.Context, id int64) (database.Team, bool, error) {
	team, ok := f.teams[id]
	return team, ok, nil
}
func (f *fakeWorkspaceStore) AddWorkspaceTeam(_ context.Context, workspaceID, teamID int64, _ *int64) error {
	f.workspaceTeams[[2]int64{workspaceID, teamID}] = true
	return nil
}
func (f *fakeWorkspaceStore) RemoveWorkspaceTeam(_ context.Context, workspaceID, teamID int64) error {
	delete(f.workspaceTeams, [2]int64{workspaceID, teamID})
	return nil
}
func (f *fakeWorkspaceStore) ListWorkspaceTeamMemberAccountIDs(_ context.Context, workspaceID, teamID int64) ([]int64, error) {
	if !f.workspaceTeams[[2]int64{workspaceID, teamID}] {
		return nil, nil
	}
	return f.teamMembers[teamID], nil
}

type fakeCache struct {
	orgPolicy  []int64
	principals [][2]int64
}

func (c *fakeCache) InvalidateOrgPolicy(orgID int64) { c.orgPolicy = append(c.orgPolicy, orgID) }
func (c *fakeCache) InvalidatePrincipals(orgID, accountID int64) {
	c.principals = append(c.principals, [2]int64{orgID, accountID})
}

type workspaceFixtureParts struct {
	service    *Service
	store      *fakeStore
	workspaces *fakeWorkspaceStore
	cache      *fakeCache
	ancestry   *fakeAncestry
	revoker    *fakeRevoker
	audit      *auditRecorder
	principal  access.Principal
}

// workspaceFixture is org 1 with member account 7, org workspace 10 and its
// environment 20, plus workspace 11 in org 2. The policy grants every
// workspace, environment and workspace-membership permission.
func workspaceFixture() workspaceFixtureParts {
	service, store, _, principal := catalogFixture()
	otherOrg := int64(2)
	store.workspaces[11] = database.Workspace{ID: 11, OrgID: &otherOrg, OwnerType: "org", OwnerID: otherOrg}
	store.environments[21] = database.Environment{ID: 21, WorkspaceID: 11}
	workspaces := newFakeWorkspaceStore(store)
	workspaces.accessible[10] = true
	workspaces.accessibleEnvs[20] = true
	cache, ancestry, revoker, recorder := &fakeCache{}, &fakeAncestry{}, &fakeRevoker{}, &auditRecorder{}
	service.deps.Store = orgFakeStore{store}
	service.deps.Workspaces = workspaces
	service.deps.Cache = cache
	service.deps.Ancestry = ancestry
	service.deps.Revoker = revoker
	service.deps.Audit = recorder
	service.deps.Policy = fakePolicy{permissions: map[string]bool{
		access.PermWsCreate: true, access.PermWsWrite: true, access.PermWsDelete: true,
		access.PermEnvCreate: true, access.PermEnvWrite: true, access.PermEnvDelete: true,
		access.PermPolicyRead: true, access.PermPolicyModify: true,
	}}
	return workspaceFixtureParts{service, store, workspaces, cache, ancestry, revoker, recorder, principal}
}

// workspaceCalls exercises every workspace, environment and membership method
// against the routed org, workspace and environment.
func workspaceCalls(orgID, workspaceID, environmentID int64) map[string]func(*Service, access.Principal) error {
	ctx := context.Background()
	return map[string]func(*Service, access.Principal) error{
		"get workspace": func(s *Service, p access.Principal) error {
			_, err := s.GetWorkspace(ctx, p, orgID, workspaceID)
			return err
		},
		"update workspace": func(s *Service, p access.Principal) error {
			return s.UpdateWorkspace(ctx, p, orgID, workspaceID, UpdateWorkspaceInput{Name: "x"})
		},
		"delete workspace": func(s *Service, p access.Principal) error { return s.DeleteWorkspace(ctx, p, orgID, workspaceID) },
		"list environments": func(s *Service, p access.Principal) error {
			_, err := s.ListEnvironments(ctx, p, orgID, workspaceID, ListQuery{Page: 1, PageSize: 25})
			return err
		},
		"create environment": func(s *Service, p access.Principal) error {
			_, err := s.CreateEnvironment(ctx, p, orgID, workspaceID, EnvironmentInput{Name: "x"})
			return err
		},
		"get environment": func(s *Service, p access.Principal) error {
			_, err := s.GetEnvironment(ctx, p, orgID, workspaceID, environmentID)
			return err
		},
		"update environment": func(s *Service, p access.Principal) error {
			return s.UpdateEnvironment(ctx, p, orgID, workspaceID, environmentID, UpdateEnvironmentInput{Name: "x"})
		},
		"delete environment": func(s *Service, p access.Principal) error {
			_, err := s.DeleteEnvironment(ctx, p, orgID, workspaceID, environmentID)
			return err
		},
		"list connections": func(s *Service, p access.Principal) error {
			_, err := s.ListConnections(ctx, p, ListConnectionsInput{OrgID: orgID, WorkspaceID: workspaceID, RouteEnvironmentID: &environmentID, Page: 1, PageSize: 25})
			return err
		},
		"list members": func(s *Service, p access.Principal) error {
			_, err := s.ListWorkspaceMembers(ctx, p, orgID, workspaceID, database.ListWorkspaceMembersParams{})
			return err
		},
		"list effective members": func(s *Service, p access.Principal) error {
			_, err := s.ListWorkspaceEffectiveMembers(ctx, p, orgID, workspaceID, database.ListWorkspaceMembersParams{})
			return err
		},
		"add member": func(s *Service, p access.Principal) error {
			return s.AddWorkspaceMember(ctx, p, orgID, workspaceID, 7)
		},
		"remove member": func(s *Service, p access.Principal) error {
			return s.RemoveWorkspaceMember(ctx, p, orgID, workspaceID, 7)
		},
		"list teams": func(s *Service, p access.Principal) error {
			_, err := s.ListWorkspaceTeams(ctx, p, orgID, workspaceID, database.ListWorkspaceTeamsParams{})
			return err
		},
		"add team": func(s *Service, p access.Principal) error {
			_, err := s.AddWorkspaceTeam(ctx, p, orgID, workspaceID, 5)
			return err
		},
		"remove team": func(s *Service, p access.Principal) error {
			_, err := s.RemoveWorkspaceTeam(ctx, p, orgID, workspaceID, 5)
			return err
		},
	}
}

func TestWorkspaceServiceDenyMatrix(t *testing.T) {
	ctx := context.Background()
	orgCalls := map[string]func(*Service, access.Principal) error{
		"list workspaces": func(s *Service, p access.Principal) error {
			_, err := s.ListWorkspaces(ctx, p, 1, ListQuery{Page: 1, PageSize: 25})
			return err
		},
		"create workspace": func(s *Service, p access.Principal) error {
			_, err := s.CreateWorkspace(ctx, p, 1, CreateWorkspaceInput{Name: "x"})
			return err
		},
	}

	for label, call := range orgCalls {
		t.Run(label+"/non member", func(t *testing.T) {
			f := workspaceFixture()
			f.principal.Subject.ID = 99
			if err := call(f.service, f.principal); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
	t.Run("create workspace/no permission", func(t *testing.T) {
		f := workspaceFixture()
		f.service.deps.Policy = fakePolicy{permissions: map[string]bool{}}
		if err := orgCalls["create workspace"](f.service, f.principal); !errors.Is(err, ErrForbidden) {
			t.Fatalf("error = %v, want ErrForbidden", err)
		}
		if len(f.workspaces.created) != 0 {
			t.Fatal("denied create wrote a workspace")
		}
	})

	for label, call := range workspaceCalls(1, 10, 20) {
		t.Run(label+"/non member", func(t *testing.T) {
			f := workspaceFixture()
			f.principal.Subject.ID = 99
			if err := call(f.service, f.principal); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
	for label, call := range workspaceCalls(1, 11, 21) {
		t.Run(label+"/foreign workspace", func(t *testing.T) {
			f := workspaceFixture()
			if err := call(f.service, f.principal); !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
			if _, ok := f.store.workspaces[11]; !ok {
				t.Fatal("foreign workspace was deleted")
			}
		})
	}
	for _, label := range []string{"get environment", "update environment", "delete environment", "list connections"} {
		t.Run(label+"/foreign environment", func(t *testing.T) {
			f := workspaceFixture()
			if err := workspaceCalls(1, 10, 21)[label](f.service, f.principal); !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
			if _, ok := f.store.environments[21]; !ok {
				t.Fatal("foreign environment was deleted")
			}
		})
	}
	for _, label := range []string{
		"update workspace", "delete workspace", "create environment", "update environment", "delete environment",
		"list members", "list effective members", "add member", "remove member", "list teams", "add team", "remove team",
	} {
		t.Run(label+"/no permission", func(t *testing.T) {
			f := workspaceFixture()
			f.service.deps.Policy = fakePolicy{permissions: map[string]bool{}}
			if err := workspaceCalls(1, 10, 20)[label](f.service, f.principal); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
			if len(f.workspaces.deleted)+len(f.workspaces.deletedEnvs)+len(f.workspaces.updated)+len(f.revoker.workspaceRevokes)+len(f.cache.principals) != 0 {
				t.Fatal("denied call changed state")
			}
		})
	}
}

func TestWorkspaceReadsHideInaccessibleResources(t *testing.T) {
	f := workspaceFixture()
	ctx := context.Background()
	f.workspaces.accessible[10] = false
	if _, err := f.service.GetWorkspace(ctx, f.principal, 1, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetWorkspace error = %v, want ErrNotFound", err)
	}
	f.workspaces.accessibleEnvs[20] = false
	if _, err := f.service.GetEnvironment(ctx, f.principal, 1, 10, 20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetEnvironment error = %v, want ErrNotFound", err)
	}
}

func TestListWorkspacesFiltersSortsAndPages(t *testing.T) {
	f := workspaceFixture()
	orgID := int64(1)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.store.workspaces = map[int64]database.Workspace{
		10: {ID: 10, OrgID: &orgID, OwnerType: "org", OwnerID: orgID, Name: "Data Lake", CreatedAt: base},
		12: {ID: 12, OrgID: &orgID, OwnerType: "org", OwnerID: orgID, Name: "Analytics", CreatedAt: base.Add(time.Hour)},
		13: {ID: 13, OrgID: &orgID, OwnerType: "org", OwnerID: orgID, Name: "Data Mart", CreatedAt: base.Add(2 * time.Hour)},
		14: {ID: 14, OrgID: &orgID, OwnerType: "org", OwnerID: orgID, Name: "Data Hidden", CreatedAt: base},
	}
	f.workspaces.accessible = map[int64]bool{10: true, 12: true, 13: true}

	page, err := f.service.ListWorkspaces(context.Background(), f.principal, 1, ListQuery{Search: "DATA", Sort: "name", Order: "desc", Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Items[0].ID != 13 || page.Items[1].ID != 10 || page.Items[0].EnvironmentCount != 1 {
		t.Fatalf("page = %+v", page)
	}

	page, err = f.service.ListWorkspaces(context.Background(), f.principal, 1, ListQuery{Name: " Analytics ", Page: 1, PageSize: 25})
	if err != nil || page.Total != 1 || page.Items[0].ID != 12 {
		t.Fatalf("page = %+v, %v", page, err)
	}

	page, err = f.service.ListWorkspaces(context.Background(), f.principal, 1, ListQuery{Page: 2, PageSize: 1})
	if err != nil || page.Total != 3 || len(page.Items) != 1 || page.Items[0].ID != 12 {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

func TestListConnectionsFiltersSortsAndScopesToRoutedEnvironment(t *testing.T) {
	f := workspaceFixture()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.store.environments[22] = database.Environment{ID: 22, WorkspaceID: 10}
	f.store.connections = map[int64]database.Connection{
		1: {ID: 1, WorkspaceID: 10, EnvironmentID: 20, Name: "Orders", Driver: "postgres", AccessMode: "read_write", CreatedAt: base},
		2: {ID: 2, WorkspaceID: 10, EnvironmentID: 20, Name: "Billing", Driver: "mysql", AccessMode: "read_only", CreatedAt: base.Add(time.Hour)},
		3: {ID: 3, WorkspaceID: 10, EnvironmentID: 22, Name: "Order Archive", Driver: "postgres", AccessMode: "read_only", CreatedAt: base.Add(2 * time.Hour)},
		4: {ID: 4, WorkspaceID: 11, EnvironmentID: 21, Name: "Orders Foreign", Driver: "postgres", AccessMode: "read_only", CreatedAt: base},
	}
	ctx := context.Background()

	page, err := f.service.ListConnections(ctx, f.principal, ListConnectionsInput{OrgID: 1, WorkspaceID: 10, Search: "ORDER", Sort: "name", Order: "asc", Page: 1, PageSize: 25})
	if err != nil || page.Total != 2 || page.Items[0].ID != 3 || page.Items[1].ID != 1 {
		t.Fatalf("search page = %+v, %v", page, err)
	}

	page, err = f.service.ListConnections(ctx, f.principal, ListConnectionsInput{OrgID: 1, WorkspaceID: 10, Driver: "postgres", AccessMode: "read_only", Page: 1, PageSize: 25})
	if err != nil || page.Total != 1 || page.Items[0].ID != 3 {
		t.Fatalf("driver page = %+v, %v", page, err)
	}

	page, err = f.service.ListConnections(ctx, f.principal, ListConnectionsInput{OrgID: 1, WorkspaceID: 10, Page: 1, PageSize: 25})
	if err != nil || page.Total != 3 || page.Items[0].ID != 3 || page.Items[2].ID != 1 {
		t.Fatalf("default sort page = %+v, %v", page, err)
	}

	routed := int64(20)
	other := int64(22)
	page, err = f.service.ListConnections(ctx, f.principal, ListConnectionsInput{OrgID: 1, WorkspaceID: 10, RouteEnvironmentID: &routed, EnvironmentID: &other, Page: 1, PageSize: 25})
	if err != nil || page.Total != 2 || page.Items[0].ID != 2 || page.Items[1].ID != 1 {
		t.Fatalf("routed environment page = %+v, %v", page, err)
	}
}

func TestCreateWorkspaceValidatesInvalidatesAndAudits(t *testing.T) {
	f := workspaceFixture()
	ctx := context.Background()

	_, err := f.service.CreateWorkspace(ctx, f.principal, 1, CreateWorkspaceInput{})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["name"] != "Name is required." {
		t.Fatalf("error = %v, want name validation", err)
	}

	ws, err := f.service.CreateWorkspace(ctx, f.principal, 1, CreateWorkspaceInput{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}
	if ws.EnvironmentCount != 1 {
		t.Fatalf("counts not populated: %+v", ws)
	}
	if len(f.cache.orgPolicy) != 1 || len(f.cache.principals) != 1 || f.cache.principals[0] != [2]int64{1, 7} {
		t.Fatalf("cache = %+v", f.cache)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(f.audit.events))
	}
	event := f.audit.events[0]
	if event.Action != ActionWorkspaceCreated || event.Resource != "workspace" || event.Outcome != audit.OutcomeSuccess || len(event.Metadata) != 0 {
		t.Fatalf("audit event = %+v", event)
	}

	f.workspaces.createErr = errors.New("UNIQUE constraint failed: workspaces.name")
	_, err = f.service.CreateWorkspace(ctx, f.principal, 1, CreateWorkspaceInput{Name: "Engineering"})
	if !errors.As(err, &validation) || validation.Fields["name"] != "A workspace with this name already exists in this organization." {
		t.Fatalf("error = %v, want duplicate name", err)
	}
}

func TestWorkspaceAuditFailureDoesNotFailChange(t *testing.T) {
	f := workspaceFixture()
	f.service.deps.Audit = failingAudit{}
	if _, err := f.service.CreateWorkspace(context.Background(), f.principal, 1, CreateWorkspaceInput{Name: "Engineering"}); err != nil {
		t.Fatalf("CreateWorkspace error = %v", err)
	}
	if err := f.service.DeleteWorkspace(context.Background(), f.principal, 1, 10); err != nil {
		t.Fatalf("DeleteWorkspace error = %v", err)
	}
}

func TestUpdateWorkspaceRejectsImmutableFields(t *testing.T) {
	f := workspaceFixture()
	orgID, ownerType, ownerID := int64(2), "account", int64(3)
	err := f.service.UpdateWorkspace(context.Background(), f.principal, 1, 10, UpdateWorkspaceInput{OrgID: &orgID, OwnerType: &ownerType, OwnerID: &ownerID})
	var validation *ValidationError
	if !errors.As(err, &validation) || len(validation.Fields) != 4 {
		t.Fatalf("error = %v, want four field errors", err)
	}
	if len(f.workspaces.updated) != 0 {
		t.Fatal("invalid update was written")
	}
}

func TestDeleteWorkspaceInvalidatesAncestryAndAudits(t *testing.T) {
	f := workspaceFixture()
	if err := f.service.DeleteWorkspace(context.Background(), f.principal, 1, 10); err != nil {
		t.Fatal(err)
	}
	if len(f.workspaces.deleted) != 1 || len(f.ancestry.invalidated) != 1 || f.ancestry.invalidated[0] != 10 {
		t.Fatalf("deleted=%v invalidated=%v", f.workspaces.deleted, f.ancestry.invalidated)
	}
	if len(f.audit.events) != 1 || f.audit.events[0].Action != ActionWorkspaceDeleted || f.audit.events[0].ResourceID != "10" {
		t.Fatalf("audit events = %+v", f.audit.events)
	}
}
