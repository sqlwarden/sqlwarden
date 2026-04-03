package database

import (
	"context"
	"testing"
)

func TestConnectionCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg(context.Background(), "conn-test-org", "Conn Test Org")
	ws, _ := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")

	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, &org.ID, "org", org.ID, "my-db", "postgres", "encrypted-dsn", "open")
	if err != nil {
		t.Fatal(err)
	}
	if conn.ID == 0 {
		t.Fatal("expected non-zero connection ID")
	}

	found, ok, err := db.GetConnection(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected connection to be found")
	}
	if found.Name != "my-db" {
		t.Fatalf("name mismatch: %s", found.Name)
	}

	conns, err := db.ListConnections(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}

	env, err := db.InsertEnvironment(context.Background(), ws.ID, &org.ID, "org", org.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	connInEnv, err := db.InsertConnection(context.Background(), ws.ID, &env.ID, &org.ID, "org", org.ID, "reporting-db", "postgres", "env-dsn", "open")
	if err != nil {
		t.Fatal(err)
	}

	ids, err := db.ListConnectionIDsByEnvironment(context.Background(), env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != connInEnv.ID {
		t.Fatalf("expected only env-tagged connection ID %d, got %v", connInEnv.ID, ids)
	}

	err = db.UpdateConnection(context.Background(), conn.ID, "my-db-updated", "new-encrypted-dsn", "restricted")
	if err != nil {
		t.Fatal(err)
	}

	updated, ok, err := db.GetConnection(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected updated connection to be found")
	}
	if updated.Name != "my-db-updated" {
		t.Fatalf("expected updated name, got %s", updated.Name)
	}
	if updated.Driver != "postgres" {
		t.Fatalf("expected original driver to remain unchanged, got %s", updated.Driver)
	}
	if updated.AccessMode != "restricted" {
		t.Fatalf("expected updated access mode, got %s", updated.AccessMode)
	}
	if updated.EnvironmentID != nil {
		t.Fatal("expected environment_id to remain unchanged")
	}

	err = db.DeleteConnection(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, _ = db.GetConnection(context.Background(), conn.ID)
	if ok {
		t.Fatal("expected connection to be deleted")
	}

	err = db.DeleteConnection(context.Background(), connInEnv.ID)
	if err != nil {
		t.Fatal(err)
	}
}
