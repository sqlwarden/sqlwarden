package database

import (
	"context"
	"testing"
)

func TestTeamCRUD(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg(context.Background(), "team-test-org", "Team Test Org")
	if err != nil {
		t.Fatal(err)
	}

	team, err := db.InsertTeam(context.Background(), org.ID, "eng", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if team.ID == 0 {
		t.Fatal("expected non-zero team ID")
	}

	found, ok, err := db.GetTeam(context.Background(), org.ID, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected team to be found")
	}
	if found.ID != team.ID {
		t.Fatalf("team ID mismatch: got %d, want %d", found.ID, team.ID)
	}

	teams, err := db.ListTeams(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
}

func TestTeamMembership(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg(context.Background(), "team-member-org", "Team Member Org")
	team, _ := db.InsertTeam(context.Background(), org.ID, "devs", "Devs")

	pw := "pw"
	acc, _ := db.InsertAccount(context.Background(), "dev@example.com", "Dev", &pw)

	err := db.AddTeamMember(context.Background(), team.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	members, err := db.ListTeamMembers(context.Background(), team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].AccountID != acc.ID {
		t.Fatalf("expected 1 member with account_id %d", acc.ID)
	}

	err = db.RemoveTeamMember(context.Background(), team.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	members, _ = db.ListTeamMembers(context.Background(), team.ID)
	if len(members) != 0 {
		t.Fatal("expected no members after removal")
	}
}
