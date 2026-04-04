package database

import (
	"context"
	"testing"
)

func TestEnvironmentCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg(context.Background(), "env-test-org", "Env Test Org")
	ws, _ := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")

	env, err := db.InsertEnvironment(context.Background(), ws.ID, &org.ID, "org", org.ID, "staging", "Staging env")
	if err != nil {
		t.Fatal(err)
	}
	if env.ID == 0 {
		t.Fatal("expected non-zero environment ID")
	}

	found, ok, err := db.GetEnvironment(context.Background(), env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected environment to be found")
	}
	if found.Name != "staging" {
		t.Fatalf("name mismatch: %s", found.Name)
	}

	envs, err := db.ListEnvironments(context.Background(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	err = db.UpdateEnvironment(context.Background(), env.ID, "production", "Updated env")
	if err != nil {
		t.Fatal(err)
	}

	found, ok, err = db.GetEnvironment(context.Background(), env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected updated environment to exist")
	}
	if found.Name != "production" || found.Description != "Updated env" {
		t.Fatalf("unexpected updated environment: %+v", found)
	}

	err = db.DeleteEnvironment(context.Background(), env.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, _ = db.GetEnvironment(context.Background(), env.ID)
	if ok {
		t.Fatal("expected environment to be deleted")
	}
}

func TestListEnvironments_SupportsSort(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg(context.Background(), "env-sort-org", "Env Sort Org")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"staging", "dev", "prod"} {
		if _, err := db.InsertEnvironment(context.Background(), ws.ID, &org.ID, "org", org.ID, name, ""); err != nil {
			t.Fatal(err)
		}
	}

	envs, err := db.ListEnvironmentsFiltered(context.Background(), ListEnvironmentsParams{
		WorkspaceID: ws.ID,
		Sort:        "name",
		Order:       "asc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 3 {
		t.Fatalf("expected 3 environments, got %d", len(envs))
	}
	if envs[0].Name != "dev" || envs[1].Name != "prod" || envs[2].Name != "staging" {
		t.Fatalf("unexpected environment order: %+v", envs)
	}
}
