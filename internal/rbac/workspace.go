package rbac

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

const duplicateWorkspaceRoleMessage = "A role with this name already exists in this workspace."

// workspaceResourceTypes are the resources a workspace role can be scoped to
// and a workspace policy can bind.
var workspaceResourceTypes = map[string]bool{"workspace": true, "environment": true, "connection": true}

// ListWorkspaceRolesInput filters and pages a workspace role list. Builtin is
// the raw filter value: empty, "true", or "false".
type ListWorkspaceRolesInput struct {
	Builtin  string
	Name     string
	Search   string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// ListWorkspaceRoles returns a page of the roles defined in the workspace.
func (s *Service) ListWorkspaceRoles(ctx context.Context, p access.Principal, orgID, workspaceID int64, in ListWorkspaceRolesInput) (response.Paginated[Role], error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return response.Paginated[Role]{}, err
	}
	builtin, err := parseBuiltin(in.Builtin)
	if err != nil {
		return response.Paginated[Role]{}, err
	}
	return s.store.ListWorkspaceRolesPage(ctx, database.ListRolesParams{
		OrgID:       orgID,
		WorkspaceID: &workspaceID,
		Search:      in.Search,
		Name:        strings.TrimSpace(in.Name),
		IsBuiltin:   builtin,
		Sort:        in.Sort,
		Order:       in.Order,
		Page:        in.Page,
		PageSize:    in.PageSize,
	})
}

type CreateWorkspaceRoleInput struct {
	Name        string
	Description string
	// ScopeType defaults to "workspace"; "environment" and "connection" are
	// also accepted.
	ScopeType   string
	Permissions []string
}

// CreateWorkspaceRole creates a custom role owned by the workspace. The scope
// and every permission are validated before anything is written.
func (s *Service) CreateWorkspaceRole(ctx context.Context, p access.Principal, orgID, workspaceID int64, in CreateWorkspaceRoleInput) (Role, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return Role{}, err
	}
	fields := map[string]string{}
	if in.Name == "" {
		addField(fields, "name", "Name is required.")
	}
	if in.ScopeType == "" {
		in.ScopeType = "workspace"
	}
	if !workspaceResourceTypes[in.ScopeType] {
		addField(fields, "scope_type", "Scope type must be workspace, environment, or connection.")
	}
	for _, permission := range in.Permissions {
		if !access.ValidForScope(permission, in.ScopeType) {
			addField(fields, "permissions", "Permission "+permission+" is not valid for "+in.ScopeType+" scope.")
		}
	}
	if len(fields) != 0 {
		return Role{}, &ValidationError{Fields: fields}
	}

	roleID, err := s.store.CreateRole(ctx, orgID, &workspaceID, in.Name, in.Description, in.ScopeType, in.Permissions)
	switch {
	case err == nil:
	case isPermissionError(err):
		return Role{}, &ValidationError{Fields: map[string]string{"permissions": invalidPermissionsMessage}}
	case isUniqueViolation(err):
		return Role{}, &ValidationError{Fields: map[string]string{"name": duplicateWorkspaceRoleMessage}}
	default:
		return Role{}, err
	}
	role, err := s.loadRole(ctx, orgID, roleID)
	if err != nil {
		return Role{}, err
	}
	s.emit(ctx, p, orgID, ActionRoleCreated, "role", role.ID, map[string]string{
		"workspace_id": strconv.FormatInt(workspaceID, 10),
		"permissions":  strings.Join(in.Permissions, ","),
	})
	return role, nil
}

// GetWorkspaceRole returns a role owned by the workspace. Organization roles
// and roles of other workspaces are reported as ErrNotFound.
func (s *Service) GetWorkspaceRole(ctx context.Context, p access.Principal, orgID, workspaceID, roleID int64) (Role, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return Role{}, err
	}
	return s.loadWorkspaceRole(ctx, orgID, workspaceID, roleID)
}

// UpdateWorkspaceRole replaces the name, description, and permissions of a
// custom role owned by the workspace. Builtin roles report ErrBuiltinRole.
func (s *Service) UpdateWorkspaceRole(ctx context.Context, p access.Principal, orgID, workspaceID, roleID int64, in UpdateRoleInput) (Role, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return Role{}, err
	}
	if in.Name == "" {
		return Role{}, &ValidationError{Fields: map[string]string{"name": "Name is required."}}
	}
	current, err := s.loadWorkspaceRole(ctx, orgID, workspaceID, roleID)
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
		return Role{}, &ValidationError{Fields: map[string]string{"name": duplicateWorkspaceRoleMessage}}
	default:
		return Role{}, err
	}
	s.emit(ctx, p, orgID, ActionRoleUpdated, "role", roleID, map[string]string{"workspace_id": strconv.FormatInt(workspaceID, 10)})
	return s.loadRole(ctx, orgID, roleID)
}

