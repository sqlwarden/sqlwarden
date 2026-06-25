package sqlite

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
)

var _ classifier.Classifier = (*sqliteDriver)(nil)

func (d *sqliteDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	return gosqlx.Classify(ctx, d.Dialect(), req)
}
