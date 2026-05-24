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
	if access.ValidForScope(access.PermWsDelete, "workspace") {
		t.Fatal("expected ws:delete invalid for workspace role scope")
	}
}

func TestValidForResource(t *testing.T) {
	tests := []struct {
		name         string
		permission   string
		resourceType string
		want         bool
	}{
		{
			name:         "workspace delete is applicable to workspace resources",
			permission:   access.PermWsDelete,
			resourceType: "workspace",
			want:         true,
		},
		{
			name:         "workspace create is not applicable to existing workspace resources",
			permission:   access.PermWsCreate,
			resourceType: "workspace",
			want:         false,
		},
		{
			name:         "environment create is applicable to workspace resources",
			permission:   access.PermEnvCreate,
			resourceType: "workspace",
			want:         true,
		},
		{
			name:         "environment create is not applicable to existing environment resources",
			permission:   access.PermEnvCreate,
			resourceType: "environment",
			want:         false,
		},
		{
			name:         "connection create is applicable to environment resources",
			permission:   access.PermConnCreate,
			resourceType: "environment",
			want:         true,
		},
		{
			name:         "connection create is not applicable to existing connection resources",
			permission:   access.PermConnCreate,
			resourceType: "connection",
			want:         false,
		},
		{
			name:         "organization read is not applicable to workspace resources",
			permission:   access.PermOrgRead,
			resourceType: "workspace",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := access.ValidForResource(tt.permission, tt.resourceType); got != tt.want {
				t.Fatalf("ValidForResource(%q, %q) = %v, want %v", tt.permission, tt.resourceType, got, tt.want)
			}
		})
	}
}

func TestOrgBuiltinRoles(t *testing.T) {
	ownerPerms := access.OrgBuiltinRoles[access.BuiltinOrgOwnerRole]
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

	adminPerms := access.OrgBuiltinRoles[access.BuiltinOrgAdminRole]
	for _, p := range adminPerms {
		if p == "org:delete" {
			t.Fatal("admin must not have org:delete")
		}
		if p == "org:transfer_ownership" {
			t.Fatal("admin must not have org:transfer_ownership")
		}
	}

	memberPerms := access.OrgBuiltinRoles[access.BuiltinOrgMemberRole]
	if len(memberPerms) != 1 || memberPerms[0] != "org:read" {
		t.Fatalf("member role should only grant org:read, got %v", memberPerms)
	}
}

func TestWorkspaceBuiltinRoles(t *testing.T) {
	adminPerms := access.WorkspaceBuiltinRoles[access.BuiltinWorkspaceAdminRole]
	if len(adminPerms) == 0 {
		t.Fatalf("%s must have permissions", access.BuiltinWorkspaceAdminRole)
	}
	for _, p := range adminPerms {
		if p == "ws:create" || p == "ws:delete" {
			t.Fatalf("%s must not have %s", access.BuiltinWorkspaceAdminRole, p)
		}
	}

	memberPerms := access.WorkspaceBuiltinRoles[access.BuiltinWorkspaceMemberRole]
	if len(memberPerms) != 1 || memberPerms[0] != access.PermWsRead {
		t.Fatalf("%s must only grant %s by default, got %v", access.BuiltinWorkspaceMemberRole, access.PermWsRead, memberPerms)
	}
	for _, p := range memberPerms {
		if p == "ws:write" || p == "conn:delete" {
			t.Fatalf("%s must not have %s", access.BuiltinWorkspaceMemberRole, p)
		}
	}
}
