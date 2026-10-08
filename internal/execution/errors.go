package execution

import "errors"

// FailureCode is stable across local errors and future remote status mapping.
type FailureCode string

const (
	FailureSessionNotFound         FailureCode = "session_not_found"
	FailureSessionLost             FailureCode = "session_lost"
	FailureCursorLost              FailureCode = "cursor_lost"
	FailureTransactionLost         FailureCode = "transaction_lost"
	FailureExecutionOutcomeUnknown FailureCode = "execution_outcome_unknown"
	FailureLimitExceeded           FailureCode = "limit_exceeded"
)

// Failure carries a stable machine-readable runtime failure code while
// retaining the underlying error for errors.Is and errors.As checks.
type Failure struct {
	Code      FailureCode
	Retryable bool
	Err       error
}

func (e *Failure) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

// Unwrap exposes the underlying local or transport error.
func (e *Failure) Unwrap() error { return e.Err }

// TargetError is an error the target database or its driver returned for a
// statement or read. Its message originates from the target and is meant for
// the user who issued the statement; errors without this type are internal.
type TargetError struct {
	Err error
}

func (e *TargetError) Error() string { return e.Err.Error() }

func (e *TargetError) Unwrap() error { return e.Err }

// ConnectError reports that the driver could not be built or the target
// refused or failed the connection attempt. Like TargetError its message is
// meant for the user; credential and policy-store failures never carry it.
type ConnectError struct {
	Err error
}

func (e *ConnectError) Error() string { return e.Err.Error() }

func (e *ConnectError) Unwrap() error { return e.Err }

var (
	ErrSessionNotFound          = errors.New("execution session not found")
	ErrSessionLost              = errors.New("execution session is no longer available")
	ErrCursorLost               = errors.New("execution cursor is no longer available")
	ErrTransactionLost          = errors.New("execution transaction is no longer available")
	ErrOutcomeUnknown           = errors.New("execution outcome is unknown")
	ErrLimitExceeded            = errors.New("execution limit exceeded")
	ErrTransactionOpen          = errors.New("execution transaction is open")
	ErrNoOpenTransaction        = errors.New("execution has no open transaction")
	ErrCursorsUnsupported       = errors.New("execution cursors are not supported by this driver")
	ErrSchemaUnsupported        = errors.New("execution schema inspection is not supported by this driver")
	ErrRelationshipsUnsupported = errors.New("execution relationship inspection is not supported by this driver")
	ErrDefinitionUnsupported    = errors.New("execution definition inspection is not supported by this driver")
	ErrDDLRequiresApply         = errors.New("execution structured DDL must be applied through ApplyDDL")
	ErrInvalidTxMode            = errors.New("execution transaction mode must be auto or manual")
	ErrUnknownFolder            = errors.New("execution navigator folder is not defined for this node")
	ErrInvalidDDL               = errors.New("execution DDL request is invalid")
	ErrDDLUnsupported           = errors.New("execution DDL is not supported by this driver")
)
