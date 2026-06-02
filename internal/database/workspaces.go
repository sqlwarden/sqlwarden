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
	ID               int64     `bun:",pk,autoincrement" json:"id"`
	OrgID            *int64    `bun:",nullzero"         json:"org_id,omitempty"`
	OwnerType        string    `bun:",notnull"          json:"owner_type"`
	OwnerID          int64     `bun:",notnull"          json:"owner_id"`
	Name             string    `bun:",notnull"          json:"name"`
	Description      string    `bun:",nullzero"         json:"description,omitempty"`
	EnvironmentCount int       `bun:"-"                 json:"environment_count"`
	ConnectionCount  int       `bun:"-"                 json:"connection_count"`
	CreatedAt        time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt        time.Time `bun:",notnull"          json:"updated_at"`
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
	result := response.PaginateItems(workspaces, params.Page, params.PageSize)
	if err = db.PopulateWorkspaceCounts(ctx, result.Items); err != nil {
		return response.Paginated[Workspace]{}, err
	}
	return result, nil
}

// PopulateWorkspaceCounts adds environment and connection counts for the provided
// page of workspaces. It is intentionally page-bounded to avoid global aggregates.
func (db *DB) PopulateWorkspaceCounts(ctx context.Context, workspaces []Workspace) error {
	if len(workspaces) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(workspaces))
	for _, workspace := range workspaces {
		ids = append(ids, workspace.ID)
	}

	var envCounts []struct {
		WorkspaceID int64 `bun:"workspace_id"`
		Count       int   `bun:"count"`
	}
	if err := db.NewSelect().
		TableExpr("environments").
		ColumnExpr("workspace_id, COUNT(*) AS count").
		Where("workspace_id IN (?)", bun.List(ids)).
		GroupExpr("workspace_id").
		Scan(ctx, &envCounts); err != nil {
		return err
	}

	var connCounts []struct {
		WorkspaceID int64 `bun:"workspace_id"`
		Count       int   `bun:"count"`
	}
	if err := db.NewSelect().
		TableExpr("connections").
		ColumnExpr("workspace_id, COUNT(*) AS count").
		Where("workspace_id IN (?)", bun.List(ids)).
		GroupExpr("workspace_id").
		Scan(ctx, &connCounts); err != nil {
		return err
	}

	countsByWorkspace := make(map[int64]struct {
		environments int
		connections  int
	}, len(workspaces))
	for _, row := range envCounts {
		counts := countsByWorkspace[row.WorkspaceID]
		counts.environments = row.Count
		countsByWorkspace[row.WorkspaceID] = counts
	}
	for _, row := range connCounts {
		counts := countsByWorkspace[row.WorkspaceID]
		counts.connections = row.Count
		countsByWorkspace[row.WorkspaceID] = counts
	}

	for index := range workspaces {
		counts := countsByWorkspace[workspaces[index].ID]
		workspaces[index].EnvironmentCount = counts.environments
		workspaces[index].ConnectionCount = counts.connections
	}

	return nil
}

// ListAccessibleWorkspaces returns workspaces within orgID that accountID can discover.
// Discovery includes direct workspace access and ancestor visibility propagated from
// accessible descendant environments and connections.
func (db *DB) ListAccessibleWorkspaces(ctx context.Context, accountID, orgID int64) ([]Workspace, error) {
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
SELECT DISTINCT w.*
FROM workspaces w
WHERE w.owner_type = 'org' AND w.owner_id = ?
  AND (
    ` + discoveryRoleBindingExists("rb", "r", "rp", "org", "?", workspaceDiscoveryOrgPermissionExpr) + `
    OR ` + discoveryRoleBindingExists("rb2", "r2", "rp", "workspace", "w.id", workspaceDiscoveryWorkspacePermissionExpr) + `
    OR EXISTS (
        SELECT 1
        FROM environments e
        WHERE e.workspace_id = w.id
          AND ` + discoveryRoleBindingExists("rb3", "r3", "rp", "environment", "e.id", workspaceDiscoveryEnvironmentPermissionExpr) + `
    )
    OR EXISTS (
        SELECT 1
        FROM connections c
        WHERE c.workspace_id = w.id
          AND ` + discoveryRoleBindingExists("rb4", "r4", "rp", "connection", "c.id", workspaceDiscoveryConnectionPermissionExpr) + `
    )
  )
ORDER BY w.name ASC`

	var wss []Workspace
	err := db.NewRaw(q,
		accountID,                          // my_teams CTE
		accountID,                          // my_org_memberships CTE
		accountID, orgID, accountID, orgID, // my_workspace_memberships CTE
		orgID, // w.owner_id
		orgID, orgID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
	).Scan(ctx, &wss)
	return wss, err
}

func (db *DB) HasAccessibleWorkspace(ctx context.Context, accountID, orgID, workspaceID int64) (bool, error) {
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
    FROM workspaces w
    WHERE w.id = ?
      AND w.owner_type = 'org'
      AND w.owner_id = ?
      AND (
        ` + discoveryRoleBindingExists("rb", "r", "rp", "org", "?", workspaceDiscoveryOrgPermissionExpr) + `
        OR ` + discoveryRoleBindingExists("rb2", "r2", "rp", "workspace", "w.id", workspaceDiscoveryWorkspacePermissionExpr) + `
        OR EXISTS (
            SELECT 1
            FROM environments e
            WHERE e.workspace_id = w.id
              AND ` + discoveryRoleBindingExists("rb3", "r3", "rp", "environment", "e.id", workspaceDiscoveryEnvironmentPermissionExpr) + `
        )
        OR EXISTS (
            SELECT 1
            FROM connections c
            WHERE c.workspace_id = w.id
              AND ` + discoveryRoleBindingExists("rb4", "r4", "rp", "connection", "c.id", workspaceDiscoveryConnectionPermissionExpr) + `
        )
      )
)`

	var ok bool
	err := db.NewRaw(q,
		accountID,
		accountID,
		accountID, orgID, accountID, orgID,
		workspaceID, orgID,
		orgID, orgID, accountID,
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

	// Role bindings are policy assignments, so clean up bindings that target
	// this workspace or its children before workspace-scoped roles cascade.
	if _, err := db.NewDelete().TableExpr("role_bindings").
		Where(`(resource_type = 'workspace' AND resource_id = ?)
		    OR (resource_type = 'environment' AND resource_id IN (SELECT id FROM environments WHERE workspace_id = ?))
		    OR (resource_type = 'connection' AND resource_id IN (SELECT id FROM connections WHERE workspace_id = ?))
		    OR role_id IN (SELECT id FROM roles WHERE workspace_id = ?)`, id, id, id, id).
		Exec(ctx); err != nil {
		return err
	}

	// Clean up hierarchy rows for this workspace and all its children.
	// resource_hierarchy has no FK constraints so we must do this manually.
	if _, err := db.NewDelete().TableExpr("resource_hierarchy").
		Where(`(child_type = 'workspace' AND child_id = ?)
		    OR (parent_type = 'workspace' AND parent_id = ?)
		    OR (child_type = 'connection' AND parent_type = 'environment'
		        AND child_id IN (SELECT id FROM connections WHERE workspace_id = ?))`, id, id, id).
		Exec(ctx); err != nil {
		return err
	}

	_, err := db.NewDelete().Model((*Workspace)(nil)).Where("id = ?", id).Exec(ctx)
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
