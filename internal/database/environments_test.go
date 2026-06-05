package database

import (
	"context"
	"errors"
	"testing"

	"github.com/uptrace/bun"
)

func TestEnvironmentCRUD(t *testing.T) {
	db := newTestDB(t)

	org, _ := db.InsertOrg(context.Background(), "env-test-org", "Env Test Org")
	ws, _ := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Main", "")

	env, err := db.InsertEnvironment(context.Background(), ws.ID, "staging", "Staging env")
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

	envs, err := db.ListEnvironmentsPage(context.Background(), ListEnvironmentsParams{WorkspaceID: ws.ID, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(envs.Items) != 2 {
		t.Fatalf("expected 2 environments including Default, got %d", len(envs.Items))
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

func TestListEnvironments_SupportsPaginationSearchFilterAndSort(t *testing.T) {
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
		if _, err := db.InsertEnvironment(context.Background(), ws.ID, name, ""); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.ListEnvironmentsPage(context.Background(), ListEnvironmentsParams{
		WorkspaceID: ws.ID,
		Search:      "pro",
		Name:        "prod",
		Sort:        "name",
		Order:       "asc",
		Page:        1,
		PageSize:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(result.Items))
	}
	if result.Items[0].Name != "prod" {
		t.Fatalf("unexpected environment payload: %+v", result.Items[0])
	}
}

func TestInsertEnvironment_RollsBackEnvironmentAndHierarchy(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t, driver)
			org, err := db.InsertOrg(ctx, "env-rollback-"+driver, "Environment Rollback "+driver)
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}

			sentinel := errors.New("abort environment insert")
			var env Environment
			err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				var err error
				env, err = db.insertEnvironmentWithExecutor(ctx, tx, ws.ID, "Rolled Back", "")
				if err != nil {
					return err
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected sentinel rollback error, got %v", err)
			}

			if got := countTableRows(t, db, "environments", "workspace_id = ?", ws.ID); got != 1 {
				t.Fatalf("expected only default environment after rollback, got %d rows", got)
			}
			if env.ID != 0 {
				if got := countTableRows(t, db, "resource_hierarchy", "child_type = 'environment' AND child_id = ?", env.ID); got != 0 {
					t.Fatalf("expected environment hierarchy to roll back, got %d rows", got)
				}
			}
		})
	}
}

func TestDeleteEnvironment_RemovesHierarchyAtomically(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t, driver)
			org, err := db.InsertOrg(ctx, "env-delete-tx-"+driver, "Environment Delete "+driver)
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}
			env, err := db.InsertEnvironment(ctx, ws.ID, "Temporary", "")
			if err != nil {
				t.Fatal(err)
			}
			if got := countTableRows(t, db, "resource_hierarchy", "child_type = 'environment' AND child_id = ?", env.ID); got != 1 {
				t.Fatalf("expected environment hierarchy before delete, got %d", got)
			}

			if err = db.DeleteEnvironment(ctx, env.ID); err != nil {
				t.Fatal(err)
			}
			if got := countTableRows(t, db, "environments", "id = ?", env.ID); got != 0 {
				t.Fatalf("expected environment to be deleted, got %d rows", got)
			}
			if got := countTableRows(t, db, "resource_hierarchy", "child_type = 'environment' AND child_id = ?", env.ID); got != 0 {
				t.Fatalf("expected environment hierarchy to be deleted, got %d rows", got)
			}
		})
	}
}
