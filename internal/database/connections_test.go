package database

import (
	"testing"
)

func TestConnectionCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg("conn-test-org", "Conn Test Org")
	ws, _ := db.InsertWorkspace(&org.ID, "org", org.ID, "Main", "")

	conn, err := db.InsertConnection(ws.ID, nil, &org.ID, "org", org.ID, "my-db", "postgres", "encrypted-dsn", "open")
	if err != nil {
		t.Fatal(err)
	}
	if conn.ID == 0 {
		t.Fatal("expected non-zero connection ID")
	}

	found, ok, err := db.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected connection to be found")
	}
	if found.Name != "my-db" {
		t.Fatalf("name mismatch: %s", found.Name)
	}

	conns, err := db.ListConnections(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}

	err = db.DeleteConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, _ = db.GetConnection(conn.ID)
	if ok {
		t.Fatal("expected connection to be deleted")
	}
}
