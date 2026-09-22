package catalog

import (
	"errors"

	"github.com/sqlwarden/internal/validator"
)

// ValidationError reports input a catalog use case refused, field by field.
//
// The catalog owns these messages because it owns the rules that produce them;
// a transport renders them in its own envelope rather than restating the rule.
// A transport that also validates its own wire-format concerns merges its
// errors into the same validator before responding, so a caller still sees
// every problem with a request at once.
type ValidationError struct {
	Validator validator.Validator
}

// Error implements error.
func (e *ValidationError) Error() string { return "catalog input validation failed" }

// AsValidationError reports whether err is a [ValidationError] and returns it.
func AsValidationError(err error) (*ValidationError, bool) {
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		return invalid, true
	}
	return nil, false
}

// newValidationError returns a [ValidationError] when v holds problems, and
// nil when it does not, so callers can return it directly.
func newValidationError(v validator.Validator) error {
	if !v.HasErrors() {
		return nil
	}
	return &ValidationError{Validator: v}
}
