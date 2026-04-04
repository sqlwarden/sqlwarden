package database

import (
	"context"
	"testing"
)

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

func TestListOrgMembers_SupportsSearchAndSort(t *testing.T) {
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

			items, err := db.ListOrgMembers(ctx, ListOrgMembersParams{
				OrgID:  org.ID,
				Search: "ali",
				Sort:   "name",
				Order:  "asc",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 {
				t.Fatalf("expected 1 member, got %d", len(items))
			}
			if items[0].Name != "Alice Analyst" {
				t.Fatalf("expected Alice Analyst, got %s", items[0].Name)
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

			ownerRole := insertTestRole(t, db, org.ID, nil, "owner", "org", true, "org:write")
			adminRole := insertTestRole(t, db, org.ID, nil, "admin", "org", true, "org:read")
			keptRole := insertTestRole(t, db, org.ID, nil, "viewer", "org", false, "org:read")

			insertTestRoleBinding(t, db, org.ID, ownerRole.ID, "account", testUsers["alice"].id, "org", org.ID)
			insertTestRoleBinding(t, db, org.ID, adminRole.ID, "account", testUsers["alice"].id, "org", org.ID)
			insertTestRoleBinding(t, db, org.ID, keptRole.ID, "account", testUsers["alice"].id, "org", org.ID)

			if err := db.DeleteAccountRoleBindings(ctx, org.ID, testUsers["alice"].id, "org", org.ID, []int64{ownerRole.ID, adminRole.ID}); err != nil {
				t.Fatal(err)
			}

			bindings, err := db.ListRoleBindings(ctx, org.ID, "org", org.ID)
			if err != nil {
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
