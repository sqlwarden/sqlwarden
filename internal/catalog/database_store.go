package catalog

import (
	"context"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

// Seeder seeds the builtin roles and policies a newly created resource needs,
// inside the transaction that creates it. *access.Enforcer satisfies it.
type Seeder interface {
	SeedOrgWithExecutor(ctx context.Context, exec bun.IDB, orgID, ownerAccountID int64) error
	SeedWorkspaceWithExecutor(ctx context.Context, exec bun.IDB, orgID, workspaceID, creatorAccountID int64) error
}

// DatabaseStore adapts the metadata database and the policy seeder to [Store].
//
// It exists so the catalog service states use cases rather than SQL, and so
// resource creation and its hierarchy and policy seeding commit as one
// transaction. It adds no rules of its own.
type DatabaseStore struct {
	db     *database.DB
	seeder Seeder
}

// NewDatabaseStore returns a [Store] backed by the metadata database, seeding
// builtin roles and policies through seeder.
func NewDatabaseStore(db *database.DB, seeder Seeder) *DatabaseStore {
	return &DatabaseStore{db: db, seeder: seeder}
}

// CreateOrganization implements [Store].
func (s *DatabaseStore) CreateOrganization(ctx context.Context, slug, name string, ownerAccountID int64) (database.Organization, error) {
	var org database.Organization
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		org, err = s.db.InsertOrgWithExecutor(ctx, tx, slug, name)
		if err != nil {
			return err
		}
		if err = s.db.AddOrgMemberWithExecutor(ctx, tx, org.ID, ownerAccountID); err != nil {
			return err
		}
		return s.seeder.SeedOrgWithExecutor(ctx, tx, org.ID, ownerAccountID)
	})
	if err != nil {
		return database.Organization{}, err
	}
	return org, nil
}

// Organization implements [Store].
func (s *DatabaseStore) Organization(ctx context.Context, orgID int64) (database.Organization, bool, error) {
	return s.db.GetOrg(ctx, orgID)
}

// Organizations implements [Store].
func (s *DatabaseStore) Organizations(ctx context.Context, params database.ListOrganizationsParams) (response.Paginated[database.OrganizationListItem], error) {
	return s.db.ListOrganizationsPage(ctx, params)
}

// AccountOrganizations implements [Store].
func (s *DatabaseStore) AccountOrganizations(ctx context.Context, params database.ListAccountOrgsParams) (response.Paginated[database.AccountOrganizationListItem], error) {
	return s.db.ListAccountOrgsPage(ctx, params)
}

// UpdateOrganizationSettings implements [Store].
func (s *DatabaseStore) UpdateOrganizationSettings(ctx context.Context, orgID int64, name *string, snapshotsEnabled, maskCredentialsOnEdit *bool) error {
	return s.db.UpdateOrgSettings(ctx, orgID, name, snapshotsEnabled, maskCredentialsOnEdit)
}

// DeleteOrganization implements [Store].
func (s *DatabaseStore) DeleteOrganization(ctx context.Context, orgID int64) error {
	return s.db.DeleteOrg(ctx, orgID)
}

// IsOrgMember implements [Store].
func (s *DatabaseStore) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	return s.db.IsOrgMember(ctx, orgID, accountID)
}

// OrganizationMembers implements [Store].
func (s *DatabaseStore) OrganizationMembers(ctx context.Context, params database.ListOrgMembersParams) (response.Paginated[database.OrgMemberListItem], error) {
	return s.db.ListOrgMembersPage(ctx, params)
}

// OrganizationMember implements [Store].
func (s *DatabaseStore) OrganizationMember(ctx context.Context, orgID, accountID int64) (database.OrgMemberListItem, bool, error) {
	return s.db.GetOrgMember(ctx, orgID, accountID)
}

// OrgRoles implements [Store].
func (s *DatabaseStore) OrgRoles(ctx context.Context, orgID int64) ([]database.Role, error) {
	return s.db.ListOrgRoles(ctx, orgID)
}

// CountRoleBinding implements [Store].
func (s *DatabaseStore) CountRoleBinding(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error) {
	return s.db.CountRoleBinding(ctx, orgID, roleID, resourceType, resourceID)
}

// AccountHasRoleBinding implements [Store].
func (s *DatabaseStore) AccountHasRoleBinding(ctx context.Context, orgID, roleID, accountID int64, resourceType string, resourceID int64) (bool, error) {
	return s.db.AccountHasRoleBinding(ctx, orgID, roleID, accountID, resourceType, resourceID)
}

