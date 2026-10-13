package rbac

import (
	"context"

	"github.com/sqlwarden/internal/access"
)

// PermissionCatalog is the backend source of truth for permission names,
// labels, descriptions, and which scopes and resources each applies to.
// Fields are declared in JSON key order.
type PermissionCatalog struct {
	PermissionDetails []access.PermissionDefinition            `json:"permission_details"`
	Permissions       []string                                 `json:"permissions"`
	ResourceDetails   map[string][]access.PermissionDefinition `json:"resource_details"`
	ResourceMap       map[string][]string                      `json:"resource_map"`
	ScopeDetails      map[string][]access.PermissionDefinition `json:"scope_details"`
	ScopeMap          map[string][]string                      `json:"scope_map"`
}

// ListPermissions returns the permission catalog to any organization member.
func (s *Service) ListPermissions(ctx context.Context, p access.Principal, orgID int64) (PermissionCatalog, error) {
	if err := s.requireOrg(ctx, p, orgID, ""); err != nil {
		return PermissionCatalog{}, err
	}
	return PermissionCatalog{
		PermissionDetails: access.AllPermissionDefinitions(),
		Permissions:       access.AllPermissions(),
		ResourceDetails:   access.ResourcePermissionDefinitionMap(),
		ResourceMap:       access.ResourcePermissions,
		ScopeDetails:      access.ScopePermissionDefinitionMap(),
		ScopeMap:          access.ScopePermissions,
	}, nil
}
