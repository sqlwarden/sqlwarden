package audit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeDenied  = "denied"
)

const (
	ActionSetupCompleted = "instance.setup"
	ActionSignIn         = "auth.sign_in"
)

var ErrInvalidEvent = errors.New("invalid audit event")

type Actor struct {
	SubjectKind    string
	SubjectID      *int64
	OnBehalfOfKind string
	OnBehalfOfID   *int64
	CredentialKind string
	CredentialID   string
	ClientID       string
	AuthMethod     string
	Assurance      string
}

// Event is one immutable audit intent emitted by an application use case.
//
// Metadata carries only non-sensitive descriptive context. Credentials, tokens,
// SQL text, bind parameters, and row values must never appear in it.
type Event struct {
	Sequence int64
	ID       string
	// OccurredAt is when the audited action happened. A zero value is filled
	// in by the core writer from its clock.
	OccurredAt time.Time
	OrgID      *int64
	Actor      Actor
	Action     string
	Resource   string
	ResourceID string
	Outcome    string
	// DecisionReason is a stable reason code for an authorization decision.
	DecisionReason string
	Metadata       map[string]string
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

// Writer durably records audit events.
type Writer interface {
	Write(ctx context.Context, event Event) error
}

// WriterFunc adapts a function to [Writer].
type WriterFunc func(context.Context, Event) error

// Write implements [Writer].
func (f WriterFunc) Write(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Discard is a [Writer] that records nothing.
var Discard Writer = WriterFunc(func(context.Context, Event) error { return nil })

// Emit records event through writer, treating a nil writer as [Discard].
func Emit(ctx context.Context, writer Writer, event Event) error {
	if writer == nil {
		return nil
	}
	return writer.Write(ctx, event)
}