// DeleteWorkspaceRole deletes an unbound custom role owned by the workspace.
// Builtin roles report ErrBuiltinRole and bound roles an
// access.RoleInUseError.
func (s *Service) DeleteWorkspaceRole(ctx context.Context, p access.Principal, orgID, workspaceID, roleID int64) error {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return err
	}
	if _, err := s.loadWorkspaceRole(ctx, orgID, workspaceID, roleID); err != nil {
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
	s.emit(ctx, p, orgID, ActionRoleDeleted, "role", roleID, map[string]string{"workspace_id": strconv.FormatInt(workspaceID, 10)})
	return nil
}

// WorkspacePermissionCatalog lists the permissions a workspace-scoped role may
// grant. Fields are declared in JSON key order.
type WorkspacePermissionCatalog struct {
	PermissionDetails []access.PermissionDefinition `json:"permission_details"`
	Permissions       []string                      `json:"permissions"`
}

// ListWorkspacePermissions returns the workspace permission catalog to any
// organization member.
func (s *Service) ListWorkspacePermissions(ctx context.Context, p access.Principal, orgID, workspaceID int64) (WorkspacePermissionCatalog, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, ""); err != nil {
		return WorkspacePermissionCatalog{}, err
	}
	return WorkspacePermissionCatalog{
		PermissionDetails: access.ScopePermissionDefinitions("workspace"),
		Permissions:       access.ScopePermissions["workspace"],
	}, nil
}

// ListWorkspacePolicies returns a page of the bindings on the workspace and
// the environments and connections inside it. params.OrgID and
// params.WorkspaceID are set from the arguments.
func (s *Service) ListWorkspacePolicies(ctx context.Context, p access.Principal, orgID, workspaceID int64, params ListWorkspacePoliciesParams) (response.Paginated[PolicyItem], error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return response.Paginated[PolicyItem]{}, err
	}
	params.OrgID = orgID
	params.WorkspaceID = workspaceID
	return s.store.ListWorkspacePoliciesPage(ctx, params)
}

// GetWorkspacePolicy returns a binding on the workspace or on a resource
// inside it. Other bindings are reported as ErrNotFound.
func (s *Service) GetWorkspacePolicy(ctx context.Context, p access.Principal, orgID, workspaceID, bindingID int64) (PolicyItem, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return PolicyItem{}, err
	}
	binding, err := s.loadWorkspaceBinding(ctx, orgID, workspaceID, bindingID)
	if err != nil {
		return PolicyItem{}, err
	}
	return s.store.GetPolicyBindingItem(ctx, orgID, binding)
}

type GrantWorkspacePolicyInput struct {
	RoleID      int64
	SubjectType string
	SubjectID   int64
	// ResourceType defaults to "workspace". Environment and connection grants
	// name the resource in ResourceID.
	ResourceType string
	ResourceID   int64
}

// GrantWorkspacePolicy binds a role to a subject on the workspace or on an
// environment or connection inside it, and returns the bound resource id. The
// subject must belong to the organization (or be the workspace's members), the
// resource must belong to the workspace, and the role's scope must match the
// resource type. Granting an existing binding again succeeds without effect.
func (s *Service) GrantWorkspacePolicy(ctx context.Context, p access.Principal, orgID, workspaceID int64, in GrantWorkspacePolicyInput) (int64, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return 0, err
	}
	if in.ResourceType == "" {
		in.ResourceType = "workspace"
	}
	fields := map[string]string{}
	if in.RoleID <= 0 {
		addField(fields, "role_id", "Role is required.")
	}
	if !validWorkspaceSubjectType(in.SubjectType) {
		addField(fields, "subject_type", "Subject type must be account, team, org_members, or workspace_members.")
	}
	if in.SubjectID <= 0 {
		addField(fields, "subject_id", "Subject is required.")
	}
	if !workspaceResourceTypes[in.ResourceType] {
		addField(fields, "resource_type", "Resource type must be workspace, environment, or connection.")
	}
	if in.ResourceType != "workspace" && in.ResourceID <= 0 {
		addField(fields, "resource_id", "Resource is required for non-workspace resources.")
	}
	if len(fields) != 0 {
		return 0, &ValidationError{Fields: fields}
	}

	exists, err := s.workspaceSubjectExists(ctx, orgID, workspaceID, in.SubjectType, in.SubjectID)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrNotFound
	}
	resourceID := workspaceID
	if in.ResourceType != "workspace" {
		resourceID = in.ResourceID
	}
	inWorkspace, err := s.resourceBelongsToWorkspace(ctx, in.ResourceType, resourceID, workspaceID)
	if err != nil {
		return 0, err
	}
	if !inWorkspace {
		return 0, ErrNotFound
	}
	role, err := s.loadRole(ctx, orgID, in.RoleID)
	if err != nil {
		return 0, err
	}
	if role.ScopeType != in.ResourceType {
		return 0, &ValidationError{Fields: map[string]string{"role_id": "Role scope must match resource type."}}
	}
	if role.WorkspaceID != nil && *role.WorkspaceID != workspaceID {
		return 0, ErrNotFound
	}
	if !s.holdsPrivileged(ctx, p, orgID, role) {
		return 0, &RefusalError{Err: ErrPrivilegedPolicy, Role: role}
	}
	if err := s.store.BindRole(ctx, orgID, role.ID, in.SubjectType, in.SubjectID, in.ResourceType, resourceID, p.Subject.ID); err != nil {
		return 0, err
	}
	s.emit(ctx, p, orgID, ActionPolicyGranted, "role", role.ID, map[string]string{
		"workspace_id":  strconv.FormatInt(workspaceID, 10),
		"subject_type":  in.SubjectType,
		"subject_id":    strconv.FormatInt(in.SubjectID, 10),
		"resource_type": in.ResourceType,
		"resource_id":   strconv.FormatInt(resourceID, 10),
	})
	return resourceID, nil
}

