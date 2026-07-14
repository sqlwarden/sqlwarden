package mysql

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/dbengine/cursor"
)

var _ cursor.QueryCursorDriver = (*mysqlDriver)(nil)

func (d *mysqlDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	// SQL is intentionally user-authored IDE input and is permission-gated by the web layer.
	rows, err := d.db.QueryContext(ctx, req.SQL, req.Args...) // lgtm[go/sql-injection]
	if err != nil {
		return nil, fmt.Errorf("mysql: start query: %w", err)
	}
	cursor, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("mysql: start query cursor: %w", err)
	}
	return cursor, nil
}
