package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type WorkspaceRole struct {
	ID          string    `bun:",pk"      json:"id"`
	TenantID    string    `bun:",notnull" json:"tenant_id"`
	Name        string    `bun:",notnull" json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull" json:"updated_at"`
}

func (db *DB) InsertWorkspaceRole(tenantID, name, description string) (WorkspaceRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	role := WorkspaceRole{
		ID:          newID(),
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := db.NewInsert().
		Model(&role).
		Exec(ctx)
	if err != nil {
		return WorkspaceRole{}, err
	}

	return role, nil
}

func (db *DB) GetWorkspaceRole(id string) (WorkspaceRole, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var role WorkspaceRole
	err := db.NewSelect().
		Model(&role).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceRole{}, false, nil
	}
	if err != nil {
		return WorkspaceRole{}, false, err
	}

	return role, true, nil
}

func (db *DB) GetWorkspaceRoleByName(tenantID, name string) (WorkspaceRole, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var role WorkspaceRole
	err := db.NewSelect().
		Model(&role).
		Where("tenant_id = ? AND name = ?", tenantID, name).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceRole{}, false, nil
	}
	if err != nil {
		return WorkspaceRole{}, false, err
	}

	return role, true, nil
}

func (db *DB) GetWorkspaceRolesByTenant(tenantID string) ([]WorkspaceRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var roles []WorkspaceRole
	err := db.NewSelect().
		Model(&roles).
		Where("tenant_id = ?", tenantID).
		Order("created_at ASC").
		Scan(ctx)

	return roles, err
}

func (db *DB) DeleteWorkspaceRole(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*WorkspaceRole)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}
