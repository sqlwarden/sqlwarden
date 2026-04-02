package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Environment struct {
	ID          int64     `bun:",pk,autoincrement" json:"id"`
	WorkspaceID int64     `bun:",notnull"          json:"workspace_id"`
	OrgID       *int64    `bun:",nullzero"         json:"org_id,omitempty"`
	OwnerType   string    `bun:",notnull"          json:"owner_type"`
	OwnerID     int64     `bun:",notnull"          json:"owner_id"`
	Name        string    `bun:",notnull"          json:"name"`
	Description string    `bun:",nullzero"         json:"description,omitempty"`
	CreatedAt   time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"          json:"updated_at"`
}

func (db *DB) InsertEnvironment(ctx context.Context, workspaceID int64, orgID *int64, ownerType string, ownerID int64, name, description string) (Environment, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	env := Environment{
		WorkspaceID: workspaceID,
		OrgID:       orgID,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_, err := db.NewInsert().Model(&env).Returning("id").Exec(ctx)
	if err != nil {
		return Environment{}, err
	}

	hierarchyOwnerType := ownerType
	hierarchyOwnerID := ownerID
	if ownerType == "org" && orgID != nil {
		hierarchyOwnerID = *orgID
	}
	hm := map[string]interface{}{
		"child_type":  "environment",
		"child_id":    env.ID,
		"parent_type": "workspace",
		"parent_id":   workspaceID,
		"owner_type":  hierarchyOwnerType,
		"owner_id":    hierarchyOwnerID,
	}
	_, err = db.NewInsert().TableExpr("resource_hierarchy").Model(&hm).Ignore().Exec(ctx)
	if err != nil {
		return Environment{}, err
	}

	return env, nil
}

func (db *DB) GetEnvironment(ctx context.Context, id int64) (Environment, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var env Environment
	err := db.NewSelect().Model(&env).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Environment{}, false, nil
	}
	if err != nil {
		return Environment{}, false, err
	}
	return env, true, nil
}

func (db *DB) ListEnvironments(ctx context.Context, workspaceID int64) ([]Environment, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var envs []Environment
	err := db.NewSelect().Model(&envs).
		Where("workspace_id = ?", workspaceID).
		OrderExpr("name ASC").
		Scan(ctx)
	return envs, err
}

func (db *DB) UpdateEnvironment(ctx context.Context, id int64, name, description string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().Model((*Environment)(nil)).
		Set("name = ?", name).
		Set("description = ?", description).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (db *DB) DeleteEnvironment(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*Environment)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where("child_type = 'environment' AND child_id = ?", id).
		Exec(ctx)
	return err
}
