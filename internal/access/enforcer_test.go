package access_test

import (
	"testing"

	"github.com/sqlwarden/internal/access"
)

func TestValidPermission_KnownPerms(t *testing.T) {
	known := []string{"org:read", "ws:write", "conn:execute", "policy:modify"}
	for _, p := range known {
		if !access.ValidPermission(p) {
			t.Errorf("expected %q to be valid", p)
		}
	}
	if access.ValidPermission("not:real") {
		t.Error("expected not:real to be invalid")
	}
}
