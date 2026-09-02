package oracle

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/cursor"
)

var _ cursor.QueryCursorDriver = (*oracleDriver)(nil)

func (d *oracleDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	// SQL is user-authored editor input, permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: start query: %w", err)
	}
	c, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("oracle: start query cursor: %w", err)
	}
	return c, nil
}
