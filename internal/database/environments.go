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

type Environment struct {
	ID          int64     `bun:",pk,autoincrement" json:"id"`
	WorkspaceID int64     `bun:",notnull"          json:"workspace_id"`
	Name        string    `bun:",notnull"          json:"name"`
	Description string    `bun:",nullzero"         json:"description,omitempty"`
	CreatedAt   time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"          json:"updated_at"`
}

type ListEnvironmentsParams struct {
	WorkspaceID int64
	Search      string
	Name        string
	Sort        string
	Order       string
	Page        int
	PageSize    int
}

func (db *DB) InsertEnvironment(ctx context.Context, workspaceID int64, name, description string) (Environment, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var env Environment
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		env, err = db.insertEnvironmentWithExecutor(ctx, tx, workspaceID, name, description)
		return err
	})
	if err != nil {
		return Environment{}, err
	}
	return env, nil
}

func (db *DB) insertEnvironmentWithExecutor(ctx context.Context, exec bun.IDB, workspaceID int64, name, description string) (Environment, error) {
	env := Environment{
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_, err := exec.NewInsert().Model(&env).Returning("id").Exec(ctx)
	if err != nil {
		return Environment{}, err
	}

	hierarchyOwnerType, hierarchyOwnerID, err := db.workspaceHierarchyOwnerWithExecutor(ctx, exec, workspaceID)
	if err != nil {
		return Environment{}, err
	}
	hm := map[string]interface{}{
		"child_type":  "environment",
		"child_id":    env.ID,
		"parent_type": "workspace",
		"parent_id":   workspaceID,
		"owner_type":  hierarchyOwnerType,
		"owner_id":    hierarchyOwnerID,
	}
	_, err = exec.NewInsert().TableExpr("resource_hierarchy").Model(&hm).Ignore().Exec(ctx)
	if err != nil {
		return Environment{}, err
	}

	return env, nil
}

func (db *DB) DefaultEnvironmentID(ctx context.Context, workspaceID int64) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.defaultEnvironmentIDWithExecutor(ctx, db.DB, workspaceID)
}

