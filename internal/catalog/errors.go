package catalog

import "errors"

var (
	ErrForbidden      = errors.New("catalog: forbidden")
	ErrNotFound       = errors.New("catalog: connection not found")
	ErrValidation     = errors.New("catalog: validation failed")
	ErrActiveSessions = errors.New("catalog: connection has active sessions")
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
