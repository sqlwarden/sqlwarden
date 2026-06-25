package sqlite

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/dbengine/cursor"
)

var _ cursor.QueryCursorDriver = (*sqliteDriver)(nil)

func (d *sqliteDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	rows, err := d.db.QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: start query: %w", err)
	}
	cursor, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: start query cursor: %w", err)
	}
	return cursor, nil
}
