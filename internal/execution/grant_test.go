package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGrantAuthorityIssueAndValidate(t *testing.T) {
	authority, err := NewGrantAuthority([]byte("0123456789abcdef0123456789abcdef"), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	authority.now = func() time.Time { return now }
	scope := Scope{TenantID: "tenant", AccountID: "account", WorkspaceID: "workspace", ConnectionID: "connection"}
	grant, err := authority.Issue(scope, []string{"query"}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Signature == "" || grant.RevocationGeneration != 9 {
		t.Fatalf("Issue() = %+v", grant)
	}
	if err := authority.Validate(context.Background(), grant, scope); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := authority.ValidatePermission(context.Background(), grant, scope, "query"); err != nil {
		t.Fatalf("ValidatePermission() error = %v", err)
	}
	assertGrantInvalid(t, authority.ValidatePermission(context.Background(), grant, scope, "execute"))

	tampered := grant
	tampered.Scope.ConnectionID = "other"
	assertGrantInvalid(t, authority.Validate(context.Background(), tampered, tampered.Scope))
	assertGrantInvalid(t, authority.Validate(context.Background(), grant, Scope{ConnectionID: "other"}))

	authority.now = func() time.Time { return grant.ExpiresAt }
	assertGrantInvalid(t, authority.Validate(context.Background(), grant, scope))
}

func TestNewGrantAuthorityRejectsWeakConfiguration(t *testing.T) {
	if _, err := NewGrantAuthority([]byte("short"), "api", "connector", time.Minute); err == nil {
		t.Fatal("expected short signing key to fail")
	}
	if _, err := NewGrantAuthority(make([]byte, 32), "", "connector", time.Minute); err == nil {
		t.Fatal("expected empty issuer to fail")
	}
}

func assertGrantInvalid(t *testing.T, err error) {
	t.Helper()
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != FailureGrantInvalid || !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("error = %v, want grant_invalid", err)
	}
}
