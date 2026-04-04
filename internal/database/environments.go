package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
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

type ListEnvironmentsParams struct {
	WorkspaceID int64
	Search      string
	Name        string
	Sort        string
	Order       string
	Page        int
	PageSize    int
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

func (db *DB) listEnvironmentsFiltered(ctx context.Context, params ListEnvironmentsParams) ([]Environment, error) {
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
	return envs, err
}

func (db *DB) ListEnvironmentsPage(ctx context.Context, params ListEnvironmentsParams) (response.Paginated[Environment], error) {
	envs, err := db.listEnvironmentsFiltered(ctx, params)
	if err != nil {
		return response.Paginated[Environment]{}, err
	}
	return response.PaginateItems(envs, params.Page, params.PageSize), nil
}

// ListAccessibleEnvironments returns environments in workspaceID that accountID has any binding on,
// checking org-level, workspace-level, and direct environment-level bindings.
func (db *DB) ListAccessibleEnvironments(ctx context.Context, accountID, orgID, workspaceID int64) ([]Environment, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	const q = `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
)
SELECT DISTINCT e.*
FROM environments e
WHERE e.workspace_id = ?
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
        WHERE rb3.org_id = ? AND rb3.resource_type = 'environment' AND rb3.resource_id = e.id
          AND (
            (rb3.subject_type = 'account' AND rb3.subject_id = ?)
            OR (rb3.subject_type = 'team' AND rb3.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM permission_bindings pb3
        WHERE pb3.org_id = ? AND pb3.resource_type = 'environment' AND pb3.resource_id = e.id
          AND (
            (pb3.subject_type = 'account' AND pb3.subject_id = ?)
            OR (pb3.subject_type = 'team' AND pb3.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
  )
ORDER BY e.name ASC`

	var envs []Environment
	err := db.NewRaw(q,
		accountID,               // my_teams CTE
		workspaceID,             // e.workspace_id
		orgID, orgID, accountID, // org role binding
		orgID, orgID, accountID, // org perm binding
		orgID, workspaceID, accountID, // ws role binding
		orgID, workspaceID, accountID, // ws perm binding
		orgID, accountID, // env role binding
		orgID, accountID, // env perm binding
	).Scan(ctx, &envs)
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

	// FK: connections.environment_id ON DELETE SET NULL — connections survive but lose their env tag.
	_, err := db.NewDelete().Model((*Environment)(nil)).Where("id = ?", id).Exec(ctx)
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

	// Remove connection → environment hierarchy rows for connections that were in this environment.
	// Those connections remain (SET NULL) and still have their connection → workspace row, so they
	// continue to inherit workspace-scope and org-scope bindings.
	_, err = db.NewDelete().TableExpr("resource_hierarchy").
		Where("child_type = 'connection' AND parent_type = 'environment' AND parent_id = ?", id).
		Exec(ctx)
	return err
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
