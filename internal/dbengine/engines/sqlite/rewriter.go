package sqlite

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/gosqlx"
	"github.com/sqlwarden/internal/dbengine/rewriter"
)

var _ rewriter.Rewriter = (*sqliteDriver)(nil)

func (d *sqliteDriver) Rewrite(ctx context.Context, req rewriter.Request) (rewriter.Result, error) {
	return gosqlx.Rewrite(ctx, d.Dialect(), req)
}
