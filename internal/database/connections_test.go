package database

import (
	"context"
	"testing"
)

func TestConnectionCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg(context.Background(), "conn-test-org", "Conn Test Org")
	ws, _ := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")

	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "my-db", "postgres", "encrypted-dsn", "open")
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

	conns, err := db.ListConnectionsPage(context.Background(), ListConnectionsParams{WorkspaceID: ws.ID, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(conns.Items) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns.Items))
	}

	env, err := db.InsertEnvironment(context.Background(), ws.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	connInEnv, err := db.InsertConnection(context.Background(), ws.ID, &env.ID, "reporting-db", "postgres", "env-dsn", "open")
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
	if updated.EnvironmentID != conn.EnvironmentID {
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

func TestListConnections_SupportsSearchFilterSortAndPagination(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg(context.Background(), "conn-list-filters", "Conn List Filters")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	envA, err := db.InsertEnvironment(context.Background(), ws.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	envB, err := db.InsertEnvironment(context.Background(), ws.ID, "staging", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.InsertConnection(context.Background(), ws.ID, &envA.ID, "Primary DB", "postgres", "dsn-a", "open"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertConnection(context.Background(), ws.ID, &envB.ID, "Replica DB", "mysql", "dsn-b", "restricted"); err != nil {
		t.Fatal(err)
	}

	result, err := db.ListConnectionsPage(context.Background(), ListConnectionsParams{
		WorkspaceID:   ws.ID,
		Search:        "db",
		EnvironmentID: &envA.ID,
		Driver:        "postgres",
		Sort:          "name",
		Order:         "asc",
		Page:          1,
		PageSize:      10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Name != "Primary DB" {
		t.Fatalf("expected Primary DB, got %s", result.Items[0].Name)
	}
}
