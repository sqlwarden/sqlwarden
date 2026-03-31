package database

import (
	"testing"
)

func TestWorkspaceCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg("ws-test-org", "WS Test Org")

	ws, err := db.InsertWorkspace(&org.ID, "org", org.ID, "Production", "prod workspace")
	if err != nil {
		t.Fatal(err)
	}
	if ws.ID == 0 {
		t.Fatal("expected non-zero workspace ID")
	}

	found, ok, err := db.GetWorkspace(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected workspace to be found")
	}
	if found.Name != "Production" {
		t.Fatalf("name mismatch: %s", found.Name)
	}

	wss, err := db.ListWorkspacesByOwner("org", org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(wss))
	}

	err = db.DeleteWorkspace(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, _ = db.GetWorkspace(ws.ID)
	if ok {
		t.Fatal("expected workspace to be deleted")
	}
}
