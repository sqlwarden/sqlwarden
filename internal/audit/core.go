package audit

import (
	"context"
	"fmt"
	"time"
)

// Store is the durable audit persistence contract.
type Store interface {
	InsertEvent(ctx context.Context, event Event) error
}

// Reader reads back recorded audit events in durable insertion order.
type Reader interface {
	Events(ctx context.Context, limit int) ([]Event, error)
	EventsAfter(ctx context.Context, afterID string, limit int) ([]Event, error)
}

const storageResolution = time.Microsecond

// Normalize fills in the identity and timestamp a record needs, using now as
// the clock. It is idempotent.
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

// CoreWriter is the durable audit writer every edition builds on.
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
