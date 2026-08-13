package postgres

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/cursor"
)

var _ cursor.QueryCursorDriver = (*postgresDriver)(nil)

func (d *postgresDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.db.QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: start query: %w", err)
	}
	cursor, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres: start query cursor: %w", err)
	}
	return cursor, nil
}
