package web

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/execution"
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

// isTargetRejected reports whether an execution Open failed on connector-side
// target policy. Remote runtimes carry only the most specific sentinel.
func isTargetRejected(err error) bool {
	return errors.Is(err, execution.ErrTargetRejected) ||
		errors.Is(err, execution.ErrSQLiteTargetDisabled) ||
		errors.Is(err, execution.ErrSQLiteInMemoryTargetDisabled)
}

// targetCredentialsUnavailableMessage is the user-facing text for credential
// resolution failures; the underlying error is never shown.
const targetCredentialsUnavailableMessage = "The connection credentials could not be loaded. Update the connection and try again."

// isTargetCredentialFailure reports whether an execution Open failed resolving
// stored connection credentials. Retrying cannot repair the stored record, so
// callers classify these as permanent.
func isTargetCredentialFailure(err error) bool {
	return errors.Is(err, execution.ErrCredentialsNotFound) ||
		errors.Is(err, execution.ErrCredentialDecryption) ||
		errors.Is(err, execution.ErrCredentialsInvalid)
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
