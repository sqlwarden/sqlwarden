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

	byID, ok, err := db.GetTeamByID(context.Background(), team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected team lookup by ID to succeed")
	}
	if byID.Slug != team.Slug {
		t.Fatalf("team slug mismatch: got %q want %q", byID.Slug, team.Slug)
	}

	teams, err := db.ListTeamsPage(context.Background(), ListTeamsParams{OrgID: org.ID, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(teams.Items) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams.Items))
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

func TestGetAccountTeamsAndDeleteTeam(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "account-teams-"+driver, "Account Teams")
			if err != nil {
				t.Fatal(err)
			}
			teamA, err := db.InsertTeam(ctx, org.ID, "devs", "Developers")
			if err != nil {
				t.Fatal(err)
			}
			teamB, err := db.InsertTeam(ctx, org.ID, "ops", "Operations")
			if err != nil {
				t.Fatal(err)
			}

			if err := db.AddTeamMember(ctx, teamA.ID, testUsers["alice"].id); err != nil {
				t.Fatal(err)
			}
			if err := db.AddTeamMember(ctx, teamB.ID, testUsers["alice"].id); err != nil {
				t.Fatal(err)
			}

			teams, err := db.GetAccountTeams(ctx, org.ID, testUsers["alice"].id)
			if err != nil {
				t.Fatal(err)
			}
			if len(teams) != 2 {
				t.Fatalf("expected 2 account teams, got %d", len(teams))
			}

			if err := db.DeleteTeam(ctx, teamA.ID, org.ID); err != nil {
				t.Fatal(err)
			}

			_, found, err := db.GetTeamByID(ctx, teamA.ID)
			if err != nil {
				t.Fatal(err)
			}
			if found {
				t.Fatal("expected deleted team lookup to miss")
			}

			teams, err = db.GetAccountTeams(ctx, org.ID, testUsers["alice"].id)
			if err != nil {
				t.Fatal(err)
			}
			if len(teams) != 1 || teams[0].ID != teamB.ID {
				t.Fatalf("expected only remaining team after delete, got %+v", teams)
			}
		})
	}
}

func TestListTeams_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "list-teams-"+driver, "List Teams")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.InsertTeam(ctx, org.ID, "alpha", "Alpha Team"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.InsertTeam(ctx, org.ID, "zeta", "Zeta Team"); err != nil {
				t.Fatal(err)
			}

			result, err := db.ListTeamsPage(ctx, ListTeamsParams{
				OrgID:    org.ID,
				Search:   "team",
				Slug:     "zeta",
				Sort:     "name",
				Order:    "desc",
				Page:     1,
				PageSize: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 1 {
				t.Fatalf("expected total=1, got %d", result.Total)
			}
			if len(result.Items) != 1 {
				t.Fatalf("expected 1 team, got %d", len(result.Items))
			}
			if result.Items[0].Name != "Zeta Team" {
				t.Fatalf("expected Zeta Team, got %s", result.Items[0].Name)
			}
		})
	}
}
