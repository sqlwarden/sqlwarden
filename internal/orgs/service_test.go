package orgs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/platform/invalidate"
	"github.com/sqlwarden/internal/response"
)

const (
	testOrg     int64 = 1
	missingOrg  int64 = 3
	ownerID     int64 = 5
	coOwnerID   int64 = 6
	writerID    int64 = 7
	readerID    int64 = 8
	nonMemberID int64 = 9
	transferID  int64 = 10
	adminID     int64 = 11

	ownerRoleID  int64 = 100
	adminRoleID  int64 = 101
	memberRoleID int64 = 102
	customRoleID int64 = 103
)

type fakeStore struct {
	members     map[int64]bool
	owners      map[int64]bool
	snapshotsOn bool
	connections []int64

	created          []string
	createErr        error
	updated          int
	deleted          []int64
	purged           []int64
	removed          []int64
	replaced         []replaceCall
	replaceErr       error
	removedRevokedBy *int64
}

type replaceCall struct {
	accountID, roleID int64
	replacedRoleIDs   []int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		members: map[int64]bool{
			ownerID: true, writerID: true, readerID: true, transferID: true, adminID: true,
		},
		owners:      map[int64]bool{ownerID: true},
		snapshotsOn: true,
		connections: []int64{41, 42},
	}
}

func instanceAdmin(id int64) access.Principal {
	p := account(id)
	p.InstanceAdmin = true
	return p
}

func (s *fakeStore) CreateOwnedOrg(_ context.Context, slug, name string, owner int64) (database.Organization, error) {
	if s.createErr != nil {
		return database.Organization{}, s.createErr
	}
	s.created = append(s.created, slug)
	return database.Organization{ID: 50, Slug: slug, Name: name}, nil
}

func (s *fakeStore) GetOrg(_ context.Context, id int64) (database.Organization, bool, error) {
	if id != testOrg {
		return database.Organization{}, false, nil
	}
	return database.Organization{ID: id, Slug: "acme", Name: "Acme", SchemaSnapshotsEnabled: s.snapshotsOn}, true, nil
}

func (s *fakeStore) UpdateOrgSettings(_ context.Context, _ int64, _ *string, snapshots *bool, _ *bool) error {
	s.updated++
	if snapshots != nil {
		s.snapshotsOn = *snapshots
	}
	return nil
}

func (s *fakeStore) DeleteOrg(_ context.Context, id int64) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *fakeStore) ListOrgConnections(context.Context, int64) ([]database.Connection, error) {
	out := make([]database.Connection, len(s.connections))
	for i, id := range s.connections {
		out[i] = database.Connection{ID: id}
	}
	return out, nil
}

func (s *fakeStore) DeleteSchemaCache(_ context.Context, connID int64) error {
	s.purged = append(s.purged, connID)
	return nil
}

func (s *fakeStore) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return orgID == testOrg && s.members[accountID], nil
}

func (s *fakeStore) ListOrgMembersPage(_ context.Context, params database.ListOrgMembersParams) (response.Paginated[database.OrgMemberListItem], error) {
	return response.Paginated[database.OrgMemberListItem]{Items: []database.OrgMemberListItem{{OrgID: params.OrgID}}}, nil
}

func (s *fakeStore) GetOrgMember(_ context.Context, orgID, accountID int64) (database.OrgMemberListItem, bool, error) {
	if orgID != testOrg || !s.members[accountID] {
		return database.OrgMemberListItem{}, false, nil
	}
	return database.OrgMemberListItem{OrgID: orgID, AccountID: accountID}, true, nil
}

func (s *fakeStore) ListAccountTeamsPage(_ context.Context, params database.ListTeamsParams, _ int64) (response.Paginated[database.Team], error) {
	return response.Paginated[database.Team]{Items: []database.Team{{OrgID: params.OrgID}}}, nil
}

func (s *fakeStore) RemoveOrgMemberAccess(_ context.Context, _ int64, accountID int64, revokedBy *int64, _ string) error {
	s.removed = append(s.removed, accountID)
	s.removedRevokedBy = revokedBy
	delete(s.members, accountID)
	delete(s.owners, accountID)
	return nil
}

func (s *fakeStore) ListOrgRoles(context.Context, int64) ([]database.Role, error) {
	return []database.Role{
		{ID: ownerRoleID, Name: access.BuiltinOrgOwnerRole, IsBuiltin: true, ScopeType: "org"},
		{ID: adminRoleID, Name: access.BuiltinOrgAdminRole, IsBuiltin: true, ScopeType: "org"},
		{ID: memberRoleID, Name: access.BuiltinOrgMemberRole, IsBuiltin: true, ScopeType: "org"},
		{ID: customRoleID, Name: "Custom", ScopeType: "org"},
	}, nil
}

