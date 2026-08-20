package database

import (
	"context"
	"testing"
)

func seedQueryFavoritesFixtures(t *testing.T, db *DB) (Workspace, Account) {
	t.Helper()

	org, err := db.InsertOrg(context.Background(), "query-favorites-test-org", "Query Favorites Test Org")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	pw := "testpw"
	account, err := db.InsertAccount(context.Background(), "favorites-fixture@example.com", "Favorites Fixture", &pw)
	if err != nil {
		t.Fatal(err)
	}

	return ws, account
}

func TestQueryFavorites_CreateListUpdateDelete(t *testing.T) {
	db := newTestDB(t)
	ws, account := seedQueryFavoritesFixtures(t, db)

	created, err := db.CreateQueryFavorite(context.Background(), QueryFavorite{
		WorkspaceID: ws.ID,
		AccountID:   account.ID,
		Name:        "Top customers",
		SQL:         "select * from customers limit 10",
	})
	if err != nil {
		t.Fatalf("CreateQueryFavorite: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero favorite ID")
	}

	list, err := db.ListQueryFavorites(context.Background(), ws.ID, account.ID, "")
	if err != nil {
		t.Fatalf("ListQueryFavorites: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("expected 1 favorite matching created id, got %+v", list)
	}

	updated, found, err := db.UpdateQueryFavorite(context.Background(), created.ID, ws.ID, account.ID, "Top 10 customers", "select * from customers limit 10")
	if err != nil {
		t.Fatalf("UpdateQueryFavorite: %v", err)
	}
	if !found || updated.Name != "Top 10 customers" {
		t.Fatalf("expected updated favorite, got found=%v %+v", found, updated)
	}

	deleted, err := db.DeleteQueryFavorite(context.Background(), created.ID, ws.ID, account.ID)
	if err != nil {
		t.Fatalf("DeleteQueryFavorite: %v", err)
	}
	if !deleted {
		t.Fatalf("expected delete to report found")
	}

	list, err = db.ListQueryFavorites(context.Background(), ws.ID, account.ID, "")
	if err != nil {
		t.Fatalf("ListQueryFavorites after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 favorites after delete, got %d", len(list))
	}
}

func TestListQueryFavorites_FiltersBySearch(t *testing.T) {
	db := newTestDB(t)
	ws, account := seedQueryFavoritesFixtures(t, db)

	if _, err := db.CreateQueryFavorite(context.Background(), QueryFavorite{
		WorkspaceID: ws.ID,
		AccountID:   account.ID,
		Name:        "Top customers",
		SQL:         "select * from customers limit 10",
	}); err != nil {
		t.Fatalf("CreateQueryFavorite: %v", err)
	}
	if _, err := db.CreateQueryFavorite(context.Background(), QueryFavorite{
		WorkspaceID: ws.ID,
		AccountID:   account.ID,
		Name:        "Widget audit",
		SQL:         "select * from widgets",
	}); err != nil {
		t.Fatalf("CreateQueryFavorite: %v", err)
	}

	byName, err := db.ListQueryFavorites(context.Background(), ws.ID, account.ID, "CUSTOMERS")
	if err != nil {
		t.Fatalf("ListQueryFavorites: %v", err)
	}
	if len(byName) != 1 || byName[0].Name != "Top customers" {
		t.Fatalf("expected 1 favorite matching name, got %+v", byName)
	}

	bySQL, err := db.ListQueryFavorites(context.Background(), ws.ID, account.ID, "widgets")
	if err != nil {
		t.Fatalf("ListQueryFavorites: %v", err)
	}
	if len(bySQL) != 1 || bySQL[0].Name != "Widget audit" {
		t.Fatalf("expected 1 favorite matching sql_text, got %+v", bySQL)
	}
}

func TestQueryFavorites_ClearAndHasRowsForOrg(t *testing.T) {
	db := newTestDB(t)
	ws, account := seedQueryFavoritesFixtures(t, db)

	_, err := db.CreateQueryFavorite(context.Background(), QueryFavorite{
		WorkspaceID: ws.ID,
		AccountID:   account.ID,
		Name:        "Top customers",
		SQL:         "select 1",
	})
	if err != nil {
		t.Fatalf("CreateQueryFavorite: %v", err)
	}

	hasRows, err := db.QueryFavoritesHasRowsForOrg(context.Background(), *ws.OrgID)
	if err != nil {
		t.Fatalf("QueryFavoritesHasRowsForOrg: %v", err)
	}
	if !hasRows {
		t.Fatalf("expected rows to exist before clearing")
	}

	if err := db.ClearQueryFavoritesForOrg(context.Background(), *ws.OrgID); err != nil {
		t.Fatalf("ClearQueryFavoritesForOrg: %v", err)
	}

	hasRows, err = db.QueryFavoritesHasRowsForOrg(context.Background(), *ws.OrgID)
	if err != nil {
		t.Fatalf("QueryFavoritesHasRowsForOrg: %v", err)
	}
	if hasRows {
		t.Fatalf("expected no rows after clearing")
	}
}
