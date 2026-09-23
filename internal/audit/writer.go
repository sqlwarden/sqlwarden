package audit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Outcomes an audit event can report. The set is closed so a query over the
// audit trail can rely on it.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeDenied  = "denied"
)

// ErrInvalidEvent reports an audit event that cannot be recorded because it
// does not identify what happened.
var ErrInvalidEvent = errors.New("invalid audit event")

// Event is one immutable audit intent emitted by an application use case.
//
// Metadata carries only non-sensitive descriptive context. Credentials, tokens,
// SQL text, bind parameters, and row values must never appear in it.
type Event struct {
	// Sequence is the database-assigned durable insertion order. Producers
	// leave it zero; readers populate it for reconciliation and export cursors.
	// It is deliberately excluded from the canonical event payload because it
	// is assigned by storage after the intent is created.
	Sequence int64
	// ID is the stable event identity. An empty ID is assigned by the core
	// writer so callers do not have to generate one.
	ID string
	// OccurredAt is when the audited action happened. A zero value is filled
	// in by the core writer from its clock.
	OccurredAt time.Time
	OrgID      *int64
	AccountID  *int64
	// Action is the use case that ran, such as "identity.account.register".
	Action string
	// Resource and ResourceID identify what the action acted on. Both are
	// optional for actions that have no addressable subject.
	Resource   string
	ResourceID string
	// Outcome is one of [OutcomeSuccess], [OutcomeFailure], or [OutcomeDenied].
	Outcome  string
	Metadata map[string]string
}

// Validate reports whether the event identifies an action and a known outcome.
func (e Event) Validate() error {
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("%w: action is required", ErrInvalidEvent)
	}
	switch e.Outcome {
	case OutcomeSuccess, OutcomeFailure, OutcomeDenied:
		return nil
	default:
		return fmt.Errorf("%w: unknown outcome %q", ErrInvalidEvent, e.Outcome)
	}
}

// Writer durably records audit events. Implementations must return an error
// when durability cannot be established; optional exports must not weaken the
// durability guarantee of the wrapped core writer.
type Writer interface {
	Write(ctx context.Context, event Event) error
}

// WriterFunc adapts a function to [Writer].
type WriterFunc func(context.Context, Event) error

// Write implements [Writer].
func (f WriterFunc) Write(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Discard is a [Writer] that records nothing. It exists so a component built
// without an audit sink still has a non-nil writer, and is never the writer a
// running instance is composed with.
var Discard Writer = WriterFunc(func(context.Context, Event) error { return nil })

// Emit records event through writer, treating a nil writer as [Discard] so a
// component assembled without an audit sink does not panic.
//
// Application services return success-path errors rather than silently
// accepting a missing audit record. On a refused-action path they preserve the
// original domain error so audit storage cannot mask why the action was denied.
func Emit(ctx context.Context, writer Writer, event Event) error {
	if writer == nil {
		return nil
	}
	return writer.Write(ctx, event)
}
