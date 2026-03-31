package database

import (
	"testing"
)

func TestEnvironmentCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg("env-test-org", "Env Test Org")
	ws, _ := db.InsertWorkspace(&org.ID, "org", org.ID, "Main", "")

	env, err := db.InsertEnvironment(ws.ID, &org.ID, "org", org.ID, "staging", "Staging env")
	if err != nil {
		t.Fatal(err)
	}
	if env.ID == 0 {
		t.Fatal("expected non-zero environment ID")
	}

	found, ok, err := db.GetEnvironment(env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected environment to be found")
	}
	if found.Name != "staging" {
		t.Fatalf("name mismatch: %s", found.Name)
	}

	envs, err := db.ListEnvironments(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	err = db.DeleteEnvironment(env.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, _ = db.GetEnvironment(env.ID)
	if ok {
		t.Fatal("expected environment to be deleted")
	}
}
