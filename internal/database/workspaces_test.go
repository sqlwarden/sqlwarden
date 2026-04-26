package database

import (
	"context"
	"testing"
)

func TestWorkspaceCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg(context.Background(), "ws-test-org", "WS Test Org")

	ws, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Production", "prod workspace")
	if err != nil {
		t.Fatal(err)
	}
	if ws.ID == 0 {
		t.Fatal("expected non-zero workspace ID")
	}

	found, ok, err := db.GetWorkspace(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected workspace to be found")
	}
	if found.Name != "Production" {
		t.Fatalf("name mismatch: %s", found.Name)
	}

	wss, err := db.ListWorkspacesPage(context.Background(), ListWorkspacesParams{
		OwnerType: "org",
		OwnerID:   org.ID,
		Page:      1,
		PageSize:  25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(wss.Items) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(wss.Items))
	}
	if wss.Items[0].EnvironmentCount != 1 {
		t.Fatalf("expected default environment count 1, got %d", wss.Items[0].EnvironmentCount)
	}
	if wss.Items[0].ConnectionCount != 0 {
		t.Fatalf("expected connection count 0, got %d", wss.Items[0].ConnectionCount)
	}

	env, err := db.InsertEnvironment(context.Background(), ws.ID, "Staging", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.InsertConnection(context.Background(), ws.ID, &env.ID, "Primary", "postgres", "dsn", "open"); err != nil {
		t.Fatal(err)
	}

	wss, err = db.ListWorkspacesPage(context.Background(), ListWorkspacesParams{
		OwnerType: "org",
		OwnerID:   org.ID,
		Page:      1,
		PageSize:  25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if wss.Items[0].EnvironmentCount != 2 {
		t.Fatalf("expected environment count 2, got %d", wss.Items[0].EnvironmentCount)
	}
	if wss.Items[0].ConnectionCount != 1 {
		t.Fatalf("expected connection count 1, got %d", wss.Items[0].ConnectionCount)
	}

	err = db.UpdateWorkspace(context.Background(), ws.ID, "Production Updated", "updated description")
	if err != nil {
		t.Fatal(err)
	}

	found, ok, err = db.GetWorkspace(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected updated workspace to exist")
	}
	if found.Name != "Production Updated" || found.Description != "updated description" {
		t.Fatalf("unexpected updated workspace: %+v", found)
	}

	err = db.DeleteWorkspace(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, _ = db.GetWorkspace(context.Background(), ws.ID)
	if ok {
		t.Fatal("expected workspace to be deleted")
	}
}

func TestListWorkspaces_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg(context.Background(), "ws-search-org", "WS Search Org")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Data Lake", "Analytics"} {
		if _, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, name, ""); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.ListWorkspacesPage(context.Background(), ListWorkspacesParams{
		OwnerType: "org",
		OwnerID:   org.ID,
		Search:    "data",
		Name:      "Data Lake",
		Sort:      "name",
		Order:     "asc",
		Page:      1,
		PageSize:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(result.Items))
	}
	if result.Items[0].Name != "Data Lake" {
		t.Fatalf("expected Data Lake, got %s", result.Items[0].Name)
	}
}
