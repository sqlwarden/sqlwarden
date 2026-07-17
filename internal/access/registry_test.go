package access

import "testing"

func TestRegistryAddsDistributionPermissionWithoutMutatingGlobals(t *testing.T) {
	first := NewRegistry()
	err := first.Add(PermissionRegistration{
		ID: "approval:review", Label: "Review approvals", Scopes: []string{"org"}, Resources: []string{"org"},
		DefaultOrgRoles: []string{BuiltinOrgAdminRole},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.ValidForScope("approval:review", "org") || !first.ValidForResource("approval:review", "org") {
		t.Fatal("permission was not registered")
	}
	if second := NewRegistry(); second.Valid("approval:review") {
		t.Fatal("permission leaked into another application registry")
	}
}

func TestRegistryRejectsUnknownScopeResourceAndRole(t *testing.T) {
	tests := []PermissionRegistration{
		{ID: "bad:scope", Scopes: []string{"tenant"}, Resources: []string{"org"}},
		{ID: "bad:resource", Scopes: []string{"org"}, Resources: []string{"database"}},
		{ID: "bad:role", Scopes: []string{"org"}, Resources: []string{"org"}, DefaultOrgRoles: []string{"Superuser"}},
		{ID: "missing:scope", Resources: []string{"org"}},
		{ID: "missing:resource", Scopes: []string{"org"}},
		{ID: "bad:org-default", Scopes: []string{"workspace"}, Resources: []string{"workspace"}, DefaultOrgRoles: []string{BuiltinOrgAdminRole}},
		{ID: "bad:workspace-default", Scopes: []string{"org"}, Resources: []string{"org"}, DefaultWorkspaceRoles: []string{BuiltinWorkspaceAdminRole}},
	}
	for _, input := range tests {
		if err := NewRegistry().Add(input); err == nil {
			t.Fatalf("expected %s to fail", input.ID)
		}
	}
}
