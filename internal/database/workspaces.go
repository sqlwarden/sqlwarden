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

func (db *DB) InsertWorkspace(ctx context.Context, orgID *int64, ownerType string, ownerID int64, name, description string) (Workspace, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
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

func (db *DB) GetWorkspace(ctx context.Context, id int64) (Workspace, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
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

func (db *DB) ListWorkspacesByOwner(ctx context.Context, ownerType string, ownerID int64) ([]Workspace, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var wss []Workspace
	err := db.NewSelect().Model(&wss).
		Where("owner_type = ? AND owner_id = ?", ownerType, ownerID).
		OrderExpr("name ASC").
		Scan(ctx)
	return wss, err
}

// ListAccessibleWorkspaces returns workspaces within orgID that accountID has any binding on,
// either at the org level (all workspaces visible) or directly at the workspace level.
func (db *DB) ListAccessibleWorkspaces(ctx context.Context, accountID, orgID int64) ([]Workspace, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	const q = `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
)
SELECT DISTINCT w.*
FROM workspaces w
WHERE w.owner_type = 'org' AND w.owner_id = ?
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
        WHERE rb2.org_id = ? AND rb2.resource_type = 'workspace' AND rb2.resource_id = w.id
          AND (
            (rb2.subject_type = 'account' AND rb2.subject_id = ?)
            OR (rb2.subject_type = 'team' AND rb2.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM permission_bindings pb2
        WHERE pb2.org_id = ? AND pb2.resource_type = 'workspace' AND pb2.resource_id = w.id
          AND (
            (pb2.subject_type = 'account' AND pb2.subject_id = ?)
            OR (pb2.subject_type = 'team' AND pb2.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
  )
ORDER BY w.name ASC`

	var wss []Workspace
	err := db.NewRaw(q,
		accountID,               // my_teams CTE
		orgID,                   // w.owner_id
		orgID, orgID, accountID, // org role binding
		orgID, orgID, accountID, // org perm binding
		orgID, accountID, // ws role binding
		orgID, accountID, // ws perm binding
	).Scan(ctx, &wss)
	return wss, err
}

func (db *DB) UpdateWorkspace(ctx context.Context, id int64, name, description string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().Model((*Workspace)(nil)).
		Set("name = ?", name).
		Set("description = ?", description).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (db *DB) DeleteWorkspace(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*Workspace)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	// Clean up hierarchy rows for this workspace and all its children.
	// resource_hierarchy has no FK constraints so we must do this manually.
	//
	// Covers:
	//   (workspace, id)        → its own hierarchy row
	//   (environment, *)       → rows whose parent is this workspace
	//   (connection, *)        → rows whose parent is this workspace (untagged connections)
	//   (connection, *)        → rows whose parent is an environment in this workspace
	//                            (tagged connections write a second row with parent=environment)
	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where(`(child_type = 'workspace' AND child_id = ?)
		    OR (parent_type = 'workspace' AND parent_id = ?)
		    OR (child_type = 'connection' AND parent_type = 'environment'
		        AND child_id IN (SELECT id FROM connections WHERE workspace_id = ?))`,
			id, id, id).
		Exec(ctx)
	return err
}
