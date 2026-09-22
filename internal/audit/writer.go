// Package audit defines durable security and business audit contracts.
package audit

import (
	"context"
	"time"
)

// Event is one immutable audit intent emitted by an application use case.
type Event struct {
	ID         string
	OccurredAt time.Time
	OrgID      *int64
	AccountID  *int64
	Action     string
	Resource   string
	ResourceID string
	Outcome    string
	Metadata   map[string]string
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
