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

	return db.insertEnvironmentWithExecutor(ctx, db.DB, workspaceID, name, description)
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

	var env Environment
	err := db.NewSelect().
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
)
SELECT DISTINCT e.*
FROM environments e
WHERE e.workspace_id = ?
  AND (
    ` + discoveryRoleBindingExists("rb", "rp", "org", "?", environmentDiscoveryPermissionExpr) + `
    OR ` + discoveryRoleBindingExists("rb2", "rp", "workspace", "?", environmentDiscoveryPermissionExpr) + `
    OR ` + discoveryRoleBindingExists("rb3", "rp", "environment", "e.id", environmentDiscoveryPermissionExpr) + `
    OR EXISTS (
        SELECT 1
        FROM connections c
        WHERE c.environment_id = e.id
          AND ` + discoveryRoleBindingExists("rb4", "rp", "connection", "c.id", environmentDiscoveryPermissionExpr) + `
    )
  )
ORDER BY e.name ASC`

	var envs []Environment
	err := db.NewRaw(q,
		accountID,   // my_teams CTE
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
)
SELECT EXISTS (
    SELECT 1
    FROM environments e
    WHERE e.id = ?
      AND e.workspace_id = ?
      AND (
        ` + discoveryRoleBindingExists("rb", "rp", "org", "?", environmentDiscoveryPermissionExpr) + `
        OR ` + discoveryRoleBindingExists("rb2", "rp", "workspace", "?", environmentDiscoveryPermissionExpr) + `
        OR ` + discoveryRoleBindingExists("rb3", "rp", "environment", "e.id", environmentDiscoveryPermissionExpr) + `
        OR EXISTS (
            SELECT 1
            FROM connections c
            WHERE c.environment_id = e.id
              AND ` + discoveryRoleBindingExists("rb4", "rp", "connection", "c.id", environmentDiscoveryPermissionExpr) + `
        )
      )
)`

	var ok bool
	err := db.NewRaw(q,
		accountID,
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

	connCount, err := db.NewSelect().
		TableExpr("connections").
		Where("environment_id = ?", id).
		Count(ctx)
	if err != nil {
		return err
	}
	if connCount > 0 {
		return ErrEnvironmentHasConnections
	}

	_, err = db.NewDelete().Model((*Environment)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	// Remove the environment's own hierarchy row.
	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where("child_type = 'environment' AND child_id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
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