func (db *DB) defaultEnvironmentIDWithExecutor(ctx context.Context, exec bun.IDB, workspaceID int64) (int64, error) {
	var env Environment
	err := exec.NewSelect().
		Model(&env).
		Where("workspace_id = ? AND name = 'Default'", workspaceID).
		Scan(ctx)
	if err != nil {
		return 0, err
	}
	return env.ID, nil
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

func (db *DB) ListEnvironmentsPage(ctx context.Context, params ListEnvironmentsParams) (response.Paginated[Environment], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeEnvironmentListParams(params)

	query := db.NewSelect().Model((*Environment)(nil)).
		Where("workspace_id = ?", params.WorkspaceID)
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("LOWER(name) LIKE ?", search)
	}
	if params.Name != "" {
		query = query.Where("name = ?", params.Name)
	}

	var envs []Environment
	err := query.OrderExpr(fmt.Sprintf("%s %s, id %s", environmentSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).
		Scan(ctx, &envs)
	if err != nil {
		return response.Paginated[Environment]{}, err
	}
	return response.PaginateItems(envs, params.Page, params.PageSize), nil
}

// ListAccessibleEnvironments returns environments in workspaceID that accountID can discover.
// Discovery includes direct environment access and ancestor visibility propagated from
// accessible descendant connections.
func (db *DB) ListAccessibleEnvironments(ctx context.Context, accountID, orgID, workspaceID int64) ([]Environment, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	q := `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
),
my_org_memberships AS (
    SELECT org_id FROM org_members WHERE account_id = ?
),
my_workspace_memberships AS (
    SELECT wm.workspace_id
    FROM workspace_members wm
    JOIN workspaces wm_w ON wm_w.id = wm.workspace_id
    JOIN org_members wm_om ON wm_om.org_id = wm_w.owner_id AND wm_om.account_id = wm.account_id
    WHERE wm.account_id = ? AND wm_w.owner_type = 'org' AND wm_w.owner_id = ?
  UNION
    SELECT wt.workspace_id
    FROM workspace_teams wt
    JOIN team_members tm ON tm.team_id = wt.team_id
    JOIN workspaces wt_w ON wt_w.id = wt.workspace_id
    JOIN org_members wt_om ON wt_om.org_id = wt_w.owner_id AND wt_om.account_id = tm.account_id
    WHERE tm.account_id = ? AND wt_w.owner_type = 'org' AND wt_w.owner_id = ?
)
SELECT DISTINCT e.*
FROM environments e
WHERE e.workspace_id = ?
  AND (
    ` + discoveryRoleBindingExists("rb", "r", "rp", "org", "?", environmentDiscoveryOrgPermissionExpr) + `
    OR ` + discoveryRoleBindingExists("rb2", "r2", "rp", "workspace", "?", environmentDiscoveryWorkspacePermissionExpr) + `
    OR ` + discoveryRoleBindingExists("rb3", "r3", "rp", "environment", "e.id", environmentDiscoveryEnvironmentPermissionExpr) + `
    OR EXISTS (
        SELECT 1
        FROM connections c
        WHERE c.environment_id = e.id
          AND ` + discoveryRoleBindingExists("rb4", "r4", "rp", "connection", "c.id", environmentDiscoveryConnectionPermissionExpr) + `
    )
  )
ORDER BY e.name ASC`

	var envs []Environment
	err := db.NewRaw(q,
		accountID,                          // my_teams CTE
		accountID,                          // my_org_memberships CTE
		accountID, orgID, accountID, orgID, // my_workspace_memberships CTE
		workspaceID, // e.workspace_id
		orgID, orgID, accountID,
		orgID, workspaceID, accountID,
		orgID, accountID,
		orgID, accountID,
	).Scan(ctx, &envs)
	return envs, err
}

func (db *DB) HasAccessibleEnvironment(ctx context.Context, accountID, orgID, workspaceID, environmentID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	q := `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
),
my_org_memberships AS (
    SELECT org_id FROM org_members WHERE account_id = ?
),
my_workspace_memberships AS (
    SELECT wm.workspace_id
    FROM workspace_members wm
    JOIN workspaces wm_w ON wm_w.id = wm.workspace_id
    JOIN org_members wm_om ON wm_om.org_id = wm_w.owner_id AND wm_om.account_id = wm.account_id
    WHERE wm.account_id = ? AND wm_w.owner_type = 'org' AND wm_w.owner_id = ?
  UNION
    SELECT wt.workspace_id
    FROM workspace_teams wt
    JOIN team_members tm ON tm.team_id = wt.team_id
    JOIN workspaces wt_w ON wt_w.id = wt.workspace_id
    JOIN org_members wt_om ON wt_om.org_id = wt_w.owner_id AND wt_om.account_id = tm.account_id
    WHERE tm.account_id = ? AND wt_w.owner_type = 'org' AND wt_w.owner_id = ?
)
SELECT EXISTS (
    SELECT 1
    FROM environments e
    WHERE e.id = ?
      AND e.workspace_id = ?
      AND (
        ` + discoveryRoleBindingExists("rb", "r", "rp", "org", "?", environmentDiscoveryOrgPermissionExpr) + `
        OR ` + discoveryRoleBindingExists("rb2", "r2", "rp", "workspace", "?", environmentDiscoveryWorkspacePermissionExpr) + `
        OR ` + discoveryRoleBindingExists("rb3", "r3", "rp", "environment", "e.id", environmentDiscoveryEnvironmentPermissionExpr) + `
        OR EXISTS (
            SELECT 1
            FROM connections c
            WHERE c.environment_id = e.id
              AND ` + discoveryRoleBindingExists("rb4", "r4", "rp", "connection", "c.id", environmentDiscoveryConnectionPermissionExpr) + `
        )
      )
)`

	var ok bool
	err := db.NewRaw(q,
		accountID,
		accountID,
		accountID, orgID, accountID, orgID,
		environmentID, workspaceID,
		orgID, orgID, accountID,
		orgID, workspaceID, accountID,
		orgID, accountID,
		orgID, accountID,
	).Scan(ctx, &ok)
	return ok, err
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

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		connCount, err := tx.NewSelect().
			TableExpr("connections").
			Where("environment_id = ?", id).
			Count(ctx)
		if err != nil {
			return err
		}
		if connCount > 0 {
			return ErrEnvironmentHasConnections
		}

		if _, err = tx.NewDelete().Model((*Environment)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
			return err
		}

		// Remove the environment's own hierarchy row.
		_, err = tx.NewDelete().TableExpr("resource_hierarchy").
			Where("child_type = 'environment' AND child_id = ?", id).
			Exec(ctx)
		return err
	})
}

func normalizeEnvironmentListParams(params ListEnvironmentsParams) ListEnvironmentsParams {
	if params.Sort == "" {
		params.Sort = "created_at"
	}
	switch params.Sort {
	case "name", "created_at":
	default:
		params.Sort = "created_at"
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

func environmentSortColumn(sort string) string {
	switch sort {
	case "name":
		return "name"
	default:
		return "created_at"
	}
}
