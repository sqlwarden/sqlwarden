package catalog

import (
	"context"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/response"
)

// Store is the catalog persistence contract.
//
// Creation methods are whole transactional units on purpose: CreateOrganization
// and CreateWorkspace commit the resource, its resource_hierarchy row, and its
// seeded builtin roles and policies together, and the connection and
// environment inserts go through the database helpers that write the hierarchy
// row. No use case can therefore produce a resource that is missing its
// hierarchy or policy rows, and no implementation of this contract may split
// those steps apart.
type Store interface {
	// CreateOrganization inserts the organization, records ownerAccountID as
	// its first member, and seeds its builtin roles and owner policy.
	CreateOrganization(ctx context.Context, slug, name string, ownerAccountID int64) (database.Organization, error)
	Organization(ctx context.Context, orgID int64) (database.Organization, bool, error)
	Organizations(ctx context.Context, params database.ListOrganizationsParams) (response.Paginated[database.OrganizationListItem], error)
	AccountOrganizations(ctx context.Context, params database.ListAccountOrgsParams) (response.Paginated[database.AccountOrganizationListItem], error)
	UpdateOrganizationSettings(ctx context.Context, orgID int64, name *string, snapshotsEnabled, maskCredentialsOnEdit *bool) error
	DeleteOrganization(ctx context.Context, orgID int64) error

	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	OrganizationMembers(ctx context.Context, params database.ListOrgMembersParams) (response.Paginated[database.OrgMemberListItem], error)
	OrganizationMember(ctx context.Context, orgID, accountID int64) (database.OrgMemberListItem, bool, error)
	OrgRoles(ctx context.Context, orgID int64) ([]database.Role, error)
	CountRoleBinding(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error)
	AccountHasRoleBinding(ctx context.Context, orgID, roleID, accountID int64, resourceType string, resourceID int64) (bool, error)
	RemoveOrgMemberAccess(ctx context.Context, orgID, accountID int64, revokedBy *int64, reason string) error
	// ReplaceOrgMemberBuiltinRole swaps the member's builtin organization role
	// binding for roleID in one transaction, so the member is never left
	// holding two builtin roles or none.
	ReplaceOrgMemberBuiltinRole(ctx context.Context, orgID, accountID, roleID int64, replacedRoleIDs []int64, grantorID int64) error

	// CreateWorkspace inserts the workspace with its hierarchy row and default
	// environment, and seeds its builtin workspace roles and policies.
	CreateWorkspace(ctx context.Context, orgID, creatorAccountID int64, name, description string) (database.Workspace, error)
	CreatePersonalWorkspace(ctx context.Context, accountID int64, name, description string) (database.Workspace, error)
	PersonalWorkspaces(ctx context.Context, params database.ListWorkspacesParams) (response.Paginated[database.Workspace], error)
	AccessibleWorkspaces(ctx context.Context, accountID, orgID int64) ([]database.Workspace, error)
	HasAccessibleWorkspace(ctx context.Context, accountID, orgID, workspaceID int64) (bool, error)
	PopulateWorkspaceCounts(ctx context.Context, workspaces []database.Workspace) error
	UpdateWorkspace(ctx context.Context, workspaceID int64, name, description string) error
	DeleteWorkspace(ctx context.Context, workspaceID int64) error

	CreateEnvironment(ctx context.Context, workspaceID int64, name, description string) (database.Environment, error)
	WorkspaceEnvironments(ctx context.Context, params database.ListEnvironmentsParams) (response.Paginated[database.Environment], error)
	Environment(ctx context.Context, environmentID int64) (database.Environment, bool, error)
	AccessibleEnvironments(ctx context.Context, accountID, orgID, workspaceID int64) ([]database.Environment, error)
	HasAccessibleEnvironment(ctx context.Context, accountID, orgID, workspaceID, environmentID int64) (bool, error)
	UpdateEnvironment(ctx context.Context, environmentID int64, name, description string) error
	DeleteEnvironment(ctx context.Context, environmentID int64) error
	ConnectionIDsByEnvironment(ctx context.Context, environmentID int64) ([]int64, error)

	// CreateConnection inserts the connection with its hierarchy row,
	// resolving a nil environment to the workspace's default environment.
	CreateConnection(ctx context.Context, workspaceID int64, environmentID *int64, name, driver, dsnEncrypted, accessMode string, defaultScope metadata.ScopePath, showSystemSchemas bool) (database.Connection, error)
	WorkspaceConnections(ctx context.Context, params database.ListConnectionsParams) (response.Paginated[database.Connection], error)
	AccessibleConnections(ctx context.Context, accountID, orgID, workspaceID int64) ([]database.Connection, error)
	HasAccessibleConnection(ctx context.Context, accountID, orgID, workspaceID, connectionID int64) (bool, error)
	UpdateConnection(ctx context.Context, connectionID int64, name, dsnEncrypted, accessMode, snapshotPolicy string, defaultScope metadata.ScopePath, showSystemSchemas bool) error
	UpdateConnectionTLSConfig(ctx context.Context, connectionID int64, sealed string) error
	UpdateConnectionSSHConfig(ctx context.Context, connectionID int64, sealed string) error
	DeleteConnection(ctx context.Context, connectionID int64) error
}
