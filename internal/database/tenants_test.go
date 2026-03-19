package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetTenant(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("acme", "Acme Corp")
			assert.Nil(t, err)
			assert.Equal(t, tenant.Slug, "acme")
			assert.Equal(t, tenant.Name, "Acme Corp")

			fetched, found, err := db.GetTenant(tenant.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Slug, "acme")
		})

		t.Run(driver+": Fetch by slug", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("slug-test", "Slug Test")
			assert.Nil(t, err)

			fetched, found, err := db.GetTenantBySlug("slug-test")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.ID, tenant.ID)
		})

		t.Run(driver+": Duplicate slug fails", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, err := db.InsertTenant("unique-slug", "First")
			assert.Nil(t, err)

			_, err = db.InsertTenant("unique-slug", "Second")
			assert.NotNil(t, err)
		})

		t.Run(driver+": Non-existent tenant returns not found", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, found, err := db.GetTenant("nonexistent")
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}

func TestTenantMembers(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Add, list, and remove members", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account1, err := db.InsertAccount("member1@example.com", "Member 1", &pw)
			assert.Nil(t, err)
			account2, err := db.InsertAccount("member2@example.com", "Member 2", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("members-test", "Members Test")
			assert.Nil(t, err)

			err = db.AddTenantMember(tenant.ID, account1.ID, "owner")
			assert.Nil(t, err)
			err = db.AddTenantMember(tenant.ID, account2.ID, "member")
			assert.Nil(t, err)

			members, err := db.GetTenantMembers(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 2)

			// IsTenantMember
			isMember, err := db.IsTenantMember(tenant.ID, account1.ID)
			assert.Nil(t, err)
			assert.True(t, isMember)

			// Remove member
			err = db.RemoveTenantMember(tenant.ID, account2.ID)
			assert.Nil(t, err)

			members, err = db.GetTenantMembers(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 1)

			isMember, err = db.IsTenantMember(tenant.ID, account2.ID)
			assert.Nil(t, err)
			assert.False(t, isMember)
		})

		t.Run(driver+": Update member role", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("role-update@example.com", "Role Update", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("role-update", "Role Update")
			assert.Nil(t, err)

			err = db.AddTenantMember(tenant.ID, account.ID, "member")
			assert.Nil(t, err)

			err = db.UpdateTenantMemberRole(tenant.ID, account.ID, "admin")
			assert.Nil(t, err)

			members, err := db.GetTenantMembers(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 1)
			assert.Equal(t, members[0].Role, "admin")
		})

		t.Run(driver+": GetAccountTenants", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("multi-tenant@example.com", "Multi Tenant", &pw)
			assert.Nil(t, err)

			tenant1, err := db.InsertTenant("tenant-a", "Tenant A")
			assert.Nil(t, err)
			tenant2, err := db.InsertTenant("tenant-b", "Tenant B")
			assert.Nil(t, err)

			err = db.AddTenantMember(tenant1.ID, account.ID, "member")
			assert.Nil(t, err)
			err = db.AddTenantMember(tenant2.ID, account.ID, "member")
			assert.Nil(t, err)

			tenants, err := db.GetAccountTenants(account.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(tenants), 2)
		})
	}
}

func TestCountTenantOwners(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Counts only owners", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account1, err := db.InsertAccount("owner1@example.com", "Owner 1", &pw)
			assert.Nil(t, err)
			account2, err := db.InsertAccount("owner2@example.com", "Owner 2", &pw)
			assert.Nil(t, err)
			account3, err := db.InsertAccount("member3@example.com", "Member 3", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("count-owners", "Count Owners")
			assert.Nil(t, err)

			err = db.AddTenantMember(tenant.ID, account1.ID, "owner")
			assert.Nil(t, err)
			err = db.AddTenantMember(tenant.ID, account2.ID, "owner")
			assert.Nil(t, err)
			err = db.AddTenantMember(tenant.ID, account3.ID, "member")
			assert.Nil(t, err)

			count, err := db.CountTenantOwners(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, count, 2)
		})
	}
}

func TestTenantIDPConfig(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Upsert, get, and delete IDP config", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("idp-test", "IDP Test")
			assert.Nil(t, err)

			config, err := db.UpsertTenantIDPConfig(tenant.ID, "saml", "Corp SSO", `{"key":"value"}`)
			assert.Nil(t, err)
			assert.Equal(t, config.Provider, "saml")
			assert.Equal(t, config.DisplayName, "Corp SSO")

			fetched, found, err := db.GetTenantIDPConfig(tenant.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Provider, "saml")

			// Upsert updates existing
			config2, err := db.UpsertTenantIDPConfig(tenant.ID, "oidc", "Updated SSO", `{"key":"new"}`)
			assert.Nil(t, err)
			assert.Equal(t, config2.Provider, "oidc")

			fetched2, found2, err := db.GetTenantIDPConfig(tenant.ID)
			assert.Nil(t, err)
			assert.True(t, found2)
			assert.Equal(t, fetched2.Provider, "oidc")
			assert.Equal(t, fetched2.DisplayName, "Updated SSO")

			// Delete
			err = db.DeleteTenantIDPConfig(tenant.ID)
			assert.Nil(t, err)

			_, found3, err := db.GetTenantIDPConfig(tenant.ID)
			assert.Nil(t, err)
			assert.False(t, found3)
		})

		t.Run(driver+": Config field absent from JSON", func(t *testing.T) {
			config := TenantIDPConfig{
				ID:          "test",
				TenantID:    "tenant",
				Provider:    "saml",
				DisplayName: "Test",
				Config:      "secret_config",
				IsActive:    true,
			}

			data, err := json.Marshal(config)
			assert.Nil(t, err)

			var m map[string]any
			err = json.Unmarshal(data, &m)
			assert.Nil(t, err)

			_, hasConfig := m["config"]
			assert.False(t, hasConfig)
		})
	}
}

func TestTenantCascadeDelete(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Deleting tenant removes members and IDP config", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("cascade@example.com", "Cascade", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("cascade-test", "Cascade Test")
			assert.Nil(t, err)

			err = db.AddTenantMember(tenant.ID, account.ID, "owner")
			assert.Nil(t, err)

			_, err = db.UpsertTenantIDPConfig(tenant.ID, "saml", "Test", `{"config":"data"}`)
			assert.Nil(t, err)

			// Delete tenant via raw SQL (no DeleteTenant method)
			_, err = db.ExecContext(context.Background(), "DELETE FROM tenants WHERE id = ?", tenant.ID)
			assert.Nil(t, err)

			members, err := db.GetTenantMembers(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(members), 0)

			_, found, err := db.GetTenantIDPConfig(tenant.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}
