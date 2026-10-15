package query

import (
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine/safety"
	"github.com/sqlwarden/internal/execution"
)

var (
	ErrForbidden = access.ErrForbidden
	ErrNotFound  = access.ErrNotFound

	ErrValidation = errors.New("query: validation failed")
	ErrDisabled   = errors.New("query: feature disabled")

	// ErrSessionRequired reports a runtime operation that names no session.
	ErrSessionRequired = errors.New("query: session id is required")

	ErrExplainUnsupported        = errors.New("query: explain is not supported by this connection")
	ErrExplainAnalyzeUnsupported = errors.New("query: explain analyze is not supported by this connection")
	ErrExplainMultiple           = errors.New("query: explain requires exactly one statement")
	ErrExplainAlready            = errors.New("query: statement is already an explain statement")
	ErrUnsafeConfirmation        = errors.New("query: unsafe statement requires confirmation")

	// ErrCursorsUnsupported is the execution sentinel, re-exported so callers
	// can match it without importing execution.
	ErrCursorsUnsupported = execution.ErrCursorsUnsupported
)

// RuntimeError marks a failure returned by the execution runtime, as opposed
// to a rejection decided by the service before the runtime was reached.
type RuntimeError struct {
	Err error
}

func (e *RuntimeError) Error() string { return e.Err.Error() }
func (e *RuntimeError) Unwrap() error { return e.Err }

// UnsafeError reports DML that affects every row and was not confirmed.
// Statements describe the offending statements without their SQL values.
type UnsafeError struct {
	Statements []safety.UnsafeStatement
}

func (e *UnsafeError) Error() string { return ErrUnsafeConfirmation.Error() }
func (e *UnsafeError) Unwrap() error { return ErrUnsafeConfirmation }

// ValidationError carries field-safe validation messages. Values supplied by
// callers are never copied into these messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// DisabledError reports a feature turned off by settings. Message is safe to
// show to the caller.
type DisabledError struct {
	Message string
}

func (e *DisabledError) Error() string { return ErrDisabled.Error() }
func (e *DisabledError) Unwrap() error { return ErrDisabled }
