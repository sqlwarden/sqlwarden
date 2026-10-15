package rbac

import (
	"context"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Store is the role and policy metadata the service reads and writes. Role
// and binding writes go through the enforcer, which drops cached policy for
// the organization after each change.
type Store interface {
	ListRolesPage(ctx context.Context, params database.ListRolesParams) (response.Paginated[database.Role], error)
	ListWorkspaceRolesPage(ctx context.Context, params database.ListRolesParams) (response.Paginated[database.Role], error)
	GetRole(ctx context.Context, id, orgID int64) (database.Role, bool, error)
	// CreateRole inserts a custom role with its permissions. A nil workspaceID
	// creates an organization role.
	CreateRole(ctx context.Context, orgID int64, workspaceID *int64, name, description, scopeType string, permissions []string) (int64, error)
	// UpdateRole replaces a custom role's name, description, and permissions.
	// It reports access.ErrRoleNotFound and access.ErrBuiltinRole.
	UpdateRole(ctx context.Context, roleID, orgID int64, name, description string, permissions []string) error
	// DeleteRole deletes an unbound custom role. It reports
	// access.ErrRoleNotFound, access.ErrBuiltinRole, and
	// access.RoleInUseError.
	DeleteRole(ctx context.Context, roleID, orgID int64) error

	ListOrgPoliciesPage(ctx context.Context, params database.ListOrgPoliciesParams) (response.Paginated[database.WorkspacePolicyListItem], error)
	// ListWorkspacePoliciesPage lists bindings on the workspace and on the
	// environments and connections inside it.
	ListWorkspacePoliciesPage(ctx context.Context, params database.ListWorkspacePoliciesParams) (response.Paginated[database.WorkspacePolicyListItem], error)
	GetRoleBinding(ctx context.Context, id, orgID int64) (database.RoleBinding, bool, error)
	GetPolicyBindingItem(ctx context.Context, orgID int64, binding database.RoleBinding) (database.WorkspacePolicyListItem, error)
	CountRoleBindings(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error)
	// BindRole binds the role to the subject on the resource. Binding the
	// same role, subject, and resource again succeeds without a second row.
	BindRole(ctx context.Context, orgID, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error
	UnbindRole(ctx context.Context, bindingID, orgID int64) error

	GetAccount(ctx context.Context, id int64) (database.Account, bool, error)
	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	GetTeamByID(ctx context.Context, id int64) (database.Team, bool, error)

	GetWorkspace(ctx context.Context, id int64) (database.Workspace, bool, error)
	GetEnvironment(ctx context.Context, id int64) (database.Environment, bool, error)
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
}
