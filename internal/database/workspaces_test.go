package database

import (
	"context"
	"fmt"
	"testing"
	"time"
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

func TestDeleteWorkspace_RemovesDependentResourcesRolesPoliciesAndHierarchy(t *testing.T) {
	for _, driver := range []string{"postgres", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t, driver)

			org, err := db.InsertOrg(ctx, fmt.Sprintf("ws-delete-cascade-%s", driver), fmt.Sprintf("WS Delete Cascade %s", driver))
			if err != nil {
				t.Fatal(err)
			}

			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace To Delete", "")
			if err != nil {
				t.Fatal(err)
			}

			defaultEnvID, err := db.DefaultEnvironmentID(ctx, ws.ID)
			if err != nil {
				t.Fatal(err)
			}
			extraEnv, err := db.InsertEnvironment(ctx, ws.ID, "Staging", "")
			if err != nil {
				t.Fatal(err)
			}

			defaultConn, err := db.InsertConnection(ctx, ws.ID, &defaultEnvID, "Default Connection", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}
			extraConn, err := db.InsertConnection(ctx, ws.ID, &extraEnv.ID, "Staging Connection", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}

			now := time.Now()
			role := Role{
				OrgID:       org.ID,
				WorkspaceID: &ws.ID,
				Name:        "Workspace Delete Test Role",
				Description: "verifies workspace delete cleanup",
				ScopeType:   "workspace",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if _, err := db.NewInsert().Model(&role).Returning("id").Exec(ctx); err != nil {
				t.Fatal(err)
			}
			rolePermission := map[string]any{
				"role_id":    role.ID,
				"permission": "ws:read",
			}
			if _, err := db.NewInsert().TableExpr("role_permissions").Model(&rolePermission).Exec(ctx); err != nil {
				t.Fatal(err)
			}

			for _, binding := range []RoleBinding{
				{OrgID: org.ID, RoleID: role.ID, SubjectType: "account", SubjectID: testUsers["alice"].id, ResourceType: "workspace", ResourceID: ws.ID, CreatedAt: now},
				{OrgID: org.ID, RoleID: role.ID, SubjectType: "account", SubjectID: testUsers["alice"].id, ResourceType: "environment", ResourceID: extraEnv.ID, CreatedAt: now},
				{OrgID: org.ID, RoleID: role.ID, SubjectType: "account", SubjectID: testUsers["alice"].id, ResourceType: "connection", ResourceID: extraConn.ID, CreatedAt: now},
			} {
				if _, err := db.NewInsert().Model(&binding).Exec(ctx); err != nil {
					t.Fatal(err)
				}
			}

			if got := countTableRows(t, db, "environments", "workspace_id = ?", ws.ID); got != 2 {
				t.Fatalf("expected 2 environments before delete, got %d", got)
			}
			if got := countTableRows(t, db, "connections", "workspace_id = ?", ws.ID); got != 2 {
				t.Fatalf("expected 2 connections before delete, got %d", got)
			}
			if got := countTableRows(t, db, "roles", "workspace_id = ?", ws.ID); got != 1 {
				t.Fatalf("expected 1 workspace role before delete, got %d", got)
			}
			if got := countTableRows(t, db, "role_bindings", "role_id = ?", role.ID); got != 3 {
				t.Fatalf("expected 3 role bindings before delete, got %d", got)
			}
			if got := countWorkspaceHierarchyRows(t, db, ws.ID, extraEnv.ID, defaultConn.ID, extraConn.ID); got == 0 {
				t.Fatal("expected resource hierarchy rows before delete")
			}

			if err := db.DeleteWorkspace(ctx, ws.ID); err != nil {
				t.Fatal(err)
			}

			if _, ok, err := db.GetWorkspace(ctx, ws.ID); err != nil {
				t.Fatal(err)
			} else if ok {
				t.Fatal("expected workspace to be deleted")
			}
			if got := countTableRows(t, db, "environments", "workspace_id = ?", ws.ID); got != 0 {
				t.Fatalf("expected environments to be deleted, got %d", got)
			}
			if got := countTableRows(t, db, "connections", "workspace_id = ?", ws.ID); got != 0 {
				t.Fatalf("expected connections to be deleted, got %d", got)
			}
			if got := countTableRows(t, db, "roles", "workspace_id = ?", ws.ID); got != 0 {
				t.Fatalf("expected workspace roles to be deleted, got %d", got)
			}
			if got := countTableRows(t, db, "role_permissions", "role_id = ?", role.ID); got != 0 {
				t.Fatalf("expected role permissions to be deleted, got %d", got)
			}
			if got := countTableRows(t, db, "role_bindings", "role_id = ?", role.ID); got != 0 {
				t.Fatalf("expected role bindings to be deleted, got %d", got)
			}
			if got := countWorkspaceHierarchyRows(t, db, ws.ID, extraEnv.ID, defaultConn.ID, extraConn.ID); got != 0 {
				t.Fatalf("expected resource hierarchy rows to be deleted, got %d", got)
			}
		})
	}
}

func countTableRows(t *testing.T, db *DB, tableExpr string, where string, args ...any) int {
	t.Helper()

	query := db.NewSelect().TableExpr(tableExpr).ColumnExpr("COUNT(*)")
	if where != "" {
		query = query.Where(where, args...)
	}

	var count int
	if err := query.Scan(context.Background(), &count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countWorkspaceHierarchyRows(t *testing.T, db *DB, workspaceID, environmentID, defaultConnectionID, extraConnectionID int64) int {
	t.Helper()

	return countTableRows(t, db, "resource_hierarchy", `
		(child_type = 'workspace' AND child_id = ?)
		OR (parent_type = 'workspace' AND parent_id = ?)
		OR (child_type = 'environment' AND child_id = ?)
		OR (parent_type = 'environment' AND parent_id = ?)
		OR (child_type = 'connection' AND child_id IN (?, ?))
	`, workspaceID, workspaceID, environmentID, environmentID, defaultConnectionID, extraConnectionID)
}
