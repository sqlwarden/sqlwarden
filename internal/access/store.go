package access

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

// Role is the authorization view of a role row. It carries only what policy
// decisions need, so the access service does not depend on the metadata
// database package that already depends on this one.
type Role struct {
	ID          int64
	OrgID       int64
	WorkspaceID *int64
	Name        string
	ScopeType   string
	IsBuiltin   bool
	Permissions []string
}

// RoleBinding is the authorization view of a role binding row.
type RoleBinding struct {
	ID           int64
	OrgID        int64
	RoleID       int64
	SubjectType  string
	SubjectID    int64
	ResourceType string
	ResourceID   int64
	CreatedAt    time.Time
}

// Store is the persistence contract the access service reads through when it
// checks tenant boundaries and role scope. Writes go through [Enforcer], which
// owns cache invalidation.
type Store interface {
	Role(ctx context.Context, orgID, roleID int64) (Role, bool, error)
	RoleBinding(ctx context.Context, orgID, bindingID int64) (RoleBinding, bool, error)
	CountRoleBindings(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error)
	AccountExists(ctx context.Context, accountID int64) (bool, error)
	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	TeamOrg(ctx context.Context, teamID int64) (int64, bool, error)
	WorkspaceOrg(ctx context.Context, workspaceID int64) (int64, bool, error)
	EnvironmentWorkspace(ctx context.Context, environmentID int64) (int64, bool, error)
	ConnectionWorkspace(ctx context.Context, connectionID int64) (int64, bool, error)
}

// SQLStore reads authorization rows straight from the metadata database.
type SQLStore struct {
	db *bun.DB
}

// NewSQLStore returns a [Store] backed by db.
func NewSQLStore(db *bun.DB) *SQLStore { return &SQLStore{db: db} }

// Role implements [Store].
func (s *SQLStore) Role(ctx context.Context, orgID, roleID int64) (Role, bool, error) {
	var row struct {
		ID          int64
		OrgID       int64
		WorkspaceID *int64
		Name        string
		ScopeType   string
		IsBuiltin   bool
	}
	err := s.db.NewSelect().
		TableExpr("roles").
		ColumnExpr("id, org_id, workspace_id, name, scope_type, is_builtin").
		Where("id = ? AND org_id = ?", roleID, orgID).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return Role{}, false, nil
	}
	if err != nil {
		return Role{}, false, err
	}

	var permissionRows []struct{ Permission string }
	err = s.db.NewSelect().
		TableExpr("role_permissions").
		ColumnExpr("permission").
		Where("role_id = ?", roleID).
		Scan(ctx, &permissionRows)
	if err != nil {
		return Role{}, false, err
	}
	permissions := make([]string, len(permissionRows))
	for i, permissionRow := range permissionRows {
		permissions[i] = permissionRow.Permission
	}

	return Role{
		ID:          row.ID,
		OrgID:       row.OrgID,
		WorkspaceID: row.WorkspaceID,
		Name:        row.Name,
		ScopeType:   row.ScopeType,
		IsBuiltin:   row.IsBuiltin,
		Permissions: permissions,
	}, true, nil
}

// RoleBinding implements [Store].
func (s *SQLStore) RoleBinding(ctx context.Context, orgID, bindingID int64) (RoleBinding, bool, error) {
	var binding RoleBinding
	err := s.db.NewSelect().
		TableExpr("role_bindings").
		ColumnExpr("id, org_id, role_id, subject_type, subject_id, resource_type, resource_id, created_at").
		Where("id = ? AND org_id = ?", bindingID, orgID).
		Scan(ctx, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return RoleBinding{}, false, nil
	}
	if err != nil {
		return RoleBinding{}, false, err
	}
	return binding, true, nil
}

// CountRoleBindings implements [Store].
func (s *SQLStore) CountRoleBindings(ctx context.Context, orgID, roleID int64, resourceType string, resourceID int64) (int, error) {
	return s.db.NewSelect().
		TableExpr("role_bindings").
		Where("org_id = ? AND role_id = ? AND resource_type = ? AND resource_id = ?", orgID, roleID, resourceType, resourceID).
		Count(ctx)
}

// AccountExists implements [Store].
func (s *SQLStore) AccountExists(ctx context.Context, accountID int64) (bool, error) {
	return s.db.NewSelect().TableExpr("accounts").Where("id = ?", accountID).Exists(ctx)
}

// IsOrgMember implements [Store].
func (s *SQLStore) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	return s.db.NewSelect().
		TableExpr("org_members").
		Where("org_id = ? AND account_id = ?", orgID, accountID).
		Exists(ctx)
}

// TeamOrg implements [Store].
func (s *SQLStore) TeamOrg(ctx context.Context, teamID int64) (int64, bool, error) {
	return s.scanOwner(ctx, "teams", "org_id", teamID)
}

// WorkspaceOrg implements [Store]. A workspace owned by a personal space has
// no organization owner and reports not found, so org-scoped policy can never
// reach it.
func (s *SQLStore) WorkspaceOrg(ctx context.Context, workspaceID int64) (int64, bool, error) {
	var row struct {
		OwnerType string
		OwnerID   int64
	}
	err := s.db.NewSelect().
		TableExpr("workspaces").
		ColumnExpr("owner_type, owner_id").
		Where("id = ?", workspaceID).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if row.OwnerType != "org" {
		return 0, false, nil
	}
	return row.OwnerID, true, nil
}

// EnvironmentWorkspace implements [Store].
func (s *SQLStore) EnvironmentWorkspace(ctx context.Context, environmentID int64) (int64, bool, error) {
	return s.scanOwner(ctx, "environments", "workspace_id", environmentID)
}

// ConnectionWorkspace implements [Store].
func (s *SQLStore) ConnectionWorkspace(ctx context.Context, connectionID int64) (int64, bool, error) {
	return s.scanOwner(ctx, "connections", "workspace_id", connectionID)
}

func (s *SQLStore) scanOwner(ctx context.Context, table, column string, id int64) (int64, bool, error) {
	var owner int64
	err := s.db.NewSelect().TableExpr(table).ColumnExpr(column).Where("id = ?", id).Scan(ctx, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return owner, true, nil
}

var _ Store = (*SQLStore)(nil)
