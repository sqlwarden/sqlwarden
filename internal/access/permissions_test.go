package access_test

import (
	"testing"

	"github.com/sqlwarden/internal/access"
)

func TestValidPermission(t *testing.T) {
	if !access.ValidPermission("org:read") {
		t.Fatal("expected org:read to be valid")
	}
	if access.ValidPermission("bogus:action") {
		t.Fatal("expected bogus:action to be invalid")
	}
}

func TestValidForScope(t *testing.T) {
	if !access.ValidForScope("conn:execute", "connection") {
		t.Fatal("expected conn:execute valid for connection scope")
	}
	if access.ValidForScope("org:delete", "connection") {
		t.Fatal("expected org:delete invalid for connection scope")
	}
}

func TestBuiltinRoles(t *testing.T) {
	ownerPerms := access.BuiltinRoles["owner"]
	if len(ownerPerms) == 0 {
		t.Fatal("owner must have permissions")
	}
	found := false
	for _, p := range ownerPerms {
		if p == "org:transfer_ownership" {
			found = true
		}
	}
	if !found {
		t.Fatal("owner must have org:transfer_ownership")
	}

	memberPerms := access.BuiltinRoles["member"]
	for _, p := range memberPerms {
		if p == "org:delete" {
			t.Fatal("member must not have org:delete")
		}
	}
}
