package database

import (
	"context"
	"testing"
)

func seedQueryHistoryFixtures(t *testing.T, db *DB) (Connection, Account) {
	t.Helper()

	org, err := db.InsertOrg(context.Background(), "query-history-test-org", "Query History Test Org")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(context.Background(), ws.ID, nil, "history-db", "postgres", "encrypted-dsn", "open")
	if err != nil {
		t.Fatal(err)
	}
	pw := "testpw"
	account, err := db.InsertAccount(context.Background(), "history-fixture@example.com", "History Fixture", &pw)
	if err != nil {
		t.Fatal(err)
	}

	return conn, account
}

func TestInsertQueryHistoryEntry_TrimsToRetention(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)

	for i := 0; i < 5; i++ {
		_, err := db.InsertQueryHistoryEntry(context.Background(), QueryHistoryEntry{
			ConnectionID: conn.ID,
			AccountID:    account.ID,
			SQL:          "select 1",
			Status:       "ok",
			DurationMS:   1,
			RowsAffected: 1,
		}, 3)
		if err != nil {
			t.Fatalf("InsertQueryHistoryEntry: %v", err)
		}
	}

	page, err := db.ListQueryHistory(context.Background(), conn.ID, account.ID, "", 1, 25)
	if err != nil {
		t.Fatalf("ListQueryHistory: %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("expected 3 rows retained, got %d", page.Total)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items in page, got %d", len(page.Items))
	}
}

func TestListQueryHistory_FiltersBySearch(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)

	if _, err := db.InsertQueryHistoryEntry(context.Background(), QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    account.ID,
		SQL:          "select * from accounts",
		Status:       "ok",
	}, 200); err != nil {
		t.Fatalf("InsertQueryHistoryEntry: %v", err)
	}
	if _, err := db.InsertQueryHistoryEntry(context.Background(), QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    account.ID,
		SQL:          "select * from widgets",
		Status:       "ok",
	}, 200); err != nil {
		t.Fatalf("InsertQueryHistoryEntry: %v", err)
	}

	page, err := db.ListQueryHistory(context.Background(), conn.ID, account.ID, "WIDGETS", 1, 25)
	if err != nil {
		t.Fatalf("ListQueryHistory: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected 1 matching row, got %+v", page)
	}
	if page.Items[0].SQL != "select * from widgets" {
		t.Fatalf("expected widgets row, got %q", page.Items[0].SQL)
	}
}

func TestListQueryHistoryForWorkspace_AcrossConnections(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)

	otherConn, err := db.InsertConnection(context.Background(), conn.WorkspaceID, nil, "history-db-2", "postgres", "encrypted-dsn", "open")
	if err != nil {
		t.Fatalf("InsertConnection: %v", err)
	}

	otherOrg, err := db.InsertOrg(context.Background(), "other-workspace-history-org", "Other Workspace History Org")
	if err != nil {
		t.Fatalf("InsertOrg: %v", err)
	}
	otherWs, err := db.InsertWorkspace(context.Background(), &otherOrg.ID, "org", otherOrg.ID, "Other", "")
	if err != nil {
		t.Fatalf("InsertWorkspace: %v", err)
	}
	foreignConn, err := db.InsertConnection(context.Background(), otherWs.ID, nil, "foreign-db", "postgres", "encrypted-dsn", "open")
	if err != nil {
		t.Fatalf("InsertConnection: %v", err)
	}

	for _, c := range []Connection{conn, otherConn, foreignConn} {
		if _, err := db.InsertQueryHistoryEntry(context.Background(), QueryHistoryEntry{
			ConnectionID: c.ID,
			AccountID:    account.ID,
			SQL:          "select 1",
			Status:       "ok",
		}, 200); err != nil {
			t.Fatalf("InsertQueryHistoryEntry: %v", err)
		}
	}

	page, err := db.ListQueryHistoryForWorkspace(context.Background(), conn.WorkspaceID, account.ID, nil, "", 1, 25)
	if err != nil {
		t.Fatalf("ListQueryHistoryForWorkspace: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected 2 rows across the workspace's connections, got %d", page.Total)
	}

	filtered, err := db.ListQueryHistoryForWorkspace(context.Background(), conn.WorkspaceID, account.ID, &otherConn.ID, "", 1, 25)
	if err != nil {
		t.Fatalf("ListQueryHistoryForWorkspace: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 {
		t.Fatalf("expected 1 row when filtered to a single connection, got %+v", filtered)
	}
	if filtered.Items[0].ConnectionID != otherConn.ID {
		t.Fatalf("expected row for connection %d, got %d", otherConn.ID, filtered.Items[0].ConnectionID)
	}
}

