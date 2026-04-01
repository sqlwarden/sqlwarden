package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Role struct {
	ID          int64     `bun:",pk,autoincrement" json:"id"`
	OrgID       int64     `bun:",notnull"          json:"org_id"`
	WorkspaceID *int64    `bun:",nullzero"         json:"workspace_id,omitempty"`
	Name        string    `bun:",notnull"          json:"name"`
	Description string    `bun:",nullzero"         json:"description,omitempty"`
	ScopeType   string    `bun:",notnull"          json:"scope_type"`
	IsBuiltin   bool      `bun:",notnull"          json:"is_builtin"`
	CreatedAt   time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"          json:"updated_at"`

	Permissions []string `bun:"-" json:"permissions,omitempty"`
}

func (db *DB) GetRole(ctx context.Context, id, orgID int64) (Role, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var role Role
	err := db.NewSelect().Model(&role).Where("id = ? AND org_id = ?", id, orgID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Role{}, false, nil
	}
	if err != nil {
		return Role{}, false, err
	}

	var perms []string
	err = db.NewSelect().
		TableExpr("role_permissions").
		ColumnExpr("permission").
		Where("role_id = ?", id).
		Scan(ctx, &perms)
	if err != nil {
		return Role{}, false, err
	}
	role.Permissions = perms
	return role, true, nil
}

// ListRoles returns all roles for an org (org-level and all workspace-level).
func (db *DB) ListRoles(ctx context.Context, orgID int64) ([]Role, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var roles []Role
	err := db.NewSelect().Model(&roles).Where("org_id = ?", orgID).OrderExpr("name ASC").Scan(ctx)
	return roles, err
}

// ListOrgRoles returns only the org-level roles (workspace_id IS NULL) for an org.
func (db *DB) ListOrgRoles(ctx context.Context, orgID int64) ([]Role, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var roles []Role
	err := db.NewSelect().Model(&roles).
		Where("org_id = ? AND workspace_id IS NULL", orgID).
		OrderExpr("name ASC").
		Scan(ctx)
	return roles, err
}

// ListWorkspaceRoles returns only the roles scoped to a specific workspace.
func (db *DB) ListWorkspaceRoles(ctx context.Context, orgID, workspaceID int64) ([]Role, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var roles []Role
	err := db.NewSelect().Model(&roles).
		Where("org_id = ? AND workspace_id = ?", orgID, workspaceID).
		OrderExpr("name ASC").
		Scan(ctx)
	return roles, err
}
