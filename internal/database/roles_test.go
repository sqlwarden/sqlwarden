package database

import (
	"context"
	"testing"
)

func TestGetRoleAndListRoles(t *testing.T) {
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
