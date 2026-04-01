package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Workspace struct {
	ID          int64     `bun:",pk,autoincrement" json:"id"`
	OrgID       *int64    `bun:",nullzero"         json:"org_id,omitempty"`
	OwnerType   string    `bun:",notnull"          json:"owner_type"`
	OwnerID     int64     `bun:",notnull"          json:"owner_id"`
	Name        string    `bun:",notnull"          json:"name"`
	Description string    `bun:",nullzero"         json:"description,omitempty"`
	CreatedAt   time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"          json:"updated_at"`
}

func (db *DB) InsertWorkspace(orgID *int64, ownerType string, ownerID int64, name, description string) (Workspace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	ws := Workspace{
		OrgID:       orgID,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_, err := db.NewInsert().Model(&ws).Returning("id").Exec(ctx)
	if err != nil {
		return Workspace{}, err
	}

	if ownerType == "org" {
		hm := map[string]interface{}{
			"child_type":  "workspace",
			"child_id":    ws.ID,
			"parent_type": "org",
			"parent_id":   ownerID,
			"owner_type":  "org",
			"owner_id":    ownerID,
		}
		_, err = db.NewInsert().TableExpr("resource_hierarchy").Model(&hm).Ignore().Exec(ctx)
		if err != nil {
			return Workspace{}, err
		}
	}

	return ws, nil
}

func (db *DB) GetWorkspace(id int64) (Workspace, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var ws Workspace
	err := db.NewSelect().Model(&ws).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, false, nil
	}
	if err != nil {
		return Workspace{}, false, err
	}
	return ws, true, nil
}

func (db *DB) ListWorkspacesByOwner(ownerType string, ownerID int64) ([]Workspace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var wss []Workspace
	err := db.NewSelect().Model(&wss).
		Where("owner_type = ? AND owner_id = ?", ownerType, ownerID).
		OrderExpr("name ASC").
		Scan(ctx)
	return wss, err
}

func (db *DB) UpdateWorkspace(id int64, name, description string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().Model((*Workspace)(nil)).
		Set("name = ?", name).
		Set("description = ?", description).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (db *DB) DeleteWorkspace(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*Workspace)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	// Clean up hierarchy rows for this workspace and all its children (environments/connections
	// are cascade-deleted in the DB but resource_hierarchy has no FK constraints).
	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where("(child_type = 'workspace' AND child_id = ?) OR (parent_type = 'workspace' AND parent_id = ?)", id, id).
		Exec(ctx)
	return err
}
