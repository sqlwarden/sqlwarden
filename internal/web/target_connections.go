package web

import (
	"context"
	"errors"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

var (
	errSQLiteTargetDisabled         = errors.New("sqlite file target connections are disabled for this instance")
	errSQLiteInMemoryTargetDisabled = errors.New("sqlite in-memory target connections are disabled for this instance")
)

// validateTargetConnection enforces the server-side policy for user-created
// database targets. Driver registration alone is not enough because some
// registered drivers, such as SQLite, may expose host-local resources.
func (app *application) validateTargetConnection(ctx context.Context, driverName, dsn string) error {
	driverName = strings.TrimSpace(driverName)
	dsn = strings.TrimSpace(dsn)

	if _, err := engine.New(driverName); err != nil {
		return err
	}

	if driverName != string(engine.DialectSQLite) {
		return nil
	}
	settings, err := app.instanceSettings(ctx)
	if err != nil {
		return err
	}
	if isInMemorySQLiteDSN(dsn) {
		if !settings.SQLiteInMemoryTargetsEnabled {
			return errSQLiteInMemoryTargetDisabled
		}
		return nil
	}
	if !settings.SQLiteLocalTargetsEnabled {
		return errSQLiteTargetDisabled
	}
	return nil
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

func isInMemorySQLiteDSN(dsn string) bool {
	switch {
	case dsn == ":memory:":
		return true
	case strings.HasPrefix(dsn, "file::memory:"):
		return true
	default:
		return false
	}
}
