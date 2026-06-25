package postgres

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
)

var _ classifier.Classifier = (*postgresDriver)(nil)

func (d *postgresDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	return gosqlx.Classify(ctx, d.Dialect(), req)
}
