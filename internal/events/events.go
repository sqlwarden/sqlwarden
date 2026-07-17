// Package events is the in-process domain event bus. Core handlers emit
// security-relevant events; sinks are registered through the extension
// registry (community builds have none, so emission is effectively free).
// Events must never contain SQL text, DSNs, bind parameters, row values,
// or credentials.
package events

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	Time       time.Time
	Action     string // dotted verb, e.g. "auth.login", "policy.binding.grant"
	Outcome    string // "success", "failure", or "denied"
	ActorID    int64  // acting account ID; zero when unauthenticated
	OrgID      int64  // owning org ID; zero when not org-scoped
	Resource   string // resource type, e.g. "role_binding"
	ResourceID int64
	Metadata   map[string]string
}

type Sink interface {
	HandleEvent(ctx context.Context, ev Event)
}

type Bus struct {
	mu    sync.RWMutex
	sinks []Sink
}

func NewBus() *Bus { return &Bus{} }

func (b *Bus) Subscribe(s Sink) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sinks = append(b.sinks, s)
}

// Emit fans the event out to all sinks synchronously. Sinks must be fast
// and non-blocking; anything expensive belongs in a job the sink enqueues.
func (b *Bus) Emit(ctx context.Context, ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	b.mu.RLock()
	sinks := b.sinks
	b.mu.RUnlock()
	for _, s := range sinks {
		s.HandleEvent(ctx, ev)
	}
}
