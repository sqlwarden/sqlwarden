// Package events provides a best-effort, in-process domain event bus for
// decoupled integrations. It is intentionally not an audit log: compliance
// events require durable, transactional persistence before request commit.
// Events must never contain SQL text, DSNs, bind parameters, row values, or
// credentials.
package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const defaultSinkQueueCapacity = 256

var ErrBusClosed = errors.New("event bus is closed")

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

// Sink handles events outside the request path. Implementations must honor
// context cancellation and return processing failures for operator logging.
type Sink interface {
	HandleEvent(ctx context.Context, ev Event) error
}

type delivery struct {
	ctx context.Context
	ev  Event
}

type subscription struct {
	sink  Sink
	queue chan delivery
}

type Bus struct {
	mu            sync.RWMutex
	logger        *slog.Logger
	subscriptions []*subscription
	closed        bool
	wg            sync.WaitGroup
}

func NewBus(loggers ...*slog.Logger) *Bus {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Bus{logger: logger}
}

// Subscribe starts an isolated worker for the sink. One slow sink cannot
// block requests or delivery to another sink.
func (b *Bus) Subscribe(sink Sink) error {
	if sink == nil {
		return errors.New("event sink is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrBusClosed
	}
	sub := &subscription{sink: sink, queue: make(chan delivery, defaultSinkQueueCapacity)}
	b.subscriptions = append(b.subscriptions, sub)
	b.wg.Add(1)
	go b.runSink(sub)
	return nil
}

// Emit queues one defensive event copy per sink and never waits for sink I/O.
// A full sink queue drops that sink's copy and emits an operator warning.
func (b *Bus) Emit(ctx context.Context, ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	ctx = context.WithoutCancel(ctx)

	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, sub := range b.subscriptions {
		item := delivery{ctx: ctx, ev: cloneEvent(ev)}
		select {
		case sub.queue <- item:
		default:
			b.logger.WarnContext(ctx, "domain event dropped because sink queue is full",
				"event.action", ev.Action,
				"event.outcome", ev.Outcome,
			)
		}
	}
}

// Close stops accepting events, drains queued deliveries, and waits for sink
// workers. It is safe to call more than once.
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	for _, sub := range b.subscriptions {
		close(sub.queue)
	}
	b.mu.Unlock()
	b.wg.Wait()
}

func (b *Bus) runSink(sub *subscription) {
	defer b.wg.Done()
	for item := range sub.queue {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					b.logger.ErrorContext(item.ctx, "domain event sink panicked",
						"event.action", item.ev.Action,
						"panic", fmt.Sprint(recovered),
					)
				}
			}()
			if err := sub.sink.HandleEvent(item.ctx, item.ev); err != nil {
				b.logger.ErrorContext(item.ctx, "domain event sink failed",
					"event.action", item.ev.Action,
					"event.outcome", item.ev.Outcome,
					"error", err,
				)
			}
		}()
	}
}

func cloneEvent(ev Event) Event {
	if ev.Metadata == nil {
		return ev
	}
	metadata := make(map[string]string, len(ev.Metadata))
	for key, value := range ev.Metadata {
		metadata[key] = value
	}
	ev.Metadata = metadata
	return ev
}
