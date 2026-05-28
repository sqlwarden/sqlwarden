package database

import (
	"context"
	"testing"
)

func TestIsEffectiveWorkspaceMemberIncludesTeams(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "membership-"+driver, "Membership")
			if err != nil {
				t.Fatal(err)
			}
			memberID := testUsers["bob"].id
			if err := db.AddOrgMember(ctx, org.ID, memberID); err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}
			team, err := db.InsertTeam(ctx, org.ID, "developers", "Developers")
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AddTeamMember(ctx, team.ID, memberID); err != nil {
				t.Fatal(err)
			}
			if err := db.AddWorkspaceTeam(ctx, ws.ID, team.ID, nil); err != nil {
				t.Fatal(err)
			}
			member, err := db.IsEffectiveWorkspaceMember(ctx, org.ID, ws.ID, memberID)
			if err != nil {
				t.Fatal(err)
			}
			if !member {
				t.Fatal("expected team-derived workspace membership")
			}
		})
	}
}
