package web

import (
	"errors"
	"strings"

	"github.com/sqlwarden/internal/dbengine"
)

var errSQLiteTargetDisabled = errors.New("sqlite file target connections are disabled for this instance")

// validateTargetConnection enforces the server-side policy for user-created
// database targets. Driver registration alone is not enough because some
// registered drivers, such as SQLite, may expose host-local resources.
func (app *application) validateTargetConnection(driverName, dsn string) error {
	driverName = strings.TrimSpace(driverName)
	dsn = strings.TrimSpace(dsn)

	if _, err := dbengine.New(driverName); err != nil {
		return err
	}

	if driverName != string(dbengine.DialectSQLite) {
		return nil
	}
	if isInMemorySQLiteDSN(dsn) {
		return nil
	}
	if !sqliteDriverSourceAllowed(app.config, SQLiteDriverSourceLocal) {
		return errSQLiteTargetDisabled
	}
	return nil
}

func sqliteDriverSourceAllowed(cfg Config, source string) bool {
	for _, allowed := range cfg.Drivers.SQLite.AllowedSources {
		if allowed == source {
			return true
		}
	}
	return false
}

func targetConnectionFieldError(err error) string {
	if errors.Is(err, errSQLiteTargetDisabled) {
		return "SQLite file connections are disabled for this instance."
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
