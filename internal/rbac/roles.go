package rbac

import (
	"context"
	"errors"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Role is a builtin or custom role with its permissions.
type Role = database.Role

const invalidPermissionsMessage = "Permissions include a permission that is not valid for this scope."

// ListRolesInput filters and pages a role list. Builtin is the raw filter
// value: empty, "true", or "false". Scope is empty, "all", "org", or
// "workspace".
type ListRolesInput struct {
	Scope    string
	Builtin  string
	Name     string
	Search   string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// ListRoles returns a page of the organization's roles.
func (s *Service) ListRoles(ctx context.Context, p access.Principal, orgID int64, in ListRolesInput) (response.Paginated[Role], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyRead); err != nil {
		return response.Paginated[Role]{}, err
	}
	builtin, err := parseBuiltin(in.Builtin)
	if err != nil {
		return response.Paginated[Role]{}, err
	}
	scope := strings.TrimSpace(in.Scope)
	switch scope {
	case "":
		scope = "all"
	case "all", "org", "workspace":
	default:
		return response.Paginated[Role]{}, &ValidationError{Fields: map[string]string{"scope": "Scope must be all, org, or workspace."}}
	}
	return s.store.ListRolesPage(ctx, database.ListRolesParams{
		OrgID:     orgID,
		Scope:     scope,
		Search:    in.Search,
		Name:      strings.TrimSpace(in.Name),
		IsBuiltin: builtin,
		Sort:      in.Sort,
		Order:     in.Order,
		Page:      in.Page,
		PageSize:  in.PageSize,
	})
}

type CreateRoleInput struct {
	Name        string
	Description string
	// ScopeType defaults to "org"; organization roles accept no other scope.
	ScopeType   string
	WorkspaceID *int64
	Permissions []string
}

// CreateRole creates a custom organization role. The scope and every
// permission are validated before anything is written.
func (s *Service) CreateRole(ctx context.Context, p access.Principal, orgID int64, in CreateRoleInput) (Role, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyModify); err != nil {
		return Role{}, err
	}
	fields := map[string]string{}
	if in.Name == "" {
		addField(fields, "name", "Name is required.")
	}
	if in.ScopeType == "" {
		in.ScopeType = "org"
	}
	if in.ScopeType != "org" {
		addField(fields, "scope_type", "Organization roles must have scope_type=org.")
	}
	if in.WorkspaceID != nil {
		addField(fields, "workspace_id", "Organization roles cannot set workspace_id.")
	}
	for _, permission := range in.Permissions {
		if !access.ValidForScope(permission, in.ScopeType) {
			addField(fields, "permissions", "Permission "+permission+" is not valid for scope "+in.ScopeType+".")
		}
	}
	if len(fields) != 0 {
		return Role{}, &ValidationError{Fields: fields}
	}

	roleID, err := s.store.CreateRole(ctx, orgID, nil, in.Name, in.Description, in.ScopeType, in.Permissions)
	if err != nil {
		if isPermissionError(err) {
			return Role{}, &ValidationError{Fields: map[string]string{"permissions": invalidPermissionsMessage}}
		}
		return Role{}, err
	}
	role, err := s.loadRole(ctx, orgID, roleID)
	if err != nil {
		return Role{}, err
	}
	s.emit(ctx, p, orgID, ActionRoleCreated, "role", role.ID, map[string]string{"permissions": strings.Join(in.Permissions, ",")})
	return role, nil
}

// GetRole returns one of the organization's roles.
func (s *Service) GetRole(ctx context.Context, p access.Principal, orgID, roleID int64) (Role, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyRead); err != nil {
		return Role{}, err
	}
	return s.loadRole(ctx, orgID, roleID)
}

type UpdateRoleInput struct {
	Name        string
	Description string
	Permissions []string
}

// UpdateRole replaces a custom role's name, description, and permissions.
// Builtin roles are immutable and report ErrBuiltinRole. Adding or removing
// an owner-level permission requires the caller to hold it.
func (s *Service) UpdateRole(ctx context.Context, p access.Principal, orgID, roleID int64, in UpdateRoleInput) (Role, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyModify); err != nil {
		return Role{}, err
	}
	if in.Name == "" {
		return Role{}, &ValidationError{Fields: map[string]string{"name": "Name is required."}}
	}
	current, err := s.loadRole(ctx, orgID, roleID)
	if err != nil {
		return Role{}, err
	}
	if err := s.checkRoleUpdate(ctx, p, orgID, current, in.Permissions); err != nil {
		return Role{}, err
	}
	err = s.store.UpdateRole(ctx, roleID, orgID, in.Name, in.Description, in.Permissions)
	switch {
	case err == nil:
	case errors.Is(err, access.ErrBuiltinRole):
		return Role{}, ErrBuiltinRole
	case errors.Is(err, access.ErrRoleNotFound):
		return Role{}, ErrNotFound
	case isPermissionError(err):
		return Role{}, &ValidationError{Fields: map[string]string{"permissions": invalidPermissionsMessage}}
	case isUniqueViolation(err):
		return Role{}, &ValidationError{Fields: map[string]string{"name": "A role with this name already exists in this organization."}}
	default:
		return Role{}, err
	}
	s.emit(ctx, p, orgID, ActionRoleUpdated, "role", roleID, nil)
	return s.loadRole(ctx, orgID, roleID)
}

// checkRoleUpdate refuses an update of current to permissions when current is
// builtin, a permission is outside its scope, or the update adds or removes an
// owner-level permission the caller does not hold. Scope is checked first so
// an out-of-scope permission reports a validation error, not a refusal.
func (s *Service) checkRoleUpdate(ctx context.Context, p access.Principal, orgID int64, current Role, permissions []string) error {
	if current.IsBuiltin {
		return ErrBuiltinRole
	}
	for _, permission := range permissions {
		if !access.ValidForScope(permission, current.ScopeType) {
			return &ValidationError{Fields: map[string]string{"permissions": invalidPermissionsMessage}}
		}
	}
	if !s.holdsPrivilegedChange(ctx, p, orgID, current, permissions) {
		return &RefusalError{Err: ErrPrivilegedPolicy, Role: current}
	}
	return nil
}

// DeleteRole deletes a custom role. Builtin roles report ErrBuiltinRole and
// roles that policy bindings still reference report an
// access.RoleInUseError, so deleting a role never silently revokes access.
func (s *Service) DeleteRole(ctx context.Context, p access.Principal, orgID, roleID int64) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyModify); err != nil {
		return err
	}
	err := s.store.DeleteRole(ctx, roleID, orgID)
	switch {
	case err == nil:
	case errors.Is(err, access.ErrBuiltinRole):
		return ErrBuiltinRole
	case errors.Is(err, access.ErrRoleNotFound):
		return ErrNotFound
	default:
		return err
	}
	s.emit(ctx, p, orgID, ActionRoleDeleted, "role", roleID, nil)
	return nil
}

func (s *Service) loadRole(ctx context.Context, orgID, roleID int64) (Role, error) {
	role, found, err := s.store.GetRole(ctx, roleID, orgID)
	if err != nil {
		return Role{}, err
	}
	if !found {
		return Role{}, ErrNotFound
	}
	return role, nil
}

// parseBuiltin reads the builtin role filter: empty matches every role.
func parseBuiltin(raw string) (*bool, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return nil, nil
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	default:
		return nil, &ValidationError{Fields: map[string]string{"builtin": "Built-in flag must be true or false."}}
	}
}

func isPermissionError(err error) bool {
	return errors.Is(err, access.ErrInvalidScopePermission) || errors.Is(err, access.ErrUnknownPermission)
}
