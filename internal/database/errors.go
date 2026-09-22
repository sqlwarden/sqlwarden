package database

import (
	"errors"
	"strings"

	"github.com/uptrace/bun/driver/pgdriver"
)

var ErrEnvironmentHasConnections = errors.New("environment has connections")

// IsUniqueViolation reports whether err is a unique-constraint violation from
// either the PostgreSQL or SQLite driver, so callers can turn a race between
// an existence check and an insert into a domain error.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
