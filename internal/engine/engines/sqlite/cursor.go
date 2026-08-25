package sqlite

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/cursor"
)

var _ cursor.QueryCursorDriver = (*sqliteDriver)(nil)

func (d *sqliteDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: start query: %w", err)
	}
	cursor, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: start query cursor: %w", err)
	}
	return cursor, nil
}
