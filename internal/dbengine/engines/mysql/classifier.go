package mysql

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
)

var _ classifier.Classifier = (*mysqlDriver)(nil)

func (d *mysqlDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	return gosqlx.Classify(ctx, d.Dialect(), req)
}