// RemoveOrgMemberAccess implements [Store].
func (s *DatabaseStore) RemoveOrgMemberAccess(ctx context.Context, orgID, accountID int64, revokedBy *int64, reason string) error {
	return s.db.RemoveOrgMemberAccess(ctx, orgID, accountID, revokedBy, reason)
}

// ReplaceOrgMemberBuiltinRole implements [Store].
func (s *DatabaseStore) ReplaceOrgMemberBuiltinRole(ctx context.Context, orgID, accountID, roleID int64, replacedRoleIDs []int64, grantorID int64) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if len(replacedRoleIDs) > 0 {
			if _, err := tx.NewDelete().
				Model((*database.RoleBinding)(nil)).
				Where("org_id = ? AND subject_type = ? AND subject_id = ? AND resource_type = ? AND resource_id = ? AND role_id IN (?)",
					orgID, "account", accountID, "org", orgID, bun.List(replacedRoleIDs)).
				Exec(ctx); err != nil {
				return err
			}
		}
		binding := database.RoleBinding{
			OrgID:        orgID,
			RoleID:       roleID,
			SubjectType:  "account",
			SubjectID:    accountID,
			ResourceType: "org",
			ResourceID:   orgID,
			CreatedBy:    &grantorID,
			CreatedAt:    time.Now(),
		}
		_, err := tx.NewInsert().Model(&binding).Ignore().Exec(ctx)
		return err
	})
}

// CreateWorkspace implements [Store].
func (s *DatabaseStore) CreateWorkspace(ctx context.Context, orgID, creatorAccountID int64, name, description string) (database.Workspace, error) {
	var ws database.Workspace
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		ws, err = s.db.InsertWorkspaceWithExecutor(ctx, tx, &orgID, "org", orgID, name, description)
		if err != nil {
			return err
		}
		return s.seeder.SeedWorkspaceWithExecutor(ctx, tx, orgID, ws.ID, creatorAccountID)
	})
	if err != nil {
		return database.Workspace{}, err
	}
	return ws, nil
}

// CreatePersonalWorkspace implements [Store].
func (s *DatabaseStore) CreatePersonalWorkspace(ctx context.Context, accountID int64, name, description string) (database.Workspace, error) {
	return s.db.InsertWorkspace(ctx, nil, "space", accountID, name, description)
}

// PersonalWorkspaces implements [Store].
func (s *DatabaseStore) PersonalWorkspaces(ctx context.Context, params database.ListWorkspacesParams) (response.Paginated[database.Workspace], error) {
	return s.db.ListWorkspacesPage(ctx, params)
}

// AccessibleWorkspaces implements [Store].
func (s *DatabaseStore) AccessibleWorkspaces(ctx context.Context, accountID, orgID int64) ([]database.Workspace, error) {
	return s.db.ListAccessibleWorkspaces(ctx, accountID, orgID)
}

// HasAccessibleWorkspace implements [Store].
func (s *DatabaseStore) HasAccessibleWorkspace(ctx context.Context, accountID, orgID, workspaceID int64) (bool, error) {
	return s.db.HasAccessibleWorkspace(ctx, accountID, orgID, workspaceID)
}

// PopulateWorkspaceCounts implements [Store].
func (s *DatabaseStore) PopulateWorkspaceCounts(ctx context.Context, workspaces []database.Workspace) error {
	return s.db.PopulateWorkspaceCounts(ctx, workspaces)
}

// UpdateWorkspace implements [Store].
func (s *DatabaseStore) UpdateWorkspace(ctx context.Context, workspaceID int64, name, description string) error {
	return s.db.UpdateWorkspace(ctx, workspaceID, name, description)
}

// DeleteWorkspace implements [Store].
func (s *DatabaseStore) DeleteWorkspace(ctx context.Context, workspaceID int64) error {
	return s.db.DeleteWorkspace(ctx, workspaceID)
}

// CreateEnvironment implements [Store].
func (s *DatabaseStore) CreateEnvironment(ctx context.Context, workspaceID int64, name, description string) (database.Environment, error) {
	return s.db.InsertEnvironment(ctx, workspaceID, name, description)
}

