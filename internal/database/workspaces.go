package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
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

type ListWorkspacesParams struct {
	OwnerType string
	OwnerID   int64
	Search    string
	Name      string
	Sort      string
	Order     string
	Page      int
	PageSize  int
}

func (db *DB) InsertWorkspace(ctx context.Context, orgID *int64, ownerType string, ownerID int64, name, description string) (Workspace, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ws Workspace
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		ws = Workspace{
			OrgID:       orgID,
			OwnerType:   ownerType,
			OwnerID:     ownerID,
			Name:        name,
			Description: description,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		_, err := tx.NewInsert().Model(&ws).Returning("id").Exec(ctx)
		if err != nil {
			return err
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
			_, err = tx.NewInsert().TableExpr("resource_hierarchy").Model(&hm).Ignore().Exec(ctx)
			if err != nil {
				return err
			}
		}

		_, err = db.insertEnvironmentWithExecutor(ctx, tx, ws.ID, "Default", "")
		return err
	})
	if err != nil {
		return Workspace{}, err
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

func (db *DB) ListWorkspacesPage(ctx context.Context, params ListWorkspacesParams) (response.Paginated[Workspace], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeWorkspaceListParams(params)

	query := db.NewSelect().Model((*Workspace)(nil)).
		Where("owner_type = ? AND owner_id = ?", params.OwnerType, params.OwnerID)
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("LOWER(name) LIKE ?", search)
	}
	if params.Name != "" {
		query = query.Where("name = ?", params.Name)
	}

	var workspaces []Workspace
	err := query.OrderExpr(fmt.Sprintf("%s %s, id %s", workspaceSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).Scan(ctx, &workspaces)
	if err != nil {
		return response.Paginated[Workspace]{}, err
	}
	return response.PaginateItems(workspaces, params.Page, params.PageSize), nil
}

// ListAccessibleWorkspaces returns workspaces within orgID that accountID can discover.
// Discovery includes direct workspace access and ancestor visibility propagated from
// accessible descendant environments and connections.
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
    OR EXISTS (
        SELECT 1
        FROM environments e
        WHERE e.workspace_id = w.id
          AND EXISTS (
            SELECT 1 FROM role_bindings rb3
            WHERE rb3.org_id = ? AND rb3.resource_type = 'environment' AND rb3.resource_id = e.id
              AND (
                (rb3.subject_type = 'account' AND rb3.subject_id = ?)
                OR (rb3.subject_type = 'team' AND rb3.subject_id IN (SELECT team_id FROM my_teams))
              )
          )
    )
    OR EXISTS (
        SELECT 1
        FROM environments e
        WHERE e.workspace_id = w.id
          AND EXISTS (
            SELECT 1 FROM permission_bindings pb3
            WHERE pb3.org_id = ? AND pb3.resource_type = 'environment' AND pb3.resource_id = e.id
              AND (
                (pb3.subject_type = 'account' AND pb3.subject_id = ?)
                OR (pb3.subject_type = 'team' AND pb3.subject_id IN (SELECT team_id FROM my_teams))
              )
          )
    )
    OR EXISTS (
        SELECT 1
        FROM connections c
        WHERE c.workspace_id = w.id
          AND EXISTS (
            SELECT 1 FROM role_bindings rb4
            WHERE rb4.org_id = ? AND rb4.resource_type = 'connection' AND rb4.resource_id = c.id
              AND (
                (rb4.subject_type = 'account' AND rb4.subject_id = ?)
                OR (rb4.subject_type = 'team' AND rb4.subject_id IN (SELECT team_id FROM my_teams))
              )
          )
    )
    OR EXISTS (
        SELECT 1
        FROM connections c
        WHERE c.workspace_id = w.id
          AND EXISTS (
            SELECT 1 FROM permission_bindings pb4
            WHERE pb4.org_id = ? AND pb4.resource_type = 'connection' AND pb4.resource_id = c.id
              AND (
                (pb4.subject_type = 'account' AND pb4.subject_id = ?)
                OR (pb4.subject_type = 'team' AND pb4.subject_id IN (SELECT team_id FROM my_teams))
              )
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
		orgID, accountID, // env role binding
		orgID, accountID, // env perm binding
		orgID, accountID, // conn role binding
		orgID, accountID, // conn perm binding
	).Scan(ctx, &wss)
	return wss, err
}

func (db *DB) HasAccessibleWorkspace(ctx context.Context, accountID, orgID, workspaceID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	const q = `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
)
SELECT EXISTS (
    SELECT 1
    FROM workspaces w
    WHERE w.id = ?
      AND w.owner_type = 'org'
      AND w.owner_id = ?
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
        OR EXISTS (
            SELECT 1
            FROM environments e
            WHERE e.workspace_id = w.id
              AND EXISTS (
                SELECT 1 FROM role_bindings rb3
                WHERE rb3.org_id = ? AND rb3.resource_type = 'environment' AND rb3.resource_id = e.id
                  AND (
                    (rb3.subject_type = 'account' AND rb3.subject_id = ?)
                    OR (rb3.subject_type = 'team' AND rb3.subject_id IN (SELECT team_id FROM my_teams))
                  )
              )
        )
        OR EXISTS (
            SELECT 1
            FROM environments e
            WHERE e.workspace_id = w.id
              AND EXISTS (
                SELECT 1 FROM permission_bindings pb3
                WHERE pb3.org_id = ? AND pb3.resource_type = 'environment' AND pb3.resource_id = e.id
                  AND (
                    (pb3.subject_type = 'account' AND pb3.subject_id = ?)
                    OR (pb3.subject_type = 'team' AND pb3.subject_id IN (SELECT team_id FROM my_teams))
                  )
              )
        )
        OR EXISTS (
            SELECT 1
            FROM connections c
            WHERE c.workspace_id = w.id
              AND EXISTS (
                SELECT 1 FROM role_bindings rb4
                WHERE rb4.org_id = ? AND rb4.resource_type = 'connection' AND rb4.resource_id = c.id
                  AND (
                    (rb4.subject_type = 'account' AND rb4.subject_id = ?)
                    OR (rb4.subject_type = 'team' AND rb4.subject_id IN (SELECT team_id FROM my_teams))
                  )
              )
        )
        OR EXISTS (
            SELECT 1
            FROM connections c
            WHERE c.workspace_id = w.id
              AND EXISTS (
                SELECT 1 FROM permission_bindings pb4
                WHERE pb4.org_id = ? AND pb4.resource_type = 'connection' AND pb4.resource_id = c.id
                  AND (
                    (pb4.subject_type = 'account' AND pb4.subject_id = ?)
                    OR (pb4.subject_type = 'team' AND pb4.subject_id IN (SELECT team_id FROM my_teams))
                  )
              )
        )
      )
)`

	var ok bool
	err := db.NewRaw(q,
		accountID,
		workspaceID, orgID,
		orgID, orgID, accountID,
		orgID, orgID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
	).Scan(ctx, &ok)
	return ok, err
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
	//   (connection, *)        → rows whose parent is an environment in this workspace
	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where(`(child_type = 'workspace' AND child_id = ?)
		    OR (parent_type = 'workspace' AND parent_id = ?)
		    OR (child_type = 'connection' AND parent_type = 'environment'
		        AND child_id IN (SELECT id FROM connections WHERE workspace_id = ?))`,
			id, id, id).
		Exec(ctx)
	return err
}

func normalizeWorkspaceListParams(params ListWorkspacesParams) ListWorkspacesParams {
	if params.OwnerType == "" {
		params.OwnerType = "org"
	}
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "created_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
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

func workspaceSortColumn(sort string) string {
	switch sort {
	case "created_at":
		return "created_at"
	default:
		return "name"
	}
}
