package database

import (
	"context"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/uptrace/bun"
)

type testRolePermission struct {
	bun.BaseModel `bun:"table:role_permissions"`

	RoleID     int64  `bun:"role_id"`
	Permission string `bun:"permission"`
}

func TestInsertAndGetOrg(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg(context.Background(), "test-org", "Test Org")
	if err != nil {
		t.Fatal(err)
	}
	if org.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	found, ok, err := db.GetOrgBySlug(context.Background(), "test-org")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected org to be found")
	}
	if found.ID != org.ID {
		t.Fatalf("ID mismatch: got %d, want %d", found.ID, org.ID)
	}

	byID, ok, err := db.GetOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected org lookup by ID to succeed")
	}
	if byID.Slug != org.Slug {
		t.Fatalf("slug mismatch: got %q want %q", byID.Slug, org.Slug)
	}
}

func TestUpdateAndDeleteOrg(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	org, err := db.InsertOrg(ctx, "org-settings", "Org Settings")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.UpdateOrg(ctx, org.ID, "Renamed Org"); err != nil {
		t.Fatal(err)
	}
	updated, found, err := db.GetOrg(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || updated.Name != "Renamed Org" || updated.Slug != org.Slug {
		t.Fatalf("unexpected updated org: found=%v org=%+v", found, updated)
	}

	ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.DeleteOrg(ctx, org.ID); err != nil {
		t.Fatal(err)
	}
	_, found, err = db.GetOrg(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected org to be deleted")
	}

	hierarchyCount, err := db.NewSelect().
		TableExpr("resource_hierarchy").
		Where("owner_type = ? AND owner_id = ?", "org", org.ID).
		Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hierarchyCount != 0 {
		t.Fatalf("expected org hierarchy rows to be deleted, got %d", hierarchyCount)
	}

	_, found, err = db.GetWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected org workspace to be deleted")
	}
}

