package database

import (
	"encoding/json"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetConnection(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("conn-tenant", "Conn Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Conn WS", "")
			assert.Nil(t, err)

			conn, err := db.InsertConnection(ws.ID, tenant.ID, "My Conn", "postgres", "encrypted_dsn_data")
			assert.Nil(t, err)
			assert.Equal(t, conn.Name, "My Conn")
			assert.Equal(t, conn.Driver, "postgres")
			assert.Equal(t, conn.DSN, "encrypted_dsn_data")

			fetched, found, err := db.GetConnection(conn.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.Name, "My Conn")
			assert.Equal(t, fetched.DSN, "encrypted_dsn_data")
		})

		t.Run(driver+": List by workspace", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("conn-list-tenant", "Conn List Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Conn List WS", "")
			assert.Nil(t, err)

			_, err = db.InsertConnection(ws.ID, tenant.ID, "Conn 1", "postgres", "dsn1")
			assert.Nil(t, err)
			_, err = db.InsertConnection(ws.ID, tenant.ID, "Conn 2", "mysql", "dsn2")
			assert.Nil(t, err)

			conns, err := db.GetConnectionsByWorkspace(ws.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(conns), 2)
		})

		t.Run(driver+": Delete connection", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("conn-del-tenant", "Conn Del Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Conn Del WS", "")
			assert.Nil(t, err)

			conn, err := db.InsertConnection(ws.ID, tenant.ID, "Delete Me", "sqlite", "dsn")
			assert.Nil(t, err)

			err = db.DeleteConnection(conn.ID)
			assert.Nil(t, err)

			_, found, err := db.GetConnection(conn.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})

		t.Run(driver+": Workspace isolation", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("conn-iso-tenant", "Conn Iso Tenant")
			assert.Nil(t, err)

			ws1, err := db.InsertWorkspace(tenant.ID, "WS1", "")
			assert.Nil(t, err)
			ws2, err := db.InsertWorkspace(tenant.ID, "WS2", "")
			assert.Nil(t, err)

			_, err = db.InsertConnection(ws1.ID, tenant.ID, "WS1 Conn", "postgres", "dsn1")
			assert.Nil(t, err)
			_, err = db.InsertConnection(ws2.ID, tenant.ID, "WS2 Conn", "postgres", "dsn2")
			assert.Nil(t, err)

			conns1, err := db.GetConnectionsByWorkspace(ws1.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(conns1), 1)
			assert.Equal(t, conns1[0].Name, "WS1 Conn")

			conns2, err := db.GetConnectionsByWorkspace(ws2.ID)
			assert.Nil(t, err)
			assert.Equal(t, len(conns2), 1)
			assert.Equal(t, conns2[0].Name, "WS2 Conn")
		})
	}
}

func TestConnectionDSNAbsentFromJSON(t *testing.T) {
	conn := Connection{
		ID:          "test",
		WorkspaceID: "ws",
		TenantID:    "tenant",
		Name:        "Test Conn",
		Driver:      "postgres",
		DSN:         "secret_dsn",
	}

	data, err := json.Marshal(conn)
	assert.Nil(t, err)

	var m map[string]any
	err = json.Unmarshal(data, &m)
	assert.Nil(t, err)

	_, hasDSN := m["dsn"]
	assert.False(t, hasDSN)
}

func TestConnectionCascadeFromWorkspace(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Workspace delete cascades to connections", func(t *testing.T) {
			db := newTestDB(t, driver)

			tenant, err := db.InsertTenant("conn-cascade-tenant", "Conn Cascade Tenant")
			assert.Nil(t, err)

			ws, err := db.InsertWorkspace(tenant.ID, "Cascade WS", "")
			assert.Nil(t, err)

			conn, err := db.InsertConnection(ws.ID, tenant.ID, "Cascade Conn", "postgres", "dsn")
			assert.Nil(t, err)

			err = db.DeleteWorkspace(ws.ID)
			assert.Nil(t, err)

			_, found, err := db.GetConnection(conn.ID)
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}
