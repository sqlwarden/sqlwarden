package postgres

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/dbengine/cursor"
)

var _ cursor.QueryCursorDriver = (*postgresDriver)(nil)

func (d *postgresDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	// SQL is intentionally user-authored IDE input and is permission-gated by the web layer.
	rows, err := d.db.QueryContext(ctx, req.SQL, req.Args...) // lgtm[go/sql-injection]
	if err != nil {
		return nil, fmt.Errorf("postgres: start query: %w", err)
	}
	cursor, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres: start query cursor: %w", err)
	}
	return cursor, nil
}
