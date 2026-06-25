package mysql

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
)

func (d *mysqlDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	return gosqlx.Classify(ctx, d.Dialect(), req)
}

func (d *mysqlDriver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	return gosqlx.Parse(ctx, d.Dialect(), req)
}

func (d *mysqlDriver) Rewrite(ctx context.Context, req rewriter.Request) (rewriter.Result, error) {
	return gosqlx.Rewrite(ctx, d.Dialect(), req)
}
