package exportjob

import (
	"context"
	"errors"
	"strings"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/settings"
)

var (
	// ErrClassifierUnavailable reports a driver without a registered SQL
	// classifier. Exports must prove a statement is a single read query, so
	// there is no heuristic fallback.
	ErrClassifierUnavailable = errors.New("exportjob: sql classification is unavailable for this driver")
	// ErrSessionRequired reports a synchronous download without a session id.
	ErrSessionRequired = errors.New("exportjob: session id is required")
)

var (
	errNotReadQuery   = errors.New("exportjob: statement is not a read query")
	errMultiStatement = errors.New("exportjob: more than one statement")
)

const (
	messageClassifierUnavailable = "SQL export is unavailable for this driver because reliable SQL classification is not implemented."
	messageNotReadQuery          = "Only read queries can be exported."
	messageMultiStatement        = "Only a single query can be exported. Multi-query export isn't supported yet."
)

// ClassifierUnavailableMessage is the user-facing text for ErrClassifierUnavailable.
const ClassifierUnavailableMessage = messageClassifierUnavailable

// ValidationError reports input the export cannot accept, keyed by field.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for field, message := range e.Fields {
		parts = append(parts, field+": "+message)
	}
	return "exportjob: invalid input: " + strings.Join(parts, "; ")
}

func validationError(field, message string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: message}}
}

func targetBlockedMessage(err error) string {
	switch {
	case errors.Is(err, settings.ErrSQLiteFileTargetDisabled):
		return "SQLite file connections are disabled for this instance."
	case errors.Is(err, settings.ErrSQLiteInMemoryTargetDisabled):
		return "In-memory SQLite connections are disabled for this instance."
	default:
		return "Driver must be a supported driver."
	}
}

// openError classifies a failure to open the export session. Credential and
// unexpected errors are returned as-is so the job fails without echoing them.
func openError(err error) error {
	var tunnelErr *execution.TunnelError
	var connectErr *execution.ConnectError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, settings.ErrTargetDenied):
		return jobs.Permanent("export_target_blocked", targetBlockedMessage(err))
	case errors.Is(err, credentials.ErrNotFound):
		return jobs.Permanent("connection_not_found", "Connection was not found.")
	case errors.As(err, &tunnelErr), errors.As(err, &connectErr):
		return jobs.Retryable("export_connect_failed", "Could not connect to the target database.")
	default:
		return err
	}
}

func streamError(err error) error {
	switch {
	case errors.Is(err, exports.ErrByteLimitExceeded):
		return jobs.Permanent("export_too_large", "Export exceeded the configured size limit.")
	case errors.Is(err, exports.ErrCursorUnsupported):
		return jobs.Permanent("export_unsupported_driver", "This database driver does not support streaming exports.")
	case errors.Is(err, context.Canceled):
		return err
	default:
		return jobs.Retryable("export_failed", "Export failed.")
	}
}

// ErrorCategory names a stream failure for logs without echoing target text.
func ErrorCategory(err error) string {
	switch {
	case errors.Is(err, exports.ErrByteLimitExceeded):
		return "byte_limit_exceeded"
	case errors.Is(err, exports.ErrCursorUnsupported):
		return "unsupported_driver"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "target_error"
	}
}
