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

func (db *DB) InsertConnection(ctx context.Context, workspaceID int64, envID, orgID *int64, ownerType string, ownerID int64, name, driver, dsnEncrypted, accessMode string) (Connection, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
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

	hierarchyOwnerType := ownerType
	hierarchyOwnerID := ownerID
	if ownerType == "org" && orgID != nil {
		hierarchyOwnerID = *orgID
	}
	hm := map[string]interface{}{
		"child_type":  "connection",
		"child_id":    conn.ID,
		"parent_type": "workspace",
		"parent_id":   workspaceID,
		"owner_type":  hierarchyOwnerType,
		"owner_id":    hierarchyOwnerID,
	}
	_, err = db.NewInsert().TableExpr("resource_hierarchy").Model(&hm).Ignore().Exec(ctx)
	if err != nil {
		return Connection{}, err
	}

	return conn, nil
}

func (db *DB) GetConnection(ctx context.Context, id int64) (Connection, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
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

func (db *DB) ListConnections(ctx context.Context, workspaceID int64) ([]Connection, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var conns []Connection
	err := db.NewSelect().Model(&conns).
		Where("workspace_id = ?", workspaceID).
		OrderExpr("name ASC").
		Scan(ctx)
	return conns, err
}

// ListAccessibleConnections returns connections in workspaceID that accountID has any binding on,
// checking org-level, workspace-level, and direct connection-level bindings.
func (db *DB) ListAccessibleConnections(ctx context.Context, accountID, orgID, workspaceID int64) ([]Connection, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	const q = `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
)
SELECT DISTINCT c.*
FROM connections c
WHERE c.workspace_id = ?
  AND (
    EXISTS (
        SELECT 1 FROM role_bindings rb
        WHERE rb.org_id = ? AND rb.resource_type = 'org' AND rb.resource_id = ?
          AND (
            (rb.subject_type = 'account' AND rb.subject_id = ?)
            OR (rb.subject_type = 'team' AND rb.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM permission_bindings pb
        WHERE pb.org_id = ? AND pb.resource_type = 'org' AND pb.resource_id = ?
          AND (
            (pb.subject_type = 'account' AND pb.subject_id = ?)
            OR (pb.subject_type = 'team' AND pb.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM role_bindings rb2
        WHERE rb2.org_id = ? AND rb2.resource_type = 'workspace' AND rb2.resource_id = ?
          AND (
            (rb2.subject_type = 'account' AND rb2.subject_id = ?)
            OR (rb2.subject_type = 'team' AND rb2.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM permission_bindings pb2
        WHERE pb2.org_id = ? AND pb2.resource_type = 'workspace' AND pb2.resource_id = ?
          AND (
            (pb2.subject_type = 'account' AND pb2.subject_id = ?)
            OR (pb2.subject_type = 'team' AND pb2.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM role_bindings rb3
        WHERE rb3.org_id = ? AND rb3.resource_type = 'connection' AND rb3.resource_id = c.id
          AND (
            (rb3.subject_type = 'account' AND rb3.subject_id = ?)
            OR (rb3.subject_type = 'team' AND rb3.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM permission_bindings pb3
        WHERE pb3.org_id = ? AND pb3.resource_type = 'connection' AND pb3.resource_id = c.id
          AND (
            (pb3.subject_type = 'account' AND pb3.subject_id = ?)
            OR (pb3.subject_type = 'team' AND pb3.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
  )
ORDER BY c.name ASC`

	var conns []Connection
	err := db.NewRaw(q,
		accountID,               // my_teams CTE
		workspaceID,             // c.workspace_id
		orgID, orgID, accountID, // org role binding
		orgID, orgID, accountID, // org perm binding
		orgID, workspaceID, accountID, // ws role binding
		orgID, workspaceID, accountID, // ws perm binding
		orgID, accountID, // conn role binding
		orgID, accountID, // conn perm binding
	).Scan(ctx, &conns)
	return conns, err
}

func (db *DB) DeleteConnection(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
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
