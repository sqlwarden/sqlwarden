package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Workspace struct {
	ID          string    `bun:",pk"      json:"id"`
	TenantID    string    `bun:",notnull" json:"tenant_id"`
	Name        string    `bun:",notnull" json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull" json:"updated_at"`
}

func (db *DB) InsertWorkspace(tenantID, name, description string) (Workspace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	ws := Workspace{
		ID:          newID(),
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := db.NewInsert().
		Model(&ws).
		Exec(ctx)
	if err != nil {
		return Workspace{}, err
	}

	return ws, nil
}

func (db *DB) GetWorkspace(id string) (Workspace, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var ws Workspace
	err := db.NewSelect().
		Model(&ws).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, false, nil
	}
	if err != nil {
		return Workspace{}, false, err
	}

	return ws, true, nil
}

func (db *DB) GetWorkspacesByTenant(tenantID string) ([]Workspace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var workspaces []Workspace
	err := db.NewSelect().
		Model(&workspaces).
		Where("tenant_id = ?", tenantID).
		Order("created_at ASC").
		Scan(ctx)

	return workspaces, err
}

func (db *DB) UpdateWorkspace(id, name, description string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*Workspace)(nil)).
		Set("name = ?", name).
		Set("description = ?", description).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (db *DB) DeleteWorkspace(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*Workspace)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}
