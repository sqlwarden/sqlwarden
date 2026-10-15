package orgs

import (
	"context"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Store is the organization metadata the service reads and writes.
type Store interface {
	// CreateOwnedOrg inserts the organization, adds the owner as a member, and
	// seeds the builtin roles and owner binding in one transaction.
	CreateOwnedOrg(ctx context.Context, slug, name string, ownerAccountID int64) (database.Organization, error)
	GetOrg(ctx context.Context, id int64) (database.Organization, bool, error)
	UpdateOrgSettings(ctx context.Context, id int64, name *string, snapshotsEnabled *bool, allowConnectionSecretReveal *bool) error
	DeleteOrg(ctx context.Context, id int64) error
	ListOrgConnections(ctx context.Context, orgID int64) ([]database.Connection, error)
	DeleteSchemaCache(ctx context.Context, connID int64) error

	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	ListOrgMembersPage(ctx context.Context, params database.ListOrgMembersParams) (response.Paginated[database.OrgMemberListItem], error)
	GetOrgMember(ctx context.Context, orgID, accountID int64) (database.OrgMemberListItem, bool, error)
	ListAccountTeamsPage(ctx context.Context, params database.ListTeamsParams, accountID int64) (response.Paginated[database.Team], error)
	RemoveOrgMemberAccess(ctx context.Context, orgID, accountID int64, revokedBy *int64, reason string) error

	ListOrgRoles(ctx context.Context, orgID int64) ([]database.Role, error)
	CountRoleBinding(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error)
	AccountHasRoleBinding(ctx context.Context, orgID, roleID, accountID int64, resourceType string, resourceID int64) (bool, error)
	// ReplaceOrgMemberBuiltinRole removes the account's org bindings to
	// replacedRoleIDs and binds roleID in one transaction.
	ReplaceOrgMemberBuiltinRole(ctx context.Context, orgID, accountID, roleID int64, replacedRoleIDs []int64, grantorID int64) error
}

// TeamStore is the team metadata the service reads and writes. Teams are
// always addressed within an organization.
type TeamStore interface {
	ListTeamsPage(ctx context.Context, params database.ListTeamsParams) (response.Paginated[database.Team], error)
	InsertTeam(ctx context.Context, orgID int64, slug, name string) (database.Team, error)
	GetTeam(ctx context.Context, orgID int64, slug string) (database.Team, bool, error)
	UpdateTeam(ctx context.Context, id, orgID int64, name string) error
	// DeleteTeam removes the team and every role binding granted to it.
	DeleteTeam(ctx context.Context, id, orgID int64) error
	ListTeamMembersPage(ctx context.Context, params database.ListTeamMembersParams) (response.Paginated[database.TeamMemberListItem], error)
	AddTeamMember(ctx context.Context, teamID, accountID int64) error
	RemoveTeamMember(ctx context.Context, teamID, accountID int64) error
	ListWorkspaceIDsForTeam(ctx context.Context, teamID int64) ([]int64, error)
}

// SessionRevoker closes an account's live sessions after its access changed.
// Implementations queue a retry when revocation fails.
type SessionRevoker interface {
	RevokeOrgAccount(ctx context.Context, orgID, accountID string) (int, error)
	RevokeWorkspaceAccount(ctx context.Context, workspaceID, accountID string) (int, error)
}

// PolicyCache drops cached policy after bindings or memberships change.
type PolicyCache interface {
	InvalidateOrgPolicy(orgID int64)
	InvalidatePrincipals(orgID, accountID int64)
}
