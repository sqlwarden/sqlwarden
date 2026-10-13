package orgs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/response"
)

const (
	foreignOrg    int64 = 2
	coreTeamID    int64 = 20
	foreignTeamID int64 = 21
)

type fakeTeamStore struct {
	teams        []database.Team
	members      map[int64]map[int64]bool
	workspaceIDs map[int64][]int64
	insertErr    error
	addErr       error

	inserted []string
	updated  []string
	deleted  []int64
	added    [][2]int64
	removed  [][2]int64
	listed   []database.ListTeamMembersParams
}

func newFakeTeamStore() *fakeTeamStore {
	return &fakeTeamStore{
		teams: []database.Team{
			{ID: coreTeamID, OrgID: testOrg, Slug: "core", Name: "Core"},
			{ID: foreignTeamID, OrgID: foreignOrg, Slug: "other", Name: "Other"},
		},
		members:      map[int64]map[int64]bool{coreTeamID: {readerID: true}},
		workspaceIDs: map[int64][]int64{coreTeamID: {70, 71}},
	}
}

func (s *fakeTeamStore) writes() int {
	return len(s.inserted) + len(s.updated) + len(s.deleted) + len(s.added) + len(s.removed)
}

func (s *fakeTeamStore) ListTeamsPage(_ context.Context, params database.ListTeamsParams) (response.Paginated[database.Team], error) {
	var out []database.Team
	for _, team := range s.teams {
		if team.OrgID == params.OrgID {
			out = append(out, team)
		}
	}
	return response.Paginated[database.Team]{Items: out}, nil
}

func (s *fakeTeamStore) InsertTeam(_ context.Context, orgID int64, slug, name string) (database.Team, error) {
	if s.insertErr != nil {
		return database.Team{}, s.insertErr
	}
	s.inserted = append(s.inserted, slug)
	return database.Team{ID: 99, OrgID: orgID, Slug: slug, Name: name}, nil
}

func (s *fakeTeamStore) GetTeam(_ context.Context, orgID int64, slug string) (database.Team, bool, error) {
	for _, team := range s.teams {
		if team.OrgID == orgID && team.Slug == slug {
			return team, true, nil
		}
	}
	return database.Team{}, false, nil
}

func (s *fakeTeamStore) UpdateTeam(_ context.Context, _, _ int64, name string) error {
	s.updated = append(s.updated, name)
	return nil
}

func (s *fakeTeamStore) DeleteTeam(_ context.Context, id, _ int64) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *fakeTeamStore) ListTeamMembersPage(_ context.Context, params database.ListTeamMembersParams) (response.Paginated[database.TeamMemberListItem], error) {
	s.listed = append(s.listed, params)
	return response.Paginated[database.TeamMemberListItem]{}, nil
}

func (s *fakeTeamStore) AddTeamMember(_ context.Context, teamID, accountID int64) error {
	if s.addErr != nil {
		return s.addErr
	}
	s.added = append(s.added, [2]int64{teamID, accountID})
	return nil
}

func (s *fakeTeamStore) RemoveTeamMember(_ context.Context, teamID, accountID int64) error {
	s.removed = append(s.removed, [2]int64{teamID, accountID})
	return nil
}

func (s *fakeTeamStore) ListWorkspaceIDsForTeam(_ context.Context, teamID int64) ([]int64, error) {
	return s.workspaceIDs[teamID], nil
}

// teamCalls runs every team method against slug.
func teamCalls(h *harness, slug string) map[string]func(access.Principal, int64) error {
	ctx := context.Background()
	return map[string]func(access.Principal, int64) error{
		"ListTeams": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListTeams(ctx, p, org, database.ListTeamsParams{}))
		},
		"CreateTeam": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.CreateTeam(ctx, p, org, orgs.CreateTeamInput{Slug: "new-team", Name: "New"}))
		},
		"GetTeam": func(p access.Principal, org int64) error { return accesstest.Err(h.svc.GetTeam(ctx, p, org, slug)) },
		"UpdateTeam": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.UpdateTeam(ctx, p, org, slug, orgs.UpdateTeamInput{Name: "Renamed"}))
		},
		"DeleteTeam": func(p access.Principal, org int64) error { return accesstest.Err(h.svc.DeleteTeam(ctx, p, org, slug)) },
		"ListTeamMembers": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListTeamMembers(ctx, p, org, slug, database.ListTeamMembersParams{}))
		},
		"AddTeamMember": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.AddTeamMember(ctx, p, org, slug, writerID))
		},
		"RemoveTeamMember": func(p access.Principal, org int64) error {
			_, _, err := h.svc.RemoveTeamMember(ctx, p, org, slug, readerID)
			return err
		},
	}
}

