package postgres

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/gosqlx"
	"github.com/sqlwarden/internal/dbengine/parser"
)

var _ parser.Parser = (*postgresDriver)(nil)

func (d *postgresDriver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	return gosqlx.Parse(ctx, d.Dialect(), req)
}