func TestOrgMembership(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg(context.Background(), "member-test", "Member Test")
	if err != nil {
		t.Fatal(err)
	}

	pw := "pw"
	acc, err := db.InsertAccount(context.Background(), "member@example.com", "Member", &pw)
	if err != nil {
		t.Fatal(err)
	}

	err = db.AddOrgMember(context.Background(), org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.IsOrgMember(context.Background(), org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected account to be an org member")
	}

	orgs, err := db.GetAccountOrgs(context.Background(), acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("expected 1 org, got %v", orgs)
	}

	role := insertTestRole(t, db, org.ID, nil, "member-direct-role", "org", false, access.PermOrgRead)
	insertTestRoleBinding(t, db, org.ID, role.ID, "account", acc.ID, "org", org.ID)
	authSession, err := db.InsertAuthSession(context.Background(), acc.ID, time.Now().Add(24*time.Hour), "agent", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureOrgAccessSession(context.Background(), authSession.ID, org.ID, acc.ID, authSession.ExpiresAt); err != nil {
		t.Fatal(err)
	}

	err = db.RemoveOrgMember(context.Background(), org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	ok, err = db.IsOrgMember(context.Background(), org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected account to no longer be a member")
	}
	if got := countTableRows(t, db, "role_bindings", "org_id = ? AND subject_type = ? AND subject_id = ?", org.ID, "account", acc.ID); got != 0 {
		t.Fatalf("expected direct account role bindings to be deleted, got %d", got)
	}
	session, found, err := db.GetOrgAccessSession(context.Background(), authSession.ID, org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || session.RevokedAt == nil {
		t.Fatal("expected org access session to be revoked")
	}
}

func TestListAccountOrgsPage_SupportsPaginationSearchAndSort(t *testing.T) {
	db := newTestDB(t)

	pw := "pw"
	acc, err := db.InsertAccount(context.Background(), "account-orgs@example.com", "Account Orgs", &pw)
	if err != nil {
		t.Fatal(err)
	}

	for _, org := range []struct {
		slug string
		name string
	}{
		{slug: "alpha-team", name: "Alpha Team"},
		{slug: "zeta-labs", name: "Zeta Labs"},
	} {
		inserted, err := db.InsertOrg(context.Background(), org.slug, org.name)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.AddOrgMember(context.Background(), inserted.ID, acc.ID); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.ListAccountOrgsPage(context.Background(), ListAccountOrgsParams{
		AccountID: acc.ID,
		Search:    "zeta",
		Sort:      "name",
		Order:     "desc",
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
		t.Fatalf("expected 1 org, got %d", len(result.Items))
	}
	if result.Items[0].Name != "Zeta Labs" {
		t.Fatalf("expected Zeta Labs, got %s", result.Items[0].Name)
	}
	if result.Items[0].MemberCount != 1 {
		t.Fatalf("expected member_count=1, got %d", result.Items[0].MemberCount)
	}
	if result.Items[0].TeamCount != 0 {
		t.Fatalf("expected team_count=0, got %d", result.Items[0].TeamCount)
	}
}

func TestListAccountOrgsPage_IncludesComputedRole(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	pw := "pw"
	acc, err := db.InsertAccount(ctx, "account-org-role@example.com", "Account Orgs Role", &pw)
	if err != nil {
		t.Fatal(err)
	}

	org, err := db.InsertOrg(ctx, "role-test-org", "Role Test Org")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddOrgMember(ctx, org.ID, acc.ID); err != nil {
		t.Fatal(err)
	}

	role := Role{
		OrgID:     org.ID,
		Name:      access.BuiltinOrgAdminRole,
		ScopeType: "org",
		IsBuiltin: true,
		CreatedAt: org.CreatedAt,
		UpdatedAt: org.UpdatedAt,
	}
	if _, err := db.NewInsert().Model(&role).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewInsert().Model(&testRolePermission{
		RoleID:     role.ID,
		Permission: "org:write",
	}).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewInsert().Model(&RoleBinding{
		OrgID:        org.ID,
		SubjectType:  "account",
		SubjectID:    acc.ID,
		RoleID:       role.ID,
		ResourceType: "org",
		ResourceID:   org.ID,
		CreatedAt:    org.CreatedAt,
	}).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	result, err := db.ListAccountOrgsPage(ctx, ListAccountOrgsParams{
		AccountID: acc.ID,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 org, got %d", len(result.Items))
	}
	if result.Items[0].Role != access.BuiltinOrgAdminRole {
		t.Fatalf("expected role admin, got %s", result.Items[0].Role)
	}
	if result.Items[0].MemberCount != 1 {
		t.Fatalf("expected member_count=1, got %d", result.Items[0].MemberCount)
	}
	if result.Items[0].TeamCount != 0 {
		t.Fatalf("expected team_count=0, got %d", result.Items[0].TeamCount)
	}
}

func TestListOrganizationsPage_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	db := newTestDB(t)

	for _, org := range []struct {
		slug string
		name string
	}{
		{slug: "alpha-team", name: "Alpha Team"},
		{slug: "zeta-labs", name: "Zeta Labs"},
	} {
		if _, err := db.InsertOrg(context.Background(), org.slug, org.name); err != nil {
			t.Fatal(err)
		}
	}

	result, err := db.ListOrganizationsPage(context.Background(), ListOrganizationsParams{
		Search:   "zeta",
		Slug:     "zeta-labs",
		Sort:     "name",
		Order:    "desc",
		Page:     1,
		PageSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 org, got %d", len(result.Items))
	}
	if result.Items[0].Name != "Zeta Labs" {
		t.Fatalf("expected Zeta Labs, got %s", result.Items[0].Name)
	}
	if result.Items[0].MemberCount != 0 {
		t.Fatalf("expected member_count=0, got %d", result.Items[0].MemberCount)
	}
	if result.Items[0].TeamCount != 0 {
		t.Fatalf("expected team_count=0, got %d", result.Items[0].TeamCount)
	}
}

func TestGetOrgMembers(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "org-members-"+driver, "Org Members")
			if err != nil {
				t.Fatal(err)
			}

			if err := db.AddOrgMember(ctx, org.ID, testUsers["alice"].id); err != nil {
				t.Fatal(err)
			}
			if err := db.AddOrgMember(ctx, org.ID, testUsers["bob"].id); err != nil {
				t.Fatal(err)
			}

			members, err := db.GetOrgMembers(ctx, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(members) != 2 {
				t.Fatalf("expected 2 org members, got %d", len(members))
			}
		})
	}
}

func TestListOrgMembers_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "org-members-search-"+driver, "Org Members Search")
			if err != nil {
				t.Fatal(err)
			}
			alice, err := db.InsertAccount(ctx, "alice-search-"+driver+"@example.com", "Alice Analyst", nil)
			if err != nil {
				t.Fatal(err)
			}
			bob, err := db.InsertAccount(ctx, "bob-search-"+driver+"@example.com", "Bob Builder", nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AddOrgMember(ctx, org.ID, alice.ID); err != nil {
				t.Fatal(err)
			}
			if err := db.AddOrgMember(ctx, org.ID, bob.ID); err != nil {
				t.Fatal(err)
			}

			ownerRole := insertTestRole(t, db, org.ID, nil, access.BuiltinOrgOwnerRole, "org", true, "org:write")
			adminRole := insertTestRole(t, db, org.ID, nil, access.BuiltinOrgAdminRole, "org", true, "org:read")
			insertTestRoleBinding(t, db, org.ID, adminRole.ID, "account", alice.ID, "org", org.ID)
			insertTestRoleBinding(t, db, org.ID, ownerRole.ID, "account", bob.ID, "org", org.ID)

			result, err := db.ListOrgMembersPage(ctx, ListOrgMembersParams{
				OrgID:    org.ID,
				Search:   "ali",
				Role:     access.BuiltinOrgAdminRole,
				Sort:     "name",
				Order:    "asc",
				Page:     1,
				PageSize: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 1 {
				t.Fatalf("expected total=1, got %d", result.Total)
			}
			if len(result.Items) != 1 {
				t.Fatalf("expected 1 member, got %d", len(result.Items))
			}
			if result.Items[0].Name != "Alice Analyst" || result.Items[0].Role != access.BuiltinOrgAdminRole {
				t.Fatalf("unexpected member payload: %+v", result.Items[0])
			}
		})
	}
}

func TestDeleteAccountRoleBindings(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "delete-role-bindings-"+driver, "Delete Role Bindings")
			if err != nil {
				t.Fatal(err)
			}

			ownerRole := insertTestRole(t, db, org.ID, nil, access.BuiltinOrgOwnerRole, "org", true, "org:write")
			adminRole := insertTestRole(t, db, org.ID, nil, access.BuiltinOrgAdminRole, "org", true, "org:read")
			keptRole := insertTestRole(t, db, org.ID, nil, "viewer", "org", false, "org:read")

			insertTestRoleBinding(t, db, org.ID, ownerRole.ID, "account", testUsers["alice"].id, "org", org.ID)
			insertTestRoleBinding(t, db, org.ID, adminRole.ID, "account", testUsers["alice"].id, "org", org.ID)
			insertTestRoleBinding(t, db, org.ID, keptRole.ID, "account", testUsers["alice"].id, "org", org.ID)

			if err := db.DeleteAccountRoleBindings(ctx, org.ID, testUsers["alice"].id, "org", org.ID, []int64{ownerRole.ID, adminRole.ID}); err != nil {
				t.Fatal(err)
			}

			var bindings []RoleBinding
			if err := db.NewSelect().Model(&bindings).
				Where("org_id = ? AND resource_type = ? AND resource_id = ?", org.ID, "org", org.ID).
				Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if len(bindings) != 1 {
				t.Fatalf("expected 1 role binding to remain, got %d", len(bindings))
			}
			if bindings[0].RoleID != keptRole.ID {
				t.Fatalf("expected kept role binding to remain, got role_id=%d", bindings[0].RoleID)
			}
		})
	}
}

