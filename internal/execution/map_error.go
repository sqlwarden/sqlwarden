package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/internal/exports"
)

// operation classifies a runtime call for error mapping. Only the write
// classes can leave the target in an unknown state when interrupted.
type operation int

const (
	opQuery operation = iota
	opFetch
	opExecute
	opStream
	opCancel
	opTransaction
	opCommit
	opDDL
	opMetadata
)

// writes reports whether interrupting the operation may leave its effect
// applied on the target without the caller learning about it.
func (o operation) writes() bool {
	return o == opExecute || o == opCommit || o == opDDL
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// mapError translates driver, connection and export errors into the runtime
// vocabulary. Sentinels from errors.go gain their stable failure code, driver
// sentinels are wrapped so errors.Is matches the execution sentinel and the
// original, read cancellations pass through, and everything else came from the
// target and becomes a TargetError.
func mapError(err error, op operation) error {
	if err == nil {
		return nil
	}
	var existing *Failure
	if errors.As(err, &existing) {
		return err
	}
	if isContextError(err) && op.writes() {
		return &Failure{Code: FailureExecutionOutcomeUnknown, Err: fmt.Errorf("%w: %w", ErrOutcomeUnknown, err)}
	}

	switch {
	case errors.Is(err, ErrSessionNotFound):
		return &Failure{Code: FailureSessionNotFound, Err: err}
	case errors.Is(err, ErrSessionLost):
		return &Failure{Code: FailureSessionLost, Err: err}
	case errors.Is(err, ErrCursorLost):
		return &Failure{Code: FailureCursorLost, Err: err}
	case errors.Is(err, cursor.ErrCursorClosed):
		return &Failure{Code: FailureCursorLost, Err: fmt.Errorf("%w: %w", ErrCursorLost, err)}
	case errors.Is(err, ErrTransactionLost):
		return &Failure{Code: FailureTransactionLost, Err: err}
	case errors.Is(err, ErrOutcomeUnknown):
		return &Failure{Code: FailureExecutionOutcomeUnknown, Err: err}
	case errors.Is(err, ErrLimitExceeded):
		return &Failure{Code: FailureLimitExceeded, Err: err}
	case errors.Is(err, exports.ErrByteLimitExceeded):
		return &Failure{Code: FailureLimitExceeded, Err: fmt.Errorf("%w: %w", ErrLimitExceeded, err)}
	case errors.Is(err, connection.ErrQueryCursorsUnsupported), errors.Is(err, exports.ErrCursorUnsupported):
		return fmt.Errorf("%w: %w", ErrCursorsUnsupported, err)
	case errors.Is(err, connection.ErrTransactionOpen):
		return fmt.Errorf("%w: %w", ErrTransactionOpen, err)
	case errors.Is(err, transaction.ErrNoOpenTransaction):
		return fmt.Errorf("%w: %w", ErrNoOpenTransaction, err)
	case errors.Is(err, ddl.ErrUnsupported):
		return fmt.Errorf("%w: %w", ErrDDLUnsupported, err)
	}
	var target *TargetError
	if isContextError(err) || op == opCancel || errors.As(err, &target) || isRuntimeSentinel(err) {
		return err
	}
	return &TargetError{Err: err}
}

// isRuntimeSentinel reports runtime-originated errors that callers classify
// themselves and that must not be presented as target errors.
func isRuntimeSentinel(err error) bool {
	for _, sentinel := range []error{
		ErrTransactionOpen, ErrNoOpenTransaction, ErrCursorsUnsupported, ErrSchemaUnsupported,
		ErrRelationshipsUnsupported, ErrDefinitionUnsupported, ErrDDLRequiresApply, ErrInvalidTxMode,
		ErrUnknownFolder, ErrInvalidDDL, ErrDDLUnsupported,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}
