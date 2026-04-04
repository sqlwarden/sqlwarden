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

	wss, err := db.ListWorkspacesByOwner(context.Background(), "org", org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(wss))
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

func TestListWorkspaces_SupportsSearchAndSort(t *testing.T) {
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

	workspaces, err := db.ListWorkspacesFiltered(context.Background(), ListWorkspacesParams{
		OrgID:  org.ID,
		Search: "data",
		Sort:   "name",
		Order:  "asc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}
	if workspaces[0].Name != "Data Lake" {
		t.Fatalf("expected Data Lake, got %s", workspaces[0].Name)
	}
}
