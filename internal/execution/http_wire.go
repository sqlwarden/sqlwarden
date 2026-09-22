package execution

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/exports"
)

const (
	runtimeRPCPath    = "/internal/execution/v1/call"
	runtimeStreamPath = "/internal/execution/v1/stream"
)

type rpcEnvelope struct {
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}

type rpcResponse struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code      FailureCode `json:"code,omitempty"`
	Kind      string      `json:"kind,omitempty"`
	Message   string      `json:"message"`
	Retryable bool        `json:"retryable,omitempty"`
}

const (
	rpcKindTransactionOpen          = "transaction_open"
	rpcKindNoOpenTransaction        = "no_open_transaction"
	rpcKindCursorUnsupported        = "cursor_unsupported"
	rpcKindSchemaUnsupported        = "schema_unsupported"
	rpcKindRelationshipsUnsupported = "relationships_unsupported"
	rpcKindExportFormat             = "export_format_unsupported"
	rpcKindExportCursor             = "export_cursor_unsupported"
	rpcKindExportLimit              = "export_limit_exceeded"
)

func newRPCError(err error) *rpcError {
	if err == nil {
		return nil
	}
	wire := &rpcError{Message: err.Error()}
	var failure *Failure
	if errors.As(err, &failure) {
		wire.Code = failure.Code
		wire.Retryable = failure.Retryable
	}
	switch {
	case errors.Is(err, ErrTransactionOpen):
		wire.Kind = rpcKindTransactionOpen
	case errors.Is(err, ErrNoOpenTransaction):
		wire.Kind = rpcKindNoOpenTransaction
	case errors.Is(err, ErrQueryCursorUnsupported):
		wire.Kind = rpcKindCursorUnsupported
	case errors.Is(err, ErrRelationshipsUnsupported):
		wire.Kind = rpcKindRelationshipsUnsupported
	case errors.Is(err, ErrSchemaUnsupported):
		wire.Kind = rpcKindSchemaUnsupported
	case errors.Is(err, exports.ErrUnsupportedFormat):
		wire.Kind = rpcKindExportFormat
	case errors.Is(err, exports.ErrCursorUnsupported):
		wire.Kind = rpcKindExportCursor
	case errors.Is(err, exports.ErrByteLimitExceeded):
		wire.Kind = rpcKindExportLimit
	}
	return wire
}

func (e *rpcError) err() error {
	if e == nil {
		return nil
	}
	var sentinel error
	switch e.Code {
	case FailureSessionLost:
		sentinel = ErrSessionLost
	case FailureCursorLost:
		sentinel = ErrCursorLost
	case FailureTransactionLost:
		sentinel = ErrTransactionLost
	case FailureExecutionOutcomeUnknown:
		sentinel = ErrOutcomeUnknown
	case FailureGrantInvalid:
		sentinel = ErrGrantInvalid
	case FailureLimitExceeded:
		sentinel = ErrLimitExceeded
	}
	switch e.Kind {
	case rpcKindTransactionOpen:
		sentinel = ErrTransactionOpen
	case rpcKindNoOpenTransaction:
		sentinel = ErrNoOpenTransaction
	case rpcKindCursorUnsupported:
		sentinel = ErrQueryCursorUnsupported
	case rpcKindSchemaUnsupported:
		sentinel = ErrSchemaUnsupported
	case rpcKindRelationshipsUnsupported:
		sentinel = ErrRelationshipsUnsupported
	case rpcKindExportFormat:
		sentinel = exports.ErrUnsupportedFormat
	case rpcKindExportCursor:
		sentinel = exports.ErrCursorUnsupported
	case rpcKindExportLimit:
		sentinel = exports.ErrByteLimitExceeded
	}
	message := errors.New(e.Message)
	if sentinel != nil {
		message = errors.Join(sentinel, message)
	}
	if e.Code != "" {
		return &Failure{Code: e.Code, Retryable: e.Retryable, Err: message}
	}
	return fmt.Errorf("execution RPC: %w", message)
}
