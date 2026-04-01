package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Connection struct {
	ID            int64     `bun:",pk,autoincrement" json:"id"`
	WorkspaceID   int64     `bun:",notnull"          json:"workspace_id"`
	EnvironmentID *int64    `bun:",nullzero"         json:"environment_id,omitempty"`
	OrgID         *int64    `bun:",nullzero"         json:"org_id,omitempty"`
	OwnerType     string    `bun:",notnull"          json:"owner_type"`
	OwnerID       int64     `bun:",notnull"          json:"owner_id"`
	Name          string    `bun:",notnull"          json:"name"`
	Driver        string    `bun:",notnull"          json:"driver"`
	DSNEncrypted  string    `bun:",notnull"          json:"-"`
	AccessMode    string    `bun:",notnull,default:'open'" json:"access_mode"`
	CreatedAt     time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt     time.Time `bun:",notnull"          json:"updated_at"`
}

func (db *DB) InsertConnection(workspaceID int64, envID, orgID *int64, ownerType string, ownerID int64, name, driver, dsnEncrypted, accessMode string) (Connection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	conn := Connection{
		WorkspaceID:   workspaceID,
		EnvironmentID: envID,
		OrgID:         orgID,
		OwnerType:     ownerType,
		OwnerID:       ownerID,
		Name:          name,
		Driver:        driver,
		DSNEncrypted:  dsnEncrypted,
		AccessMode:    accessMode,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	_, err := db.NewInsert().Model(&conn).Returning("id").Exec(ctx)
	if err != nil {
		return Connection{}, err
	}

	ownerIDForHierarchy := ownerID
	if orgID != nil {
		ownerIDForHierarchy = *orgID
	}
	hm := map[string]interface{}{
		"child_type":  "connection",
		"child_id":    conn.ID,
		"parent_type": "workspace",
		"parent_id":   workspaceID,
		"owner_type":  "org",
		"owner_id":    ownerIDForHierarchy,
	}
	_, err = db.NewInsert().TableExpr("resource_hierarchy").Model(&hm).Ignore().Exec(ctx)
	if err != nil {
		return Connection{}, err
	}

	return conn, nil
}

func (db *DB) GetConnection(id int64) (Connection, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var conn Connection
	err := db.NewSelect().Model(&conn).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, false, nil
	}
	if err != nil {
		return Connection{}, false, err
	}
	return conn, true, nil
}

func (db *DB) ListConnections(workspaceID int64) ([]Connection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var conns []Connection
	err := db.NewSelect().Model(&conns).
		Where("workspace_id = ?", workspaceID).
		OrderExpr("name ASC").
		Scan(ctx)
	return conns, err
}

func (db *DB) DeleteConnection(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*Connection)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where("child_type = 'connection' AND child_id = ?", id).
		Exec(ctx)
	return err
}
