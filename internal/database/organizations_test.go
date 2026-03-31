package database

import (
	"testing"
)

func TestInsertAndGetOrg(t *testing.T) {
	db := newTestDB(t)

	org, err := db.InsertOrg("test-org", "Test Org")
	if err != nil {
		t.Fatal(err)
	}
	if org.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	found, ok, err := db.GetOrgBySlug("test-org")
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

	org, err := db.InsertOrg("member-test", "Member Test")
	if err != nil {
		t.Fatal(err)
	}

	pw := "pw"
	acc, err := db.InsertAccount("member@example.com", "Member", &pw)
	if err != nil {
		t.Fatal(err)
	}

	err = db.AddOrgMember(org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.IsOrgMember(org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected account to be org member")
	}

	orgs, err := db.GetAccountOrgs(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("expected 1 org, got %d", len(orgs))
	}

	err = db.RemoveOrgMember(org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	ok, err = db.IsOrgMember(org.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected account to no longer be member")
	}
}
