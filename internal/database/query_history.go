package database

import (
	"context"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/sqlwarden/internal/response"
)

type QueryHistoryEntry struct {
	bun.BaseModel `bun:"table:query_history" json:"-"`

	ID           int64     `bun:",pk,autoincrement" json:"id"`
	ConnectionID int64     `bun:",notnull"          json:"connection_id"`
	AccountID    int64     `bun:",notnull"          json:"account_id"`
	SQL          string    `bun:"sql_text,notnull"  json:"sql_text"`
	Status       string    `bun:",notnull"          json:"status"`
	ErrorMessage *string   `bun:""                  json:"error_message"`
	DurationMS   int64     `bun:",notnull"          json:"duration_ms"`
	RowsAffected int64     `bun:",notnull"          json:"rows_affected"`
	ExecutedAt   time.Time `bun:",notnull"          json:"executed_at"`
}

// InsertQueryHistoryEntry records a query history entry and trims older entries
// for the same connection/account beyond retentionCount.
func (db *DB) InsertQueryHistoryEntry(ctx context.Context, entry QueryHistoryEntry, retentionCount int) (QueryHistoryEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if entry.ExecutedAt.IsZero() {
		entry.ExecutedAt = time.Now()
	}

	if _, err := db.NewInsert().Model(&entry).Returning("*").Exec(ctx); err != nil {
		return QueryHistoryEntry{}, err
	}

	_, err := db.NewRaw(`
		DELETE FROM query_history
		WHERE connection_id = ? AND account_id = ?
		AND id NOT IN (
			SELECT id FROM query_history
			WHERE connection_id = ? AND account_id = ?
			ORDER BY executed_at DESC, id DESC
			LIMIT ?
		)`,
		entry.ConnectionID, entry.AccountID,
		entry.ConnectionID, entry.AccountID,
		retentionCount,
	).Exec(ctx)
	if err != nil {
		return QueryHistoryEntry{}, err
	}

	return entry, nil
}

func (db *DB) ListQueryHistory(ctx context.Context, connectionID, accountID int64, search string, page, pageSize int) (response.Paginated[QueryHistoryEntry], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var entries []QueryHistoryEntry

	q := db.NewSelect().Model(&entries).
		Where("connection_id = ? AND account_id = ?", connectionID, accountID)
	if search != "" {
		q = q.Where("LOWER(sql_text) LIKE ?", "%"+strings.ToLower(search)+"%")
	}

	total, err := q.Count(ctx)
	if err != nil {
		return response.Paginated[QueryHistoryEntry]{}, err
	}

	err = q.OrderExpr("executed_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(ctx)
	if err != nil {
		return response.Paginated[QueryHistoryEntry]{}, err
	}
	if entries == nil {
		entries = []QueryHistoryEntry{}
	}

	return response.Paginated[QueryHistoryEntry]{
		Items:    entries,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// ListQueryHistoryForWorkspace lists query history across every connection in a
// workspace, optionally narrowed to a single connection. Results are always
// scoped to accountID, matching ListQueryHistory's per-account visibility.
func (db *DB) ListQueryHistoryForWorkspace(ctx context.Context, workspaceID, accountID int64, connectionID *int64, search string, page, pageSize int) (response.Paginated[QueryHistoryEntry], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var entries []QueryHistoryEntry

	q := db.NewSelect().Model(&entries).
		Where("account_id = ?", accountID).
		Where("connection_id IN (SELECT id FROM connections WHERE workspace_id = ?)", workspaceID)
	if connectionID != nil {
		q = q.Where("connection_id = ?", *connectionID)
	}
	if search != "" {
		q = q.Where("LOWER(sql_text) LIKE ?", "%"+strings.ToLower(search)+"%")
	}

	total, err := q.Count(ctx)
	if err != nil {
		return response.Paginated[QueryHistoryEntry]{}, err
	}

	err = q.OrderExpr("executed_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(ctx)
	if err != nil {
		return response.Paginated[QueryHistoryEntry]{}, err
	}
	if entries == nil {
		entries = []QueryHistoryEntry{}
	}

	return response.Paginated[QueryHistoryEntry]{
		Items:    entries,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (db *DB) DeleteQueryHistoryEntry(ctx context.Context, id, connectionID, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	res, err := db.NewDelete().Model((*QueryHistoryEntry)(nil)).
		Where("id = ? AND connection_id = ? AND account_id = ?", id, connectionID, accountID).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (db *DB) ClearQueryHistoryForConnection(ctx context.Context, connectionID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*QueryHistoryEntry)(nil)).
		Where("connection_id = ? AND account_id = ?", connectionID, accountID).
		Exec(ctx)
	return err
}

func (db *DB) ClearQueryHistoryForOrg(ctx context.Context, orgID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewRaw(`
		DELETE FROM query_history
		WHERE connection_id IN (
			SELECT connections.id FROM connections
			JOIN workspaces ON connections.workspace_id = workspaces.id
			WHERE workspaces.org_id = ?
		)`, orgID).Exec(ctx)
	return err
}

func (db *DB) QueryHistoryHasRowsForOrg(ctx context.Context, orgID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var count int
	err := db.NewRaw(`
		SELECT COUNT(*) FROM query_history
		WHERE connection_id IN (
			SELECT connections.id FROM connections
			JOIN workspaces ON connections.workspace_id = workspaces.id
			WHERE workspaces.org_id = ?
		)`, orgID).Scan(ctx, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
