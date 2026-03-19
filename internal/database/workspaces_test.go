package database

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetWorkspace(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("ws-tenant", "WS Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "My Workspace", "A test workspace")
			assert.Nil(t, err)
			assert.Equal(t, ws.Name, "My Workspace")
			assert.Equal(t, ws.Description, "A test workspace")
			assert.Equal(t, ws.TenantID, tenant.ID)

			fetched, found, err := db.GetWorkspace(ws.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Name, "My Workspace")
		})

		t.Run(driver+": List by tenant", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("ws-list-tenant", "WS List Tenant")
			assert.Nil(t, err)

			_, err = db.InsertWorkspace(tenant.ID, "WS 1", "First")
			assert.Nil(t, err)
			_, err = db.InsertWorkspace(tenant.ID, "WS 2", "Second")
			assert.Nil(t, err)

			workspaces, err := db.GetWorkspacesByTenant(tenant.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(workspaces), 2)
		})

		t.Run(driver+": Update workspace", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("ws-update-tenant", "WS Update Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Old Name", "Old Desc")
			assert.Nil(t, err)

			err = db.UpdateWorkspace(ws.ID, "New Name", "New Desc")
			assert.Nil(t, err)

			fetched, found, err := db.GetWorkspace(ws.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Name, "New Name")
			assert.Equal(t, fetched.Description, "New Desc")
		})

		t.Run(driver+": Delete workspace", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("ws-del-tenant", "WS Del Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Delete Me", "")
			assert.Nil(t, err)

			err = db.DeleteWorkspace(ws.ID)
			assert.Nil(t, err)

			_, found, err := db.GetWorkspace(ws.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})

		t.Run(driver+": Tenant isolation", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant1, err := db.InsertTenant("ws-iso-1", "WS Iso 1")
			assert.Nil(t, err)
			tenant2, err := db.InsertTenant("ws-iso-2", "WS Iso 2")
			assert.Nil(t, err)

			_, err = db.InsertWorkspace(tenant1.ID, "T1 Workspace", "")
			assert.Nil(t, err)
			_, err = db.InsertWorkspace(tenant2.ID, "T2 Workspace", "")
			assert.Nil(t, err)

			ws1, err := db.GetWorkspacesByTenant(tenant1.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(ws1), 1)
			assert.Equal(t, ws1[0].Name, "T1 Workspace")

			ws2, err := db.GetWorkspacesByTenant(tenant2.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(ws2), 1)
			assert.Equal(t, ws2[0].Name, "T2 Workspace")
		})
	}
}

func TestWorkspaceCascadeToConnections(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Deleting workspace cascades to connections", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("ws-cascade-tenant", "WS Cascade Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Cascade WS", "")
			assert.Nil(t, err)

			_, err = db.InsertConnection(ws.ID, tenant.ID, "Test Conn", "postgres", "encrypted_dsn")
			assert.Nil(t, err)

			err = db.DeleteWorkspace(ws.ID)
			assert.Nil(t, err)

			conns, err := db.GetConnectionsByWorkspace(ws.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(conns), 0)
		})

		t.Run(driver+": Deleting tenant cascades to workspaces", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("ws-tenant-cascade", "WS Tenant Cascade")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "WS", "")
			assert.Nil(t, err)

			_, err = db.ExecContext(context.Background(), "DELETE FROM tenants WHERE id = ?", tenant.ID)
			assert.Nil(t, err)

			_, found, err := db.GetWorkspace(ws.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}
