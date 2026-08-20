package database

import (
	"context"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

type QueryFavorite struct {
	bun.BaseModel `bun:"table:query_favorites" json:"-"`

	ID           int64     `bun:",pk,autoincrement" json:"id"`
	WorkspaceID  int64     `bun:",notnull"          json:"workspace_id"`
	AccountID    int64     `bun:",notnull"          json:"account_id"`
	ConnectionID *int64    `bun:""                  json:"connection_id"`
	Name         string    `bun:",notnull"          json:"name"`
	SQL          string    `bun:"sql_text,notnull"  json:"sql_text"`
	CreatedAt    time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt    time.Time `bun:",notnull"          json:"updated_at"`
}

func (db *DB) CreateQueryFavorite(ctx context.Context, fav QueryFavorite) (QueryFavorite, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	now := time.Now()
	fav.CreatedAt = now
	fav.UpdatedAt = now

	if _, err := db.NewInsert().Model(&fav).Returning("*").Exec(ctx); err != nil {
		return QueryFavorite{}, err
	}
	return fav, nil
}

func (db *DB) ListQueryFavorites(ctx context.Context, workspaceID, accountID int64, search string) ([]QueryFavorite, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var favorites []QueryFavorite
	q := db.NewSelect().Model(&favorites).
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID)
	if search != "" {
		term := "%" + strings.ToLower(search) + "%"
		q = q.Where("(LOWER(name) LIKE ? OR LOWER(sql_text) LIKE ?)", term, term)
	}
	err := q.OrderExpr("name ASC").Scan(ctx)
	return favorites, err
}

func (db *DB) UpdateQueryFavorite(ctx context.Context, id, workspaceID, accountID int64, name, sqlText string) (QueryFavorite, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var updated QueryFavorite
	res, err := db.NewUpdate().Model(&updated).
		Set("name = ?", name).
		Set("sql_text = ?", sqlText).
		Set("updated_at = ?", time.Now()).
		Where("id = ? AND workspace_id = ? AND account_id = ?", id, workspaceID, accountID).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return QueryFavorite{}, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return QueryFavorite{}, false, err
	}
	if affected == 0 {
		return QueryFavorite{}, false, nil
	}
	return updated, true, nil
}

func (db *DB) DeleteQueryFavorite(ctx context.Context, id, workspaceID, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	res, err := db.NewDelete().Model((*QueryFavorite)(nil)).
		Where("id = ? AND workspace_id = ? AND account_id = ?", id, workspaceID, accountID).
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

func (db *DB) ClearQueryFavoritesForOrg(ctx context.Context, orgID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewRaw(`
		DELETE FROM query_favorites
		WHERE workspace_id IN (SELECT id FROM workspaces WHERE org_id = ?)`, orgID).Exec(ctx)
	return err
}

func (db *DB) QueryFavoritesHasRowsForOrg(ctx context.Context, orgID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var count int
	err := db.NewRaw(`
		SELECT COUNT(*) FROM query_favorites
		WHERE workspace_id IN (SELECT id FROM workspaces WHERE org_id = ?)`, orgID).Scan(ctx, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
