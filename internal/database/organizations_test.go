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