// WorkspaceEnvironments implements [Store].
func (s *DatabaseStore) WorkspaceEnvironments(ctx context.Context, params database.ListEnvironmentsParams) (response.Paginated[database.Environment], error) {
	return s.db.ListEnvironmentsPage(ctx, params)
}

// Environment implements [Store].
func (s *DatabaseStore) Environment(ctx context.Context, environmentID int64) (database.Environment, bool, error) {
	return s.db.GetEnvironment(ctx, environmentID)
}

// AccessibleEnvironments implements [Store].
func (s *DatabaseStore) AccessibleEnvironments(ctx context.Context, accountID, orgID, workspaceID int64) ([]database.Environment, error) {
	return s.db.ListAccessibleEnvironments(ctx, accountID, orgID, workspaceID)
}

// HasAccessibleEnvironment implements [Store].
func (s *DatabaseStore) HasAccessibleEnvironment(ctx context.Context, accountID, orgID, workspaceID, environmentID int64) (bool, error) {
	return s.db.HasAccessibleEnvironment(ctx, accountID, orgID, workspaceID, environmentID)
}

// UpdateEnvironment implements [Store].
func (s *DatabaseStore) UpdateEnvironment(ctx context.Context, environmentID int64, name, description string) error {
	return s.db.UpdateEnvironment(ctx, environmentID, name, description)
}

// DeleteEnvironment implements [Store].
func (s *DatabaseStore) DeleteEnvironment(ctx context.Context, environmentID int64) error {
	return s.db.DeleteEnvironment(ctx, environmentID)
}

// ConnectionIDsByEnvironment implements [Store].
func (s *DatabaseStore) ConnectionIDsByEnvironment(ctx context.Context, environmentID int64) ([]int64, error) {
	return s.db.ListConnectionIDsByEnvironment(ctx, environmentID)
}

// CreateConnection implements [Store].
func (s *DatabaseStore) CreateConnection(ctx context.Context, workspaceID int64, environmentID *int64, name, driver, dsnEncrypted, accessMode string, defaultScope metadata.ScopePath, showSystemSchemas bool) (database.Connection, error) {
	return s.db.InsertConnectionWithScope(ctx, workspaceID, environmentID, name, driver, dsnEncrypted, accessMode, defaultScope, showSystemSchemas)
}

// WorkspaceConnections implements [Store].
func (s *DatabaseStore) WorkspaceConnections(ctx context.Context, params database.ListConnectionsParams) (response.Paginated[database.Connection], error) {
	return s.db.ListConnectionsPage(ctx, params)
}

// AccessibleConnections implements [Store].
func (s *DatabaseStore) AccessibleConnections(ctx context.Context, accountID, orgID, workspaceID int64) ([]database.Connection, error) {
	return s.db.ListAccessibleConnections(ctx, accountID, orgID, workspaceID)
}

// HasAccessibleConnection implements [Store].
func (s *DatabaseStore) HasAccessibleConnection(ctx context.Context, accountID, orgID, workspaceID, connectionID int64) (bool, error) {
	return s.db.HasAccessibleConnection(ctx, accountID, orgID, workspaceID, connectionID)
}

// UpdateConnection implements [Store].
func (s *DatabaseStore) UpdateConnection(ctx context.Context, connectionID int64, name, dsnEncrypted, accessMode, snapshotPolicy string, defaultScope metadata.ScopePath, showSystemSchemas bool) error {
	return s.db.UpdateConnectionWithScopeAndPolicy(ctx, connectionID, name, dsnEncrypted, accessMode, snapshotPolicy, defaultScope, showSystemSchemas)
}

// UpdateConnectionTLSConfig implements [Store].
func (s *DatabaseStore) UpdateConnectionTLSConfig(ctx context.Context, connectionID int64, sealed string) error {
	return s.db.UpdateConnectionTLSConfig(ctx, connectionID, sealed)
}

// UpdateConnectionSSHConfig implements [Store].
func (s *DatabaseStore) UpdateConnectionSSHConfig(ctx context.Context, connectionID int64, sealed string) error {
	return s.db.UpdateConnectionSSHConfig(ctx, connectionID, sealed)
}

// DeleteConnection implements [Store].
func (s *DatabaseStore) DeleteConnection(ctx context.Context, connectionID int64) error {
	return s.db.DeleteConnection(ctx, connectionID)
}

var (
	_ Store  = (*DatabaseStore)(nil)
	_ Seeder = (*access.Enforcer)(nil)
)
