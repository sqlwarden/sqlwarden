package web

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/catalog"
)

var (
	errSQLiteTargetDisabled         = catalog.ErrSQLiteTargetDisabled
	errSQLiteInMemoryTargetDisabled = catalog.ErrSQLiteInMemoryTargetDisabled
)

// validateTargetConnection enforces the server-side policy for user-created
// database targets. Driver registration alone is not enough because some
// registered drivers, such as SQLite, may expose host-local resources.
func (app *application) validateTargetConnection(ctx context.Context, driverName, dsn string) error {
	if app.catalog != nil {
		return app.catalog.Targets().Validate(ctx, driverName, dsn)
	}
	return catalog.NewTargetPolicy(app.settingsService()).Validate(ctx, driverName, dsn)
}

func targetConnectionFieldError(err error) string {
	if errors.Is(err, errSQLiteTargetDisabled) {
		return "SQLite file connections are disabled for this instance."
	}
	if errors.Is(err, errSQLiteInMemoryTargetDisabled) {
		return "In-memory SQLite connections are disabled for this instance."
	}
	return "Driver must be a supported driver."
}
