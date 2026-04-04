package database

import (
	"context"
	"testing"
)

func TestRoleAndPermissionBindingQueries(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "policy-test-org-"+driver, "Policy Test Org")
			if err != nil {
				t.Fatal(err)
			}
			account := testUsers["alice"]
			role := insertTestRole(t, db, org.ID, nil, "reviewer", "org", false, "policy:read")
			otherRole := insertTestRole(t, db, org.ID, nil, "auditor", "org", false, "org:read")

			rb := insertTestRoleBinding(t, db, org.ID, role.ID, "account", account.id, "org", org.ID)
			insertTestRoleBinding(t, db, org.ID, otherRole.ID, "account", account.id, "org", org.ID)
			pb := insertTestPermissionBinding(t, db, org.ID, "conn:execute", "account", account.id, "org", org.ID)

			count, err := db.CountRoleBinding(ctx, org.ID, role.ID, "org", org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("expected 1 bound account, got %d", count)
			}

			hasRole, err := db.AccountHasRoleBinding(ctx, org.ID, role.ID, account.id, "org", org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !hasRole {
				t.Fatal("expected direct role binding to exist")
			}

			gotRoleBinding, found, err := db.GetRoleBinding(ctx, rb.ID, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !found || gotRoleBinding.ID != rb.ID {
				t.Fatal("expected role binding lookup to succeed")
			}

			roleBindings, err := db.ListRoleBindings(ctx, org.ID, "org", org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(roleBindings) != 2 {
				t.Fatalf("expected 2 role bindings, got %d", len(roleBindings))
			}

			gotPermissionBinding, found, err := db.GetPermissionBinding(ctx, pb.ID, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !found || gotPermissionBinding.ID != pb.ID {
				t.Fatal("expected permission binding lookup to succeed")
			}

			permissionBindings, err := db.ListPermissionBindings(ctx, org.ID, "org", org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(permissionBindings) != 1 {
				t.Fatalf("expected 1 permission binding, got %d", len(permissionBindings))
			}
		})
	}
}

func TestListWorkspacePolicies(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "workspace-policies-"+driver, "Workspace Policies")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}
			env, err := db.InsertEnvironment(ctx, ws.ID, &org.ID, "org", org.ID, "prod", "")
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.InsertConnection(ctx, ws.ID, &env.ID, &org.ID, "org", org.ID, "db", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}

			workspaceRole := insertTestRole(t, db, org.ID, &ws.ID, "ws:viewer", "workspace", true, "ws:read")
			envRole := insertTestRole(t, db, org.ID, &ws.ID, "env:viewer", "workspace", false, "env:read")

			insertTestRoleBinding(t, db, org.ID, workspaceRole.ID, "account", testUsers["alice"].id, "workspace", ws.ID)
			insertTestRoleBinding(t, db, org.ID, envRole.ID, "account", testUsers["alice"].id, "environment", env.ID)
			insertTestPermissionBinding(t, db, org.ID, "conn:execute", "account", testUsers["bob"].id, "connection", conn.ID)

			roleBindings, permissionBindings, err := db.ListWorkspacePolicies(ctx, org.ID, ws.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(roleBindings) != 2 {
				t.Fatalf("expected 2 role bindings, got %d", len(roleBindings))
			}
			if len(permissionBindings) != 1 {
				t.Fatalf("expected 1 permission binding, got %d", len(permissionBindings))
			}
		})
	}
}

func TestListWorkspacePolicies_SupportsSubjectPermissionAndResourceFilters(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "workspace-policy-filters-"+driver, "Workspace Policy Filters")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}
			env, err := db.InsertEnvironment(ctx, ws.ID, &org.ID, "org", org.ID, "prod", "")
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.InsertConnection(ctx, ws.ID, &env.ID, &org.ID, "org", org.ID, "Primary DB", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}
			team, err := db.InsertTeam(ctx, org.ID, "qa-team-"+driver, "QA Team")
			if err != nil {
				t.Fatal(err)
			}

			insertTestPermissionBinding(t, db, org.ID, "conn:execute", "team", team.ID, "connection", conn.ID)
			insertTestPermissionBinding(t, db, org.ID, "ws:read", "account", testUsers["alice"].id, "workspace", ws.ID)

			result, err := db.ListWorkspacePoliciesPage(ctx, ListWorkspacePoliciesParams{
				OrgID:        org.ID,
				WorkspaceID:  ws.ID,
				Search:       "db",
				SubjectType:  "team",
				Permission:   "conn:execute",
				ResourceType: "connection",
				Page:         1,
				PageSize:     10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 1 {
				t.Fatalf("expected total=1, got %d", result.Total)
			}
			if len(result.Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(result.Items))
			}
			if result.Items[0].SubjectName != "QA Team" {
				t.Fatalf("expected subject name QA Team, got %s", result.Items[0].SubjectName)
			}
			if result.Items[0].ResourceName != "Primary DB" {
				t.Fatalf("expected resource name Primary DB, got %s", result.Items[0].ResourceName)
			}
		})
	}
}

func TestListWorkspacePolicies_SupportsSubjectIDAndResourceIDFilters(t *testing.T) {
	t.Parallel()
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "workspace-policy-id-filters-"+driver, "Workspace Policy ID Filters")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}
			env, err := db.InsertEnvironment(ctx, ws.ID, &org.ID, "org", org.ID, "prod", "")
			if err != nil {
				t.Fatal(err)
			}
			connA, err := db.InsertConnection(ctx, ws.ID, &env.ID, &org.ID, "org", org.ID, "Primary DB", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}
			connB, err := db.InsertConnection(ctx, ws.ID, &env.ID, &org.ID, "org", org.ID, "Replica DB", "postgres", "dsn", "open")
			if err != nil {
				t.Fatal(err)
			}

			insertTestPermissionBinding(t, db, org.ID, "conn:execute", "account", testUsers["alice"].id, "connection", connA.ID)
			insertTestPermissionBinding(t, db, org.ID, "conn:read", "account", testUsers["bob"].id, "connection", connA.ID)
			insertTestPermissionBinding(t, db, org.ID, "conn:execute", "account", testUsers["alice"].id, "connection", connB.ID)

			result, err := db.ListWorkspacePoliciesPage(ctx, ListWorkspacePoliciesParams{
				OrgID:       org.ID,
				WorkspaceID: ws.ID,
				SubjectID:   testUsers["alice"].id,
				ResourceID:  connA.ID,
				Page:        1,
				PageSize:    10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 1 {
				t.Fatalf("expected total=1, got %d", result.Total)
			}
			if len(result.Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(result.Items))
			}
			if result.Items[0].SubjectID != testUsers["alice"].id {
				t.Fatalf("expected subject id %d, got %d", testUsers["alice"].id, result.Items[0].SubjectID)
			}
			if result.Items[0].ResourceID != connA.ID {
				t.Fatalf("expected resource id %d, got %d", connA.ID, result.Items[0].ResourceID)
			}
		})
	}
}
