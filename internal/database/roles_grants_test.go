package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
)

func TestWorkspaceRoleCRUD(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("role-tenant", "Role Tenant")
			assert.Nil(t, err)

			role, err := db.InsertWorkspaceRole(tenant.ID, "analyst", "Can run queries")
			assert.Nil(t, err)
			assert.Equal(t, role.Name, "analyst")
			assert.Equal(t, role.Description, "Can run queries")

			fetched, found, err := db.GetWorkspaceRole(role.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Name, "analyst")
		})

		t.Run(driver+": Fetch by name", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("role-name-tenant", "Role Name Tenant")
			assert.Nil(t, err)

			role, err := db.InsertWorkspaceRole(tenant.ID, "viewer", "Read-only")
			assert.Nil(t, err)

			fetched, found, err := db.GetWorkspaceRoleByName(tenant.ID, "viewer")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.ID, role.ID)
		})

		t.Run(driver+": List by tenant", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("role-list-tenant", "Role List Tenant")
			assert.Nil(t, err)

			_, err = db.InsertWorkspaceRole(tenant.ID, "role-a", "A")
			assert.Nil(t, err)
			_, err = db.InsertWorkspaceRole(tenant.ID, "role-b", "B")
			assert.Nil(t, err)

			roles, err := db.GetWorkspaceRolesByTenant(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(roles), 2)
		})

		t.Run(driver+": Delete workspace role", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("role-del-tenant", "Role Del Tenant")
			assert.Nil(t, err)

			role, err := db.InsertWorkspaceRole(tenant.ID, "deleteme", "")
			assert.Nil(t, err)

			err = db.DeleteWorkspaceRole(role.ID)
			assert.Nil(t, err)

			_, found, err := db.GetWorkspaceRole(role.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})

		t.Run(driver+": UNIQUE(tenant_id, name) conflict", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("role-dup-tenant", "Role Dup Tenant")
			assert.Nil(t, err)

			_, err = db.InsertWorkspaceRole(tenant.ID, "samename", "First")
			assert.Nil(t, err)

			_, err = db.InsertWorkspaceRole(tenant.ID, "samename", "Second")
			assert.NotNil(t, err)
		})
	}
}

func TestAccessGrantCRUD(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by object", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("grant@example.com", "Granter", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("grant-tenant", "Grant Tenant")
			assert.Nil(t, err)

			grant, err := db.InsertAccessGrant(tenant.ID, "user:alice", "workspace:ws1", "read", fmt.Sprintf("%d", account.ID), nil)
			assert.Nil(t, err)
			assert.Equal(t, grant.Subject, "user:alice")
			assert.Equal(t, grant.Object, "workspace:ws1")
			assert.Equal(t, grant.Action, "read")

			grants, err := db.GetAccessGrantsByObject("workspace:ws1")
			assert.Nil(t, err)
			assert.Equal(t, len(grants), 1)
			assert.Equal(t, grants[0].Subject, "user:alice")
		})

		t.Run(driver+": Delete grant by subject and object", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("grant-del@example.com", "Granter", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("grant-del-tenant", "Grant Del Tenant")
			assert.Nil(t, err)

			_, err = db.InsertAccessGrant(tenant.ID, "user:bob", "workspace:ws2", "write", fmt.Sprintf("%d", account.ID), nil)
			assert.Nil(t, err)

			err = db.DeleteAccessGrant("user:bob", "workspace:ws2")
			assert.Nil(t, err)

			grants, err := db.GetAccessGrantsByObject("workspace:ws2")
			assert.Nil(t, err)
			assert.Equal(t, len(grants), 0)
		})
	}
}

func TestGetExpiredAccessGrants(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Returns only expired grants", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("expired-grant@example.com", "Granter", &pw)
			assert.Nil(t, err)

			tenant, err := db.InsertTenant("expired-grant-tenant", "Expired Grant Tenant")
			assert.Nil(t, err)

			pastTime := time.Now().Add(-1 * time.Hour)
			futureTime := time.Now().Add(24 * time.Hour)

			_, err = db.InsertAccessGrant(tenant.ID, "user:expired", "obj:1", "read", fmt.Sprintf("%d", account.ID), &pastTime)
			assert.Nil(t, err)
			_, err = db.InsertAccessGrant(tenant.ID, "user:valid", "obj:2", "read", fmt.Sprintf("%d", account.ID), &futureTime)
			assert.Nil(t, err)
			_, err = db.InsertAccessGrant(tenant.ID, "user:noexpiry", "obj:3", "read", fmt.Sprintf("%d", account.ID), nil)
			assert.Nil(t, err)

			expired, err := db.GetExpiredAccessGrants()
			assert.Nil(t, err)
			assert.Equal(t, len(expired), 1)
			assert.Equal(t, expired[0].Subject, "user:expired")
		})
	}
}
