package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
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

type ListRolesParams struct {
	OrgID       int64
	WorkspaceID *int64
	Scope       string
	Search      string
	Name        string
	IsBuiltin   *bool
	Sort        string
	Order       string
	Page        int
	PageSize    int
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

func (db *DB) listRoles(ctx context.Context, params ListRolesParams) ([]Role, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeRoleListParams(params)

	var roles []Role
	query := db.NewSelect().Model(&roles).Where("org_id = ?", params.OrgID)

	switch params.Scope {
	case "org":
		query = query.Where("workspace_id IS NULL")
	case "workspace":
		if params.WorkspaceID != nil {
			query = query.Where("workspace_id = ?", *params.WorkspaceID)
		} else {
			query = query.Where("workspace_id IS NOT NULL")
		}
	}

	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(description) LIKE ?)", search, search)
	}
	if params.Name != "" {
		query = query.Where("name = ?", params.Name)
	}
	if params.IsBuiltin != nil {
		query = query.Where("is_builtin = ?", *params.IsBuiltin)
	}

	err := query.OrderExpr(fmt.Sprintf("%s %s, id %s", roleSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).Scan(ctx)
	return roles, err
}

func (db *DB) ListRolesPage(ctx context.Context, params ListRolesParams) (response.Paginated[Role], error) {
	roles, err := db.listRoles(ctx, params)
	if err != nil {
		return response.Paginated[Role]{}, err
	}
	return response.PaginateItems(roles, params.Page, params.PageSize), nil
}

// ListRoles returns all roles for an org (org-level and all workspace-level).
func (db *DB) ListRoles(ctx context.Context, orgID int64) ([]Role, error) {
	return db.listRoles(ctx, ListRolesParams{
		OrgID: orgID,
		Scope: "all",
	})
}

func (db *DB) ListOrgRolesPage(ctx context.Context, params ListRolesParams) (response.Paginated[Role], error) {
	params.Scope = "org"
	params.WorkspaceID = nil
	return db.ListRolesPage(ctx, params)
}

// ListOrgRoles returns only the org-level roles (workspace_id IS NULL) for an org.
func (db *DB) ListOrgRoles(ctx context.Context, orgID int64) ([]Role, error) {
	return db.listRoles(ctx, ListRolesParams{
		OrgID: orgID,
		Scope: "org",
	})
}

func (db *DB) ListWorkspaceRolesPage(ctx context.Context, params ListRolesParams) (response.Paginated[Role], error) {
	params.Scope = "workspace"
	return db.ListRolesPage(ctx, params)
}

// ListWorkspaceRoles returns only the roles scoped to a specific workspace.
func (db *DB) ListWorkspaceRoles(ctx context.Context, orgID, workspaceID int64) ([]Role, error) {
	return db.listRoles(ctx, ListRolesParams{
		OrgID:       orgID,
		WorkspaceID: &workspaceID,
		Scope:       "workspace",
	})
}

func normalizeRoleListParams(params ListRolesParams) ListRolesParams {
	switch params.Scope {
	case "org", "workspace":
	default:
		params.Scope = "all"
	}
	switch params.Sort {
	case "created_at":
		params.Sort = "created_at"
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "desc":
		params.Order = "desc"
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	params.Name = strings.TrimSpace(params.Name)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func roleSortColumn(sort string) string {
	switch sort {
	case "created_at":
		return "created_at"
	default:
		return "name"
	}
}
