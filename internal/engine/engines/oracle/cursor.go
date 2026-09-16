package oracle

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/cursor"
)

var _ cursor.QueryCursorDriver = (*oracleDriver)(nil)

func (d *oracleDriver) StartQuery(ctx context.Context, req cursor.QueryRequest) (cursor.QueryCursor, error) {
	// go-ora rejects a trailing statement terminator on a lone statement (ORA-00933);
	// editor-resolved single statements keep their trailing ";", so strip it here
	// rather than relying on every caller to have already done so.
	sql := trimTrailingSemicolon(req.SQL)
	// SQL is user-authored editor input, permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, sql, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: start query: %w", err)
	}
	c, err := cursor.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("oracle: start query cursor: %w", err)
	}
	return c, nil
}
