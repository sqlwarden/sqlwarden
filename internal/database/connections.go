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

type Connection struct {
	ID            int64     `bun:",pk,autoincrement" json:"id"`
	WorkspaceID   int64     `bun:",notnull"          json:"workspace_id"`
	EnvironmentID int64     `bun:",notnull"          json:"environment_id"`
	Name          string    `bun:",notnull"          json:"name"`
	Driver        string    `bun:",notnull"          json:"driver"`
	DSNEncrypted  string    `bun:",notnull"          json:"-"`
	AccessMode    string    `bun:",notnull,default:'open'" json:"access_mode"`
	CreatedAt     time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt     time.Time `bun:",notnull"          json:"updated_at"`
}

type ListConnectionsParams struct {
	WorkspaceID   int64
	Search        string
	EnvironmentID *int64
	Driver        string
	AccessMode    string
	Sort          string
	Order         string
	Page          int
	PageSize      int
}

func (db *DB) InsertConnection(ctx context.Context, workspaceID int64, envID *int64, name, driver, dsnEncrypted, accessMode string) (Connection, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	resolvedEnvID := int64(0)
	if envID == nil {
		var err error
		resolvedEnvID, err = db.DefaultEnvironmentID(ctx, workspaceID)
		if err != nil {
			return Connection{}, err
		}
	} else {
		resolvedEnvID = *envID
	}

	conn := Connection{
		WorkspaceID:   workspaceID,
		EnvironmentID: resolvedEnvID,
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

	hierarchyOwnerType, hierarchyOwnerID, err := db.workspaceHierarchyOwner(ctx, workspaceID)
	if err != nil {
		return Connection{}, err
	}

	envRow := map[string]interface{}{
		"child_type":  "connection",
		"child_id":    conn.ID,
		"parent_type": "environment",
		"parent_id":   resolvedEnvID,
		"owner_type":  hierarchyOwnerType,
		"owner_id":    hierarchyOwnerID,
	}
	_, err = db.NewInsert().TableExpr("resource_hierarchy").Model(&envRow).Ignore().Exec(ctx)
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

// UpdateConnection updates only mutable connection fields.
// Workspace, environment, ownership, and driver are intentionally immutable.
func (db *DB) UpdateConnection(ctx context.Context, id int64, name, dsnEncrypted, accessMode string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().Model((*Connection)(nil)).
		Set("name = ?", name).
		Set("dsn_encrypted = ?", dsnEncrypted).
		Set("access_mode = ?", accessMode).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (db *DB) ListConnectionsPage(ctx context.Context, params ListConnectionsParams) (response.Paginated[Connection], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeConnectionListParams(params)

	query := db.NewSelect().Model((*Connection)(nil)).Where("workspace_id = ?", params.WorkspaceID)
	countQuery := db.NewSelect().Model((*Connection)(nil)).Where("workspace_id = ?", params.WorkspaceID)

	if params.Search != "" {
		searchTerm := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("LOWER(name) LIKE ?", searchTerm)
		countQuery = countQuery.Where("LOWER(name) LIKE ?", searchTerm)
	}
	if params.EnvironmentID != nil {
		query = query.Where("environment_id = ?", *params.EnvironmentID)
		countQuery = countQuery.Where("environment_id = ?", *params.EnvironmentID)
	}
	if params.Driver != "" {
		query = query.Where("driver = ?", params.Driver)
		countQuery = countQuery.Where("driver = ?", params.Driver)
	}
	if params.AccessMode != "" {
		query = query.Where("access_mode = ?", params.AccessMode)
		countQuery = countQuery.Where("access_mode = ?", params.AccessMode)
	}

	total, err := countQuery.Count(ctx)
	if err != nil {
		return response.Paginated[Connection]{}, err
	}

	var items []Connection
	err = query.
		OrderExpr(fmt.Sprintf("%s %s, id %s", connectionSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).
		Limit(params.PageSize).
		Offset((params.Page-1)*params.PageSize).
		Scan(ctx, &items)
	if err != nil {
		return response.Paginated[Connection]{}, err
	}

	return response.Paginated[Connection]{
		Items:    items,
		Page:     params.Page,
		PageSize: params.PageSize,
		Total:    total,
	}, nil
}

// ListAccessibleConnections returns connections in workspaceID that accountID can discover.
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
        SELECT 1 FROM role_bindings rb4
        WHERE rb4.org_id = ? AND rb4.resource_type = 'environment' AND rb4.resource_id = c.environment_id
          AND (
            (rb4.subject_type = 'account' AND rb4.subject_id = ?)
            OR (rb4.subject_type = 'team' AND rb4.subject_id IN (SELECT team_id FROM my_teams))
          )
    )
    OR EXISTS (
        SELECT 1 FROM permission_bindings pb4
        WHERE pb4.org_id = ? AND pb4.resource_type = 'environment' AND pb4.resource_id = c.environment_id
          AND (
            (pb4.subject_type = 'account' AND pb4.subject_id = ?)
            OR (pb4.subject_type = 'team' AND pb4.subject_id IN (SELECT team_id FROM my_teams))
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
		orgID, accountID, // env role binding
		orgID, accountID, // env perm binding
		orgID, accountID, // conn role binding
		orgID, accountID, // conn perm binding
	).Scan(ctx, &conns)
	return conns, err
}

func (db *DB) HasAccessibleConnection(ctx context.Context, accountID, orgID, workspaceID, connectionID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	const q = `
WITH my_teams AS (
    SELECT team_id FROM team_members WHERE account_id = ?
)
SELECT EXISTS (
    SELECT 1
    FROM connections c
    WHERE c.id = ?
      AND c.workspace_id = ?
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
            SELECT 1 FROM role_bindings rb4
            WHERE rb4.org_id = ? AND rb4.resource_type = 'environment' AND rb4.resource_id = c.environment_id
              AND (
                (rb4.subject_type = 'account' AND rb4.subject_id = ?)
                OR (rb4.subject_type = 'team' AND rb4.subject_id IN (SELECT team_id FROM my_teams))
              )
        )
        OR EXISTS (
            SELECT 1 FROM permission_bindings pb4
            WHERE pb4.org_id = ? AND pb4.resource_type = 'environment' AND pb4.resource_id = c.environment_id
              AND (
                (pb4.subject_type = 'account' AND pb4.subject_id = ?)
                OR (pb4.subject_type = 'team' AND pb4.subject_id IN (SELECT team_id FROM my_teams))
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
)`

	var ok bool
	err := db.NewRaw(q,
		accountID,
		connectionID, workspaceID,
		orgID, orgID, accountID,
		orgID, orgID, accountID,
		orgID, workspaceID, accountID,
		orgID, workspaceID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
		orgID, accountID,
	).Scan(ctx, &ok)
	return ok, err
}

// ListConnectionIDsByEnvironment returns the IDs of all connections tagged to the given environment.
// Used before environment deletion to know which connection ancestry caches need invalidation.
func (db *DB) ListConnectionIDsByEnvironment(ctx context.Context, envID int64) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []int64
	err := db.NewSelect().
		TableExpr("connections").
		ColumnExpr("id").
		Where("environment_id = ?", envID).
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
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

func normalizeConnectionListParams(params ListConnectionsParams) ListConnectionsParams {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	if params.Sort == "" {
		params.Sort = "created_at"
	}
	switch params.Sort {
	case "name", "driver", "created_at":
	default:
		params.Sort = "created_at"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "desc"
	}
	params.Search = strings.TrimSpace(params.Search)
	params.Driver = strings.TrimSpace(params.Driver)
	params.AccessMode = strings.TrimSpace(params.AccessMode)
	return params
}

func connectionSortColumn(sort string) string {
	switch sort {
	case "name":
		return "name"
	case "driver":
		return "driver"
	default:
		return "created_at"
	}
}