func (s *fakeStore) CountRoleBinding(_ context.Context, _, roleID int64, _ string, _ int64) (int, error) {
	if roleID != ownerRoleID {
		return 0, nil
	}
	return len(s.owners), nil
}

func (s *fakeStore) AccountHasRoleBinding(_ context.Context, _, roleID, accountID int64, _ string, _ int64) (bool, error) {
	return roleID == ownerRoleID && s.owners[accountID], nil
}

func (s *fakeStore) ReplaceOrgMemberBuiltinRole(_ context.Context, _, accountID, roleID int64, replaced []int64, _ int64) error {
	if s.replaceErr != nil {
		return s.replaceErr
	}
	s.replaced = append(s.replaced, replaceCall{accountID: accountID, roleID: roleID, replacedRoleIDs: replaced})
	delete(s.owners, accountID)
	if roleID == ownerRoleID {
		s.owners[accountID] = true
	}
	return nil
}

type fakeDirectory struct{ store *fakeStore }

func (d fakeDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == testOrg, nil
}

func (d fakeDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	// missingOrg reports membership so the Requirer reaches its org lookup.
	if orgID == missingOrg {
		return true, nil
	}
	return d.store.IsOrgMember(context.Background(), orgID, accountID)
}

func (fakeDirectory) IsEffectiveWorkspaceMember(context.Context, int64, int64, int64) (bool, error) {
	return false, nil
}

func (fakeDirectory) GetWorkspace(context.Context, int64) (access.Workspace, bool, error) {
	return access.Workspace{}, false, nil
}

func (fakeDirectory) GetConnection(context.Context, int64) (access.Connection, bool, error) {
	return access.Connection{}, false, nil
}

type fakePolicy struct{}

func (fakePolicy) Can(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if ownerType != "org" || resourceType != "org" || resourceID != orgID {
		return false
	}
	switch accountID {
	case ownerID, coOwnerID:
		return true
	case writerID:
		return permission == access.PermOrgRead || permission == access.PermOrgWrite
	case readerID:
		return permission == access.PermOrgRead
	case transferID:
		return permission == access.PermOrgRead || permission == access.PermOrgTransferOwnership
	default:
		return false
	}
}

func (fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

type fakeCache struct {
	orgs       []int64
	principals [][2]int64
}

func (c *fakeCache) InvalidateOrgPolicy(orgID int64) { c.orgs = append(c.orgs, orgID) }
func (c *fakeCache) InvalidatePrincipals(orgID, accountID int64) {
	c.principals = append(c.principals, [2]int64{orgID, accountID})
}

type fakeRevoker struct {
	calls          [][2]string
	workspaceCalls [][2]string
	err            error
}

func (r *fakeRevoker) RevokeOrgAccount(_ context.Context, orgID, accountID string) (int, error) {
	r.calls = append(r.calls, [2]string{orgID, accountID})
	return 1, r.err
}

func (r *fakeRevoker) RevokeWorkspaceAccount(_ context.Context, workspaceID, accountID string) (int, error) {
	r.workspaceCalls = append(r.workspaceCalls, [2]string{workspaceID, accountID})
	return 1, r.err
}

type harness struct {
	svc     *orgs.Service
	store   *fakeStore
	teams   *fakeTeamStore
	cache   *fakeCache
	revoker *fakeRevoker
	inval   *invalidate.Memory
	invites *fakeInvitationStore
	mailer  *fakeMailer
	events  []audit.Event
}

func newHarness(t *testing.T) *harness {
	return newHarnessWith(t, orgs.InvitationsEnabled)
}

func newHarnessWith(t *testing.T, invitations orgs.InvitationPolicy) *harness {
	t.Helper()
	h := &harness{
		store: newFakeStore(), teams: newFakeTeamStore(), cache: &fakeCache{}, revoker: &fakeRevoker{}, inval: invalidate.NewMemory(nil),
		invites: newFakeInvitationStore(), mailer: &fakeMailer{},
	}
	h.svc = orgs.New(orgs.Deps{
		Store:            h.store,
		Teams:            h.teams,
		Requirer:         access.NewRequirer(fakeDirectory{store: h.store}, fakePolicy{}),
		Cache:            h.cache,
		Revoker:          h.revoker,
		Invalidator:      h.inval,
		Invitations:      h.invites,
		InvitationPolicy: invitations,
		Mailer:           h.mailer,
		BaseURL:          fakeBaseURL("https://sqlwarden.example/"),
		Audit: audit.WriterFunc(func(_ context.Context, e audit.Event) error {
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

func ptr[T any](v T) *T { return &v }

// orgCalls runs every org-scoped method; each entry names the minimum caller
// that is allowed.
func orgCalls(h *harness) map[string]func(access.Principal, int64) error {
	ctx := context.Background()
	return map[string]func(access.Principal, int64) error{
		"Get": func(p access.Principal, org int64) error { return accesstest.Err(h.svc.Get(ctx, p, org)) },
		"Update": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.Update(ctx, p, org, orgs.UpdateOrgInput{Name: ptr("New")}))
		},
		"Delete": func(p access.Principal, org int64) error { return h.svc.Delete(ctx, p, org) },
		"ListMembers": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListMembers(ctx, p, org, database.ListOrgMembersParams{}))
		},
		"GetMember": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.GetMember(ctx, p, org, readerID))
		},
		"ListMemberTeams": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListMemberTeams(ctx, p, org, readerID, database.ListTeamsParams{}))
		},
		"RemoveMember": func(p access.Principal, org int64) error { return h.svc.RemoveMember(ctx, p, org, readerID) },
		"UpdateMemberRole": func(p access.Principal, org int64) error {
			return h.svc.UpdateMemberRole(ctx, p, org, readerID, access.BuiltinOrgAdminRole)
		},
	}
}