func TestOrgIDPConfigLifecycle(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			org, err := db.InsertOrg(ctx, "org-idp-"+driver, "Org IDP")
			if err != nil {
				t.Fatal(err)
			}

			_, found, err := db.GetOrgIDPConfig(ctx, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if found {
				t.Fatal("expected no IDP config initially")
			}

			config, err := db.UpsertOrgIDPConfig(ctx, org.ID, "google", "Google SSO", `{"client_id":"abc"}`, true)
			if err != nil {
				t.Fatal(err)
			}
			if config.Provider != "google" {
				t.Fatalf("unexpected provider: %s", config.Provider)
			}

			stored, found, err := db.GetOrgIDPConfig(ctx, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("expected IDP config to be found")
			}
			if stored.DisplayName != "Google SSO" || !stored.SSORequired {
				t.Fatalf("unexpected config after insert: %+v", stored)
			}

			updated, err := db.UpsertOrgIDPConfig(ctx, org.ID, "oidc", "OIDC SSO", `{"issuer":"https://idp.example.com"}`, false)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Provider != "oidc" {
				t.Fatalf("unexpected provider after update: %s", updated.Provider)
			}

			stored, found, err = db.GetOrgIDPConfig(ctx, org.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("expected updated IDP config to be found")
			}
			if stored.Provider != "oidc" || stored.DisplayName != "OIDC SSO" || stored.SSORequired {
				t.Fatalf("unexpected config after update: %+v", stored)
			}
		})
	}
}