func TestTeamMethodsDenyNonMembers(t *testing.T) {
	h := newHarness(t)
	for name, call := range teamCalls(h, "core") {
		accesstest.AssertForbidden(t, call(account(nonMemberID), testOrg), name)
	}
	if h.teams.writes() != 0 {
		t.Fatal("a denied team call changed the store")
	}
}

func TestTeamMethodsDenyNonAccountPrincipals(t *testing.T) {
	p := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectServiceAccount, ID: ownerID}}
	for name, call := range teamCalls(newHarness(t), "core") {
		accesstest.AssertForbidden(t, call(p, testOrg), name)
	}
}

func TestTeamMethodsReportMissingOrgAsNotFound(t *testing.T) {
	for name, call := range teamCalls(newHarness(t), "core") {
		accesstest.AssertNotFound(t, call(account(ownerID), missingOrg), name)
	}
}

func TestTeamWritesDenyReaders(t *testing.T) {
	for _, name := range []string{"CreateTeam", "UpdateTeam", "DeleteTeam", "AddTeamMember", "RemoveTeamMember"} {
		h := newHarness(t)
		accesstest.AssertForbidden(t, teamCalls(h, "core")[name](account(readerID), testOrg), name)
		if h.teams.writes() != 0 || len(h.revoker.workspaceCalls) != 0 {
			t.Fatalf("%s by a reader changed the store", name)
		}
	}
	h := newHarness(t)
	for _, name := range []string{"ListTeams", "GetTeam", "ListTeamMembers"} {
		accesstest.AssertForbidden(t, teamCalls(h, "core")[name](account(adminID), testOrg), name)
	}
}

func TestTeamMethodsAllowPermittedMembers(t *testing.T) {
	allowed := map[string]int64{
		"ListTeams": readerID, "GetTeam": readerID, "ListTeamMembers": readerID,
		"CreateTeam": writerID, "UpdateTeam": writerID, "DeleteTeam": writerID,
		"AddTeamMember": writerID, "RemoveTeamMember": writerID,
	}
	for name, caller := range allowed {
		if err := teamCalls(newHarness(t), "core")[name](account(caller), testOrg); err != nil {
			t.Fatalf("%s by %d: %v", name, caller, err)
		}
	}
}

func TestTeamOfAnotherOrgIsNotFound(t *testing.T) {
	h := newHarness(t)
	for name, call := range teamCalls(h, "other") {
		if name == "ListTeams" || name == "CreateTeam" {
			continue
		}
		accesstest.AssertNotFound(t, call(account(ownerID), testOrg), name)
	}
	if h.teams.writes() != 0 {
		t.Fatal("a foreign team was changed")
	}
}

func TestListTeamsForcesOrgScope(t *testing.T) {
	h := newHarness(t)
	page, err := h.svc.ListTeams(context.Background(), account(readerID), testOrg, database.ListTeamsParams{OrgID: foreignOrg})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != coreTeamID {
		t.Fatalf("items = %+v, want only the org's team", page.Items)
	}
}

func TestListTeamMembersScopesToResolvedTeam(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.ListTeamMembers(context.Background(), account(readerID), testOrg, "core", database.ListTeamMembersParams{TeamID: foreignTeamID}); err != nil {
		t.Fatal(err)
	}
	if len(h.teams.listed) != 1 || h.teams.listed[0].TeamID != coreTeamID {
		t.Fatalf("listed = %+v", h.teams.listed)
	}
}

func TestCreateTeamValidation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		in    orgs.CreateTeamInput
		field string
		want  string
	}{
		{orgs.CreateTeamInput{Slug: "ok"}, "name", "Name is required."},
		{orgs.CreateTeamInput{Name: "Ok"}, "slug", "Slug is required."},
		{orgs.CreateTeamInput{Name: "Ok", Slug: "Not Valid"}, "slug", "Slug may only contain lowercase letters, numbers, and hyphens."},
	}
	for _, tc := range cases {
		h := newHarness(t)
		_, err := h.svc.CreateTeam(ctx, account(writerID), testOrg, tc.in)
		var v *orgs.ValidationError
		if !errors.As(err, &v) || v.Fields[tc.field] != tc.want {
			t.Fatalf("CreateTeam(%+v) = %v, want %s: %s", tc.in, err, tc.field, tc.want)
		}
		if h.teams.writes() != 0 {
			t.Fatal("invalid team was inserted")
		}
	}
}

