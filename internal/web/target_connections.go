package web

import (
	"errors"

	"github.com/sqlwarden/internal/settings"
)

func isTargetPolicyDenial(err error) bool {
	return errors.Is(err, settings.ErrTargetDenied)
}

func isSQLiteTargetDisabled(err error) bool {
	return errors.Is(err, settings.ErrSQLiteFileTargetDisabled) || errors.Is(err, settings.ErrSQLiteInMemoryTargetDisabled)
}

func targetConnectionFieldError(err error) string {
	if errors.Is(err, settings.ErrSQLiteFileTargetDisabled) {
		return "SQLite file connections are disabled for this instance."
	}
	if errors.Is(err, settings.ErrSQLiteInMemoryTargetDisabled) {
		return "In-memory SQLite connections are disabled for this instance."
	}
	return "Driver must be a supported driver."
}
