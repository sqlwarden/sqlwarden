package catalog

import (
	"errors"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	ErrForbidden      = access.ErrForbidden
	ErrNotFound       = access.ErrNotFound
	ErrValidation     = errors.New("catalog: validation failed")
	ErrActiveSessions = errors.New("catalog: connection has active sessions")
	// ErrEnvironmentHasConnections refuses deleting an environment that still
	// holds connections.
	ErrEnvironmentHasConnections = database.ErrEnvironmentHasConnections
)

const (
	RevealCodeNotRevealable = "secret_not_revealable"
	RevealCodeDisabled      = "reveal_disabled"
)

// ErrReveal is a stable reveal denial that handlers can translate without
// inspecting provider or policy errors.
type ErrReveal struct {
	Code string
}

func (e *ErrReveal) Error() string { return e.Code }

// ValidationError carries field-safe validation messages. Values supplied by
// callers are never copied into these messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func validationError(field, message string) error {
	return &ValidationError{Fields: map[string]string{field: message}}
}

func isUniqueViolation(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