func TestOrgMethodsDenyNonMembers(t *testing.T) {
	for name, call := range orgCalls(newHarness(t)) {
		accesstest.AssertForbidden(t, call(account(nonMemberID), testOrg), name)
	}
}

func TestOrgMethodsDenyNonAccountPrincipals(t *testing.T) {
	p := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectServiceAccount, ID: ownerID}}
	for name, call := range orgCalls(newHarness(t)) {
		accesstest.AssertForbidden(t, call(p, testOrg), name)
	}
}

func TestOrgMethodsReportMissingOrgAsNotFound(t *testing.T) {
	for name, call := range orgCalls(newHarness(t)) {
		accesstest.AssertNotFound(t, call(account(ownerID), missingOrg), name)
	}
}

func TestOrgMethodsDenyMembersWithoutPermission(t *testing.T) {
	denied := map[string][]int64{
		"Update":           {readerID, transferID},
		"Delete":           {readerID, writerID, transferID},
		"RemoveMember":     {readerID, transferID},
		"UpdateMemberRole": {readerID, writerID},
	}
	for name, callers := range denied {
		for _, caller := range callers {
			h := newHarness(t)
			accesstest.AssertForbidden(t, orgCalls(h)[name](account(caller), testOrg), name)
			if len(h.store.removed)+len(h.store.replaced)+len(h.store.deleted)+h.store.updated != 0 {
				t.Fatalf("%s by %d changed the store after a denial", name, caller)
			}
		}
	}
	h := newHarness(t)
	for _, name := range []string{"ListMembers", "GetMember", "ListMemberTeams"} {
		accesstest.AssertForbidden(t, orgCalls(h)[name](account(adminID), testOrg), name)
	}
}

func TestOrgMethodsAllowPermittedMembers(t *testing.T) {
	allowed := map[string]int64{
		"Get":              readerID,
		"Update":           writerID,
		"Delete":           ownerID,
		"ListMembers":      readerID,
		"GetMember":        readerID,
		"ListMemberTeams":  readerID,
		"RemoveMember":     writerID,
		"UpdateMemberRole": transferID,
	}
	for name, caller := range allowed {
		h := newHarness(t)
		if err := orgCalls(h)[name](account(caller), testOrg); err != nil {
			t.Fatalf("%s by %d: %v", name, caller, err)
		}
	}
}

func TestCreateRequiresInstanceAdmin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	nonAccount := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectServiceAccount, ID: adminID}, InstanceAdmin: true}
	for name, p := range map[string]access.Principal{"non-admin": account(ownerID), "unflagged": account(adminID), "non-account": nonAccount} {
		accesstest.AssertForbidden(t, accesstest.Err(h.svc.Create(ctx, p, orgs.CreateOrgInput{Name: "Acme"})), "Create by "+name)
	}
	if len(h.store.created) != 0 {
		t.Fatal("non-admin created an organization")
	}

	org, err := h.svc.Create(ctx, instanceAdmin(adminID), orgs.CreateOrgInput{Name: "New Org"})
	if err != nil {
		t.Fatal(err)
	}
	if org.Slug != "new-org" {
		t.Fatalf("slug = %q, want new-org", org.Slug)
	}
	if len(h.cache.orgs) != 1 || h.cache.orgs[0] != org.ID {
		t.Fatalf("policy invalidations = %v", h.cache.orgs)
	}
	if got := h.actions(); len(got) != 1 || got[0] != orgs.ActionOrgCreated {
		t.Fatalf("audit = %v", got)
	}

	flagged := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: readerID}, InstanceAdmin: true}
	if _, err := h.svc.Create(ctx, flagged, orgs.CreateOrgInput{Name: "Flagged"}); err != nil {
		t.Fatalf("Create by flagged instance admin: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in    orgs.CreateOrgInput
		field string
		msg   string
	}{
		{orgs.CreateOrgInput{Name: "  "}, "name", "Name is required."},
		{orgs.CreateOrgInput{Name: "  "}, "slug", "Slug is required."},
		{orgs.CreateOrgInput{Name: "Acme", Slug: "Bad Slug"}, "slug", "Slug may only contain lowercase letters, numbers, and hyphens."},
		{orgs.CreateOrgInput{Name: "Acme", Slug: "a" + longSlug()}, "slug", "Slug must be 64 characters or fewer."},
	}
	for _, tc := range cases {
		h := newHarness(t)
		_, err := h.svc.Create(ctx, instanceAdmin(adminID), tc.in)
		var v *orgs.ValidationError
		if !errors.As(err, &v) || v.Fields[tc.field] != tc.msg {
			t.Fatalf("Create(%+v) error = %v, want %s=%q", tc.in, err, tc.field, tc.msg)
		}
		if len(h.store.created) != 0 {
			t.Fatal("invalid input reached the store")
		}
	}
}

