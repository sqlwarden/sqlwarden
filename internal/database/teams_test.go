package database

import (
	"testing"
)

func TestTeamCRUD(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg("team-test-org", "Team Test Org")
	if err != nil {
		t.Fatal(err)
	}

	team, err := db.InsertTeam(org.ID, "eng", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if team.ID == 0 {
		t.Fatal("expected non-zero team ID")
	}

	found, ok, err := db.GetTeam(org.ID, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected team to be found")
	}
	if found.ID != team.ID {
		t.Fatalf("team ID mismatch: got %d, want %d", found.ID, team.ID)
	}

	teams, err := db.ListTeams(org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
}

func TestTeamMembership(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg("team-member-org", "Team Member Org")
	team, _ := db.InsertTeam(org.ID, "devs", "Devs")

	pw := "pw"
	acc, _ := db.InsertAccount("dev@example.com", "Dev", &pw)

	err := db.AddTeamMember(team.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	members, err := db.ListTeamMembers(team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].AccountID != acc.ID {
		t.Fatalf("expected 1 member with account_id %d", acc.ID)
	}

	err = db.RemoveTeamMember(team.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	members, _ = db.ListTeamMembers(team.ID)
	if len(members) != 0 {
		t.Fatal("expected no members after removal")
	}
}
