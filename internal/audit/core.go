package audit

import (
	"context"
	"fmt"
	"time"
)

// Store is the durable audit persistence contract. The core writer records
// through it; edition decorators own their own tables and never write through
// this store.
type Store interface {
	InsertEvent(ctx context.Context, event Event) error
}

// Reader reads back recorded audit events in durable insertion order. Edition tamper
// evidence verifies and reconciles against this contract instead of reaching
// into core tables directly.
//
// EventsAfter is the reconciliation read: it returns the events that follow a
// known position, so a decorator that fell behind can resume from durable
// records rather than from state it kept in memory.
type Reader interface {
	Events(ctx context.Context, limit int) ([]Event, error)
	EventsAfter(ctx context.Context, afterID string, limit int) ([]Event, error)
}

// storageResolution is the timestamp precision an audit record round-trips
// through both supported databases. Normalizing to it means the event a
// decorator hashes is byte-for-byte the event that can be read back, so
// tamper evidence computed before the write still verifies after it.
const storageResolution = time.Microsecond

// Normalize fills in the identity and timestamp a record needs, using now as
// the clock. It is idempotent, so a decorator can normalize an event before
// delegating and the core writer will not reassign it. Normalizing early is
// what makes an event's identity stable across a decorator chain.
func Normalize(event Event, now func() time.Time) Event {
	if event.ID == "" {
		event.ID = newEventID()
	}
	if event.OccurredAt.IsZero() {
		if now == nil {
			now = time.Now
		}
		event.OccurredAt = now()
	}
	event.OccurredAt = event.OccurredAt.UTC().Truncate(storageResolution)
	return event
}

// CoreWriter is the durable audit writer every edition builds on. A write
// either reaches durable storage or returns an error; it has no buffering,
// sampling, or best-effort path.
type CoreWriter struct {
	store Store
	now   func() time.Time
}

// NewCoreWriter returns the core durable writer. A nil clock uses time.Now.
func NewCoreWriter(store Store, now func() time.Time) *CoreWriter {
	if now == nil {
		now = time.Now
	}
	return &CoreWriter{store: store, now: now}
}

// Write implements [Writer].
func (w *CoreWriter) Write(ctx context.Context, event Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	event = Normalize(event, w.now)
	if err := w.store.InsertEvent(ctx, event); err != nil {
		return fmt.Errorf("record audit event %q: %w", event.Action, err)
	}
	return nil
}

var _ Writer = (*CoreWriter)(nil)