func longSlug() string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func TestCreateReportsDuplicateOnTheSuppliedField(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.store.createErr = errors.New("UNIQUE constraint failed: organizations.slug")

	_, err := h.svc.Create(ctx, instanceAdmin(adminID), orgs.CreateOrgInput{Name: "Acme", Slug: "acme"})
	var v *orgs.ValidationError
	if !errors.As(err, &v) || v.Fields["slug"] != "An organization with this slug already exists." {
		t.Fatalf("explicit slug duplicate = %v", err)
	}
	_, err = h.svc.Create(ctx, instanceAdmin(adminID), orgs.CreateOrgInput{Name: "Acme"})
	if !errors.As(err, &v) || v.Fields["name"] != "An organization with this name already exists." {
		t.Fatalf("derived slug duplicate = %v", err)
	}
}

func TestUpdateValidation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	var v *orgs.ValidationError

	_, err := h.svc.Update(ctx, account(writerID), testOrg, orgs.UpdateOrgInput{})
	if !errors.As(err, &v) || v.Fields["request"] != "At least one setting is required." {
		t.Fatalf("empty update = %v", err)
	}
	_, err = h.svc.Update(ctx, account(writerID), testOrg, orgs.UpdateOrgInput{Name: ptr("  ")})
	if !errors.As(err, &v) || v.Fields["name"] != "Name must not be empty." {
		t.Fatalf("blank name = %v", err)
	}
	if h.store.updated != 0 {
		t.Fatal("invalid update reached the store")
	}
}

func TestUpdateDisablingSnapshotsPurgesSchemaCaches(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	var observed []int64
	h.svc.OnSchemaCachePurged(func(id int64) { observed = append(observed, id) })

	if _, err := h.svc.Update(ctx, account(writerID), testOrg, orgs.UpdateOrgInput{SchemaSnapshotsEnabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if len(h.store.purged) != 2 || len(observed) != 2 {
		t.Fatalf("purged=%v observed=%v, want both connections", h.store.purged, observed)
	}
	if got := h.actions(); len(got) != 1 || got[0] != orgs.ActionOrgUpdated {
		t.Fatalf("audit = %v", got)
	}

	if _, err := h.svc.Update(ctx, account(writerID), testOrg, orgs.UpdateOrgInput{SchemaSnapshotsEnabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if len(h.store.purged) != 2 {
		t.Fatal("already disabled snapshots were purged again")
	}
}

func TestDeleteInvalidatesPolicyAndAudits(t *testing.T) {
	h := newHarness(t)
	if err := h.svc.Delete(context.Background(), account(ownerID), testOrg); err != nil {
		t.Fatal(err)
	}
	if len(h.store.deleted) != 1 || len(h.cache.orgs) != 1 {
		t.Fatalf("deleted=%v invalidated=%v", h.store.deleted, h.cache.orgs)
	}
	if got := h.actions(); len(got) != 1 || got[0] != orgs.ActionOrgDeleted || h.events[0].ResourceID != "1" {
		t.Fatalf("audit = %+v", h.events)
	}
}

func TestAuditFailureDoesNotFailTheChange(t *testing.T) {
	store := newFakeStore()
	svc := orgs.New(orgs.Deps{
		Store:    store,
		Requirer: access.NewRequirer(fakeDirectory{store: store}, fakePolicy{}),
		Audit:    audit.WriterFunc(func(context.Context, audit.Event) error { return errors.New("audit down") }),
	})
	if err := svc.Delete(context.Background(), account(ownerID), testOrg); err != nil {
		t.Fatalf("Delete with failing audit: %v", err)
	}
}
