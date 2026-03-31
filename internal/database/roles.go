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
	Name        string    `bun:",notnull"          json:"name"`
	Description string    `bun:",nullzero"         json:"description,omitempty"`
	ScopeType   string    `bun:",notnull"          json:"scope_type"`
	IsBuiltin   bool      `bun:",notnull"          json:"is_builtin"`
	CreatedAt   time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"          json:"updated_at"`

	Permissions []string `bun:"-" json:"permissions,omitempty"`
}

func (db *DB) GetRole(id, orgID int64) (Role, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
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

func (db *DB) ListRoles(orgID int64) ([]Role, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var roles []Role
	err := db.NewSelect().Model(&roles).Where("org_id = ?", orgID).OrderExpr("name ASC").Scan(ctx)
	return roles, err
}