func TestCreateTeamDuplicateSlug(t *testing.T) {
	h := newHarness(t)
	h.teams.insertErr = errors.New("UNIQUE constraint failed: teams.org_id, teams.slug")
	_, err := h.svc.CreateTeam(context.Background(), account(writerID), testOrg, orgs.CreateTeamInput{Slug: "core", Name: "Core"})
	var v *orgs.ValidationError
	if !errors.As(err, &v) || v.Fields["slug"] != "A team with this slug already exists in this organization." {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateTeamRejectsImmutableFields(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.UpdateTeam(context.Background(), account(writerID), testOrg, "core", orgs.UpdateTeamInput{Name: "X", Slug: ptr("x"), OrgID: ptr(foreignOrg)})
	var v *orgs.ValidationError
	if !errors.As(err, &v) || v.Fields["slug"] != "Slug is immutable." || v.Fields["org_id"] != "Organization is immutable." {
		t.Fatalf("err = %v", err)
	}
	if h.teams.writes() != 0 {
		t.Fatal("team updated despite validation failure")
	}
}

func TestDeleteTeamInvalidatesOrgPolicy(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.DeleteTeam(context.Background(), account(writerID), testOrg, "core"); err != nil {
		t.Fatal(err)
	}
	if len(h.teams.deleted) != 1 || h.teams.deleted[0] != coreTeamID {
		t.Fatalf("deleted = %v", h.teams.deleted)
	}
	if len(h.cache.orgs) != 1 || h.cache.orgs[0] != testOrg {
		t.Fatalf("invalidated orgs = %v", h.cache.orgs)
	}
}

func TestAddTeamMemberRequiresOrgMembership(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.AddTeamMember(context.Background(), account(writerID), testOrg, "core", nonMemberID)
	accesstest.AssertNotFound(t, err, "AddTeamMember of a non-member")
	if h.teams.writes() != 0 {
		t.Fatal("non-member added to team")
	}

	_, err = h.svc.AddTeamMember(context.Background(), account(writerID), testOrg, "core", 0)
	var v *orgs.ValidationError
	if !errors.As(err, &v) || v.Fields["account_id"] != "Account is required." {
		t.Fatalf("err = %v", err)
	}
}

func TestAddTeamMemberInvalidatesPrincipal(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.AddTeamMember(context.Background(), account(writerID), testOrg, "core", readerID); err != nil {
		t.Fatal(err)
	}
	if len(h.cache.principals) != 1 || h.cache.principals[0] != [2]int64{testOrg, readerID} {
		t.Fatalf("principals = %v", h.cache.principals)
	}
}

func TestAddTeamMemberStoreViolations(t *testing.T) {
	h := newHarness(t)
	h.teams.addErr = errors.New("UNIQUE constraint failed: team_members.team_id, team_members.account_id")
	if _, err := h.svc.AddTeamMember(context.Background(), account(writerID), testOrg, "core", readerID); err != nil {
		t.Fatalf("existing member: %v", err)
	}
	h.teams.addErr = errors.New("FOREIGN KEY constraint failed")
	_, err := h.svc.AddTeamMember(context.Background(), account(writerID), testOrg, "core", readerID)
	accesstest.AssertNotFound(t, err, "foreign key violation")
}

func TestRemoveTeamMemberRevokesWorkspaceSessions(t *testing.T) {
	h := newHarness(t)
	h.revoker.err = errors.New("runtime down")
	_, affected, err := h.svc.RemoveTeamMember(context.Background(), account(writerID), testOrg, "core", readerID)
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}
	want := [][2]string{{"70", "8"}, {"71", "8"}}
	if len(h.revoker.workspaceCalls) != 2 || h.revoker.workspaceCalls[0] != want[0] || h.revoker.workspaceCalls[1] != want[1] {
		t.Fatalf("workspace revocations = %v", h.revoker.workspaceCalls)
	}
	if len(h.cache.principals) != 1 || h.cache.principals[0] != [2]int64{testOrg, readerID} {
		t.Fatalf("principals = %v", h.cache.principals)
	}
}
