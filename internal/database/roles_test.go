package database

import (
	"context"
	"testing"
)

func TestGetRoleAndListRoles(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "roles-test-org-"+driver, "Roles Test Org")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}

			orgRole := insertTestRole(t, db, org.ID, nil, "auditor", "org", false, "org:read", "policy:read")
			wsRole := insertTestRole(t, db, org.ID, &ws.ID, "ws:viewer", "workspace", true, "ws:read")

			got, found, err := db.GetRole(ctx, orgRole.ID, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("expected role to be found")
			}
			if len(got.Permissions) != 2 {
				t.Fatalf("expected 2 permissions, got %d", len(got.Permissions))
			}

			_, found, err = db.GetRole(ctx, 999999, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if found {
				t.Fatal("expected missing role lookup to return not found")
			}

			roles, err := db.ListRoles(ctx, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(roles) != 2 {
				t.Fatalf("expected 2 roles, got %d", len(roles))
			}
			if roles[0].Name != orgRole.Name || roles[1].Name != wsRole.Name {
				t.Fatalf("unexpected role order: %+v", roles)
			}
		})
	}
}

func TestListRolesPage_SupportsScopeFilterPaginationSearchAndSort(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "roles-page-"+driver, "Roles Page")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}

			insertTestRole(t, db, org.ID, nil, "admin", "org", true, "policy:read")
			insertTestRole(t, db, org.ID, nil, "auditor", "org", false, "org:read")
			insertTestRole(t, db, org.ID, &ws.ID, "ws:admin", "workspace", true, "ws:write")
			insertTestRole(t, db, org.ID, &ws.ID, "ws:viewer", "workspace", false, "ws:read")

			falseValue := false
			allRoles, err := db.ListRolesPage(ctx, ListRolesParams{
				OrgID:     org.ID,
				Scope:     "all",
				Search:    "view",
				IsBuiltin: &falseValue,
				Sort:      "name",
				Order:     "asc",
				Page:      1,
				PageSize:  10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if allRoles.Total != 1 || len(allRoles.Items) != 1 || allRoles.Items[0].Name != "ws:viewer" {
				t.Fatalf("unexpected all-scope paged result: %+v", allRoles)
			}

			orgRoles, err := db.ListOrgRolesPage(ctx, ListRolesParams{
				OrgID:    org.ID,
				Sort:     "name",
				Order:    "asc",
				Page:     1,
				PageSize: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if orgRoles.Total != 2 || len(orgRoles.Items) != 1 || orgRoles.Items[0].Name != "admin" {
				t.Fatalf("unexpected org roles result: %+v", orgRoles)
			}

			wsRoles, err := db.ListWorkspaceRolesPage(ctx, ListRolesParams{
				OrgID:       org.ID,
				WorkspaceID: &ws.ID,
				Name:        "ws:viewer",
				Page:        1,
				PageSize:    10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if wsRoles.Total != 1 || len(wsRoles.Items) != 1 || wsRoles.Items[0].Name != "ws:viewer" {
				t.Fatalf("unexpected workspace roles result: %+v", wsRoles)
			}
		})
	}
}
