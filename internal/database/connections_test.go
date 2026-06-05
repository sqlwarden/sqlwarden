package database

import (
	"context"
	"errors"
	"testing"

	"github.com/uptrace/bun"
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

func TestInsertConnectionWithExecutor_RollsBackConnectionAndHierarchy(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t, driver)
			org, err := db.InsertOrg(ctx, "conn-rollback-"+driver, "Connection Rollback "+driver)
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}

			sentinel := errors.New("abort connection insert")
			var conn Connection
			err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				var err error
				conn, err = db.InsertConnectionWithExecutor(ctx, tx, ws.ID, nil, "Rolled Back", "postgres", "dsn", "open")
				if err != nil {
					return err
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected sentinel rollback error, got %v", err)
			}

			if got := countTableRows(t, db, "connections", "workspace_id = ?", ws.ID); got != 0 {
				t.Fatalf("expected connection insert to roll back, got %d rows", got)
			}
			if conn.ID != 0 {
				if got := countTableRows(t, db, "resource_hierarchy", "child_type = 'connection' AND child_id = ?", conn.ID); got != 0 {
					t.Fatalf("expected connection hierarchy to roll back, got %d rows", got)
				}
			}
		})
	}
}

func TestDeleteConnection_RemovesHierarchyAtomically(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t, driver)
			org, err := db.InsertOrg(ctx, "conn-delete-tx-"+driver, "Connection Delete "+driver)
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.InsertConnection(ctx, ws.ID, nil, "Primary", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}
			if got := countTableRows(t, db, "resource_hierarchy", "child_type = 'connection' AND child_id = ?", conn.ID); got != 1 {
				t.Fatalf("expected connection hierarchy before delete, got %d", got)
			}

			if err = db.DeleteConnection(ctx, conn.ID); err != nil {
				t.Fatal(err)
			}
			if got := countTableRows(t, db, "connections", "id = ?", conn.ID); got != 0 {
				t.Fatalf("expected connection to be deleted, got %d rows", got)
			}
			if got := countTableRows(t, db, "resource_hierarchy", "child_type = 'connection' AND child_id = ?", conn.ID); got != 0 {
				t.Fatalf("expected connection hierarchy to be deleted, got %d rows", got)
			}
		})
	}
}
