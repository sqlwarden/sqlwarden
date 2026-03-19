package database

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetTeam(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("team-tenant", "Team Tenant")
			assert.Nil(t, err)

			team, err := db.InsertTeam(tenant.ID, "eng", "Engineering")
			assert.Nil(t, err)
			assert.Equal(t, team.Slug, "eng")
			assert.Equal(t, team.Name, "Engineering")
			assert.Equal(t, team.TenantID, tenant.ID)

			fetched, found, err := db.GetTeam(team.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Slug, "eng")
		})

		t.Run(driver+": Fetch by slug", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("slug-tenant", "Slug Tenant")
			assert.Nil(t, err)

			team, err := db.InsertTeam(tenant.ID, "design", "Design")
			assert.Nil(t, err)

			fetched, found, err := db.GetTeamBySlug(tenant.ID, "design")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.ID, team.ID)
		})

		t.Run(driver+": List by tenant", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("list-tenant", "List Tenant")
			assert.Nil(t, err)

			_, err = db.InsertTeam(tenant.ID, "team-a", "Team A")
			assert.Nil(t, err)
			_, err = db.InsertTeam(tenant.ID, "team-b", "Team B")
			assert.Nil(t, err)

			teams, err := db.GetTeamsByTenant(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(teams), 2)
		})

		t.Run(driver+": UNIQUE(tenant_id, slug) conflict", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("dup-slug-tenant", "Dup Slug Tenant")
			assert.Nil(t, err)

			_, err = db.InsertTeam(tenant.ID, "same-slug", "First")
			assert.Nil(t, err)

			_, err = db.InsertTeam(tenant.ID, "same-slug", "Second")
			assert.NotNil(t, err)
		})

		t.Run(driver+": Same slug in different tenants succeeds", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant1, err := db.InsertTenant("tenant-1", "Tenant 1")
			assert.Nil(t, err)
			tenant2, err := db.InsertTenant("tenant-2", "Tenant 2")
			assert.Nil(t, err)

			_, err = db.InsertTeam(tenant1.ID, "shared-slug", "Team in T1")
			assert.Nil(t, err)
			_, err = db.InsertTeam(tenant2.ID, "shared-slug", "Team in T2")
			assert.Nil(t, err)
		})

		t.Run(driver+": Delete team", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("del-tenant", "Del Tenant")
			assert.Nil(t, err)

			team, err := db.InsertTeam(tenant.ID, "to-delete", "Delete Me")
			assert.Nil(t, err)

			err = db.DeleteTeam(team.ID)
			assert.Nil(t, err)

			_, found, err := db.GetTeam(team.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}

func TestTeamMembers(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Add, list, remove team members", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account1, err := db.InsertAccount("tm1@example.com", "TM 1", &pw)
			assert.Nil(t, err)
			account2, err := db.InsertAccount("tm2@example.com", "TM 2", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("tm-tenant", "TM Tenant")
			assert.Nil(t, err)

			team, err := db.InsertTeam(tenant.ID, "tm-team", "TM Team")
			assert.Nil(t, err)

			err = db.AddTeamMember(team.ID, account1.ID)
			assert.Nil(t, err)
			err = db.AddTeamMember(team.ID, account2.ID)
			assert.Nil(t, err)

			members, err := db.GetTeamMembers(team.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 2)

			err = db.RemoveTeamMember(team.ID, account1.ID)
			assert.Nil(t, err)

			members, err = db.GetTeamMembers(team.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 1)
		})

		t.Run(driver+": GetAccountTeams", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("at@example.com", "Account Teams", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("at-tenant", "AT Tenant")
			assert.Nil(t, err)

			team1, err := db.InsertTeam(tenant.ID, "at-team-1", "AT Team 1")
			assert.Nil(t, err)
			team2, err := db.InsertTeam(tenant.ID, "at-team-2", "AT Team 2")
			assert.Nil(t, err)

			err = db.AddTeamMember(team1.ID, account.ID)
			assert.Nil(t, err)
			err = db.AddTeamMember(team2.ID, account.ID)
			assert.Nil(t, err)

			teams, err := db.GetAccountTeams(account.ID, tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(teams), 2)
		})
	}
}

func TestTeamCascadeOnDelete(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Deleting team removes members", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("cascade-tm@example.com", "Cascade TM", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("cascade-tm-tenant", "Cascade TM Tenant")
			assert.Nil(t, err)

			team, err := db.InsertTeam(tenant.ID, "cascade-team", "Cascade Team")
			assert.Nil(t, err)

			err = db.AddTeamMember(team.ID, account.ID)
			assert.Nil(t, err)

			err = db.DeleteTeam(team.ID)
			assert.Nil(t, err)

			members, err := db.GetTeamMembers(team.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 0)
		})

		t.Run(driver+": Deleting tenant cascades to teams", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("cascade-tenant-tm", "Cascade Tenant TM")
			assert.Nil(t, err)

			team, err := db.InsertTeam(tenant.ID, "child-team", "Child Team")
			assert.Nil(t, err)

			_, err = db.ExecContext(context.Background(), "DELETE FROM tenants WHERE id = ?", tenant.ID)
			assert.Nil(t, err)

			_, found, err := db.GetTeam(team.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}
