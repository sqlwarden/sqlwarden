package orgs

import (
	"errors"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	ErrForbidden  = access.ErrForbidden
	ErrNotFound   = access.ErrNotFound
	ErrValidation = errors.New("orgs: validation failed")
	// ErrLastOwner reports that the change would leave the organization
	// without an owner.
	ErrLastOwner = errors.New("orgs: organization must keep an owner")
)

// ValidationError carries field-safe validation messages. Values supplied by
// callers are never copied into these messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func isUniqueViolation(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func isForeignKeyViolation(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23503"
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