func TestListQueryHistory_EmptyResultReturnsNonNilItems(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)

	page, err := db.ListQueryHistory(context.Background(), conn.ID, account.ID, "", 1, 25)
	if err != nil {
		t.Fatalf("ListQueryHistory: %v", err)
	}
	if page.Items == nil {
		t.Fatal("expected Items to be a non-nil empty slice, got nil")
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(page.Items))
	}
}

func TestListQueryHistoryForWorkspace_EmptyResultReturnsNonNilItems(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)

	page, err := db.ListQueryHistoryForWorkspace(context.Background(), conn.WorkspaceID, account.ID, &conn.ID, "", 1, 25)
	if err != nil {
		t.Fatalf("ListQueryHistoryForWorkspace: %v", err)
	}
	if page.Items == nil {
		t.Fatal("expected Items to be a non-nil empty slice, got nil")
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(page.Items))
	}
}

func TestDeleteQueryHistoryEntry_NotFoundForOtherAccount(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)
	pw := "testpw"
	other, err := db.InsertAccount(context.Background(), "history-other@example.com", "History Other", &pw)
	if err != nil {
		t.Fatal(err)
	}

	entry, err := db.InsertQueryHistoryEntry(context.Background(), QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    account.ID,
		SQL:          "select 1",
		Status:       "ok",
	}, 200)
	if err != nil {
		t.Fatalf("InsertQueryHistoryEntry: %v", err)
	}

	found, err := db.DeleteQueryHistoryEntry(context.Background(), entry.ID, conn.ID, other.ID)
	if err != nil {
		t.Fatalf("DeleteQueryHistoryEntry: %v", err)
	}
	if found {
		t.Fatalf("expected not found for a different account's entry")
	}

	found, err = db.DeleteQueryHistoryEntry(context.Background(), entry.ID, conn.ID, account.ID)
	if err != nil {
		t.Fatalf("DeleteQueryHistoryEntry: %v", err)
	}
	if !found {
		t.Fatalf("expected delete to report found for the owning account")
	}
}

func TestClearQueryHistoryForOrg(t *testing.T) {
	db := newTestDB(t)
	conn, account := seedQueryHistoryFixtures(t, db)

	_, err := db.InsertQueryHistoryEntry(context.Background(), QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    account.ID,
		SQL:          "select 1",
		Status:       "ok",
	}, 200)
	if err != nil {
		t.Fatalf("InsertQueryHistoryEntry: %v", err)
	}

	ws, ok, err := db.GetWorkspace(context.Background(), conn.WorkspaceID)
	if err != nil || !ok {
		t.Fatalf("GetWorkspace: %v ok=%v", err, ok)
	}

	hasRows, err := db.QueryHistoryHasRowsForOrg(context.Background(), *ws.OrgID)
	if err != nil {
		t.Fatalf("QueryHistoryHasRowsForOrg: %v", err)
	}
	if !hasRows {
		t.Fatalf("expected rows to exist before clearing")
	}

	if err := db.ClearQueryHistoryForOrg(context.Background(), *ws.OrgID); err != nil {
		t.Fatalf("ClearQueryHistoryForOrg: %v", err)
	}

	hasRows, err = db.QueryHistoryHasRowsForOrg(context.Background(), *ws.OrgID)
	if err != nil {
		t.Fatalf("QueryHistoryHasRowsForOrg: %v", err)
	}
	if hasRows {
		t.Fatalf("expected no rows after clearing")
	}
}
