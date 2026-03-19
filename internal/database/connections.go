package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Connection struct {
	ID          string    `bun:",pk"      json:"id"`
	WorkspaceID string    `bun:",notnull" json:"workspace_id"`
	TenantID    string    `bun:",notnull" json:"tenant_id"`
	Name        string    `bun:",notnull" json:"name"`
	Driver      string    `bun:",notnull" json:"driver"`
	DSN         string    `bun:",notnull" json:"-"`
	CreatedAt   time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull" json:"updated_at"`
}

func (db *DB) InsertConnection(workspaceID, tenantID, name, driverName, encryptedDSN string) (Connection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	conn := Connection{
		ID:          newID(),
		WorkspaceID: workspaceID,
		TenantID:    tenantID,
		Name:        name,
		Driver:      driverName,
		DSN:         encryptedDSN,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := db.NewInsert().
		Model(&conn).
		Exec(ctx)
	if err != nil {
		return Connection{}, err
	}

	return conn, nil
}

func (db *DB) GetConnection(id string) (Connection, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var conn Connection
	err := db.NewSelect().
		Model(&conn).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, false, nil
	}
	if err != nil {
		return Connection{}, false, err
	}

	return conn, true, nil
}

func (db *DB) GetConnectionsByWorkspace(workspaceID string) ([]Connection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var conns []Connection
	err := db.NewSelect().
		Model(&conns).
		Where("workspace_id = ?", workspaceID).
		Order("created_at ASC").
		Scan(ctx)

	return conns, err
}

func (db *DB) DeleteConnection(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*Connection)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}
