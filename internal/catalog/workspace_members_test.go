package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
)

func TestAddWorkspaceMemberRequiresOrgMember(t *testing.T) {
	f := workspaceFixture()
	ctx := context.Background()
	var validation *ValidationError

	if err := f.service.AddWorkspaceMember(ctx, f.principal, 1, 10, 0); !errors.As(err, &validation) || validation.Fields["account_id"] != "Account is required." {
		t.Fatalf("error = %v, want account validation", err)
	}
	accesstest.AssertNotFound(t, f.service.AddWorkspaceMember(ctx, f.principal, 1, 10, 42), "non org member")
	if len(f.workspaces.members) != 0 {
		t.Fatal("non org member was added")
	}

	f.store.members[[2]int64{1, 42}] = true
	if err := f.service.AddWorkspaceMember(ctx, f.principal, 1, 10, 42); err != nil {
		t.Fatal(err)
	}
	if !f.workspaces.members[[2]int64{10, 42}] || f.workspaces.addMemberCreator == nil || *f.workspaces.addMemberCreator != 7 {
		t.Fatalf("members=%v creator=%v", f.workspaces.members, f.workspaces.addMemberCreator)
	}
	if len(f.cache.principals) != 1 || f.cache.principals[0] != [2]int64{1, 42} {
		t.Fatalf("cache = %+v", f.cache.principals)
	}
}

func TestRemoveWorkspaceMemberRevokesSessions(t *testing.T) {
	f := workspaceFixture()
	f.workspaces.members[[2]int64{10, 42}] = true
	if err := f.service.RemoveWorkspaceMember(context.Background(), f.principal, 1, 10, 42); err != nil {
		t.Fatal(err)
	}
	if f.workspaces.members[[2]int64{10, 42}] {
		t.Fatal("member was not removed")
	}
	if len(f.revoker.workspaceRevokes) != 1 || f.revoker.workspaceRevokes[0] != [2]string{"10", "42"} {
		t.Fatalf("revokes = %v", f.revoker.workspaceRevokes)
	}
	if len(f.cache.principals) != 1 || f.cache.principals[0] != [2]int64{1, 42} {
		t.Fatalf("cache = %+v", f.cache.principals)
	}
}

func TestRemoveWorkspaceMemberRevocationFailureDoesNotFailRemoval(t *testing.T) {
	f := workspaceFixture()
	f.revoker.workspaceErr = errors.New("runtime unavailable")
	if err := f.service.RemoveWorkspaceMember(context.Background(), f.principal, 1, 10, 42); err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestAddWorkspaceTeamRequiresOrgTeamAndRevokesMembers(t *testing.T) {
	f := workspaceFixture()
	ctx := context.Background()
	var validation *ValidationError
	f.workspaces.teams[5] = database.Team{ID: 5, OrgID: 1}
	f.workspaces.teams[6] = database.Team{ID: 6, OrgID: 2}
	f.workspaces.teamMembers[5] = []int64{42, 43}

	if _, err := f.service.AddWorkspaceTeam(ctx, f.principal, 1, 10, 0); !errors.As(err, &validation) || validation.Fields["team_id"] != "Team is required." {
		t.Fatalf("error = %v, want team validation", err)
	}
	for _, teamID := range []int64{6, 99} {
		if _, err := f.service.AddWorkspaceTeam(ctx, f.principal, 1, 10, teamID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("team %d error = %v, want ErrNotFound", teamID, err)
		}
	}

	affected, err := f.service.AddWorkspaceTeam(ctx, f.principal, 1, 10, 5)
	if err != nil || affected != 2 {
		t.Fatalf("AddWorkspaceTeam = %d, %v", affected, err)
	}
	if len(f.revoker.workspaceRevokes) != 2 || len(f.cache.principals) != 2 {
		t.Fatalf("revokes=%v cache=%v", f.revoker.workspaceRevokes, f.cache.principals)
	}
}

func TestRemoveWorkspaceTeamRevokesMembersCollectedBeforeRemoval(t *testing.T) {
	f := workspaceFixture()
	f.workspaces.workspaceTeams[[2]int64{10, 5}] = true
	f.workspaces.teamMembers[5] = []int64{42, 43}

	affected, err := f.service.RemoveWorkspaceTeam(context.Background(), f.principal, 1, 10, 5)
	if err != nil || affected != 2 {
		t.Fatalf("RemoveWorkspaceTeam = %d, %v", affected, err)
	}
	if f.workspaces.workspaceTeams[[2]int64{10, 5}] {
		t.Fatal("team was not removed")
	}
	want := [][2]string{{"10", "42"}, {"10", "43"}}
	if len(f.revoker.workspaceRevokes) != 2 || f.revoker.workspaceRevokes[0] != want[0] || f.revoker.workspaceRevokes[1] != want[1] {
		t.Fatalf("revokes = %v", f.revoker.workspaceRevokes)
	}
}

func TestListWorkspaceMembersScopesToWorkspace(t *testing.T) {
	f := workspaceFixture()
	_, err := f.service.ListWorkspaceMembers(context.Background(), f.principal, 1, 10, database.ListWorkspaceMembersParams{WorkspaceID: 11, Page: 1, PageSize: 25})
	if err != nil || f.workspaces.memberParams.WorkspaceID != 10 {
		t.Fatalf("ListWorkspaceMembers params = %+v, %v", f.workspaces.memberParams, err)
	}
}