// RevokeWorkspacePolicy removes a binding on the workspace or on a resource
// inside it and returns it. Other bindings are reported as ErrNotFound.
func (s *Service) RevokeWorkspacePolicy(ctx context.Context, p access.Principal, orgID, workspaceID, bindingID int64) (Binding, error) {
	if err := s.requireWorkspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return Binding{}, err
	}
	binding, err := s.loadWorkspaceBinding(ctx, orgID, workspaceID, bindingID)
	if err != nil {
		return Binding{}, err
	}
	role, err := s.loadRole(ctx, orgID, binding.RoleID)
	if err != nil {
		return Binding{}, err
	}
	if !s.holdsPrivileged(ctx, p, orgID, role) {
		return Binding{}, &RefusalError{Err: ErrPrivilegedPolicy, Role: role, Binding: binding}
	}
	if err := s.store.UnbindRole(ctx, binding.ID, orgID); err != nil {
		return Binding{}, err
	}
	s.emit(ctx, p, orgID, ActionPolicyRevoked, "role_binding", binding.ID, map[string]string{
		"workspace_id":  strconv.FormatInt(workspaceID, 10),
		"role_id":       strconv.FormatInt(binding.RoleID, 10),
		"subject_type":  binding.SubjectType,
		"subject_id":    strconv.FormatInt(binding.SubjectID, 10),
		"resource_type": binding.ResourceType,
		"resource_id":   strconv.FormatInt(binding.ResourceID, 10),
	})
	return binding, nil
}

func (s *Service) loadWorkspaceRole(ctx context.Context, orgID, workspaceID, roleID int64) (Role, error) {
	role, err := s.loadRole(ctx, orgID, roleID)
	if err != nil {
		return Role{}, err
	}
	if role.WorkspaceID == nil || *role.WorkspaceID != workspaceID {
		return Role{}, ErrNotFound
	}
	return role, nil
}

func (s *Service) loadWorkspaceBinding(ctx context.Context, orgID, workspaceID, bindingID int64) (Binding, error) {
	binding, found, err := s.store.GetRoleBinding(ctx, bindingID, orgID)
	if err != nil {
		return Binding{}, err
	}
	if !found {
		return Binding{}, ErrNotFound
	}
	ok, err := s.resourceBelongsToWorkspace(ctx, binding.ResourceType, binding.ResourceID, workspaceID)
	if err != nil {
		return Binding{}, err
	}
	if !ok {
		return Binding{}, ErrNotFound
	}
	return binding, nil
}

// resourceBelongsToWorkspace reports whether the workspace, environment, or
// connection is the workspace or sits inside it. Bindings on any other
// resource type never belong to a workspace.
func (s *Service) resourceBelongsToWorkspace(ctx context.Context, resourceType string, resourceID, workspaceID int64) (bool, error) {
	switch resourceType {
	case "workspace":
		return resourceID == workspaceID, nil
	case "environment":
		env, found, err := s.store.GetEnvironment(ctx, resourceID)
		if err != nil {
			return false, err
		}
		return found && env.WorkspaceID == workspaceID, nil
	case "connection":
		conn, found, err := s.store.GetConnection(ctx, resourceID)
		if err != nil {
			return false, err
		}
		return found && conn.WorkspaceID == workspaceID, nil
	default:
		return false, nil
	}
}

func validWorkspaceSubjectType(subjectType string) bool {
	return subjectType == access.SubjectTypeWorkspaceMembers || ValidSubjectType(subjectType)
}

func (s *Service) workspaceSubjectExists(ctx context.Context, orgID, workspaceID int64, subjectType string, subjectID int64) (bool, error) {
	if subjectType == access.SubjectTypeWorkspaceMembers {
		return subjectID == workspaceID, nil
	}
	return s.subjectExists(ctx, orgID, subjectType, subjectID)
}
