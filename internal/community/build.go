// Package community is the Community Edition application composition root.
package community

import (
	"context"

	"github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/edition"
)

// Build composes the core application with the Community edition. The package
// has no dependency on Enterprise source and remains buildable when the ee
// tree is absent.
func Build(ctx context.Context, opts app.Options) (*app.Application, error) {
	opts.Edition = edition.NewCommunity()
	return app.Build(ctx, opts)
}
