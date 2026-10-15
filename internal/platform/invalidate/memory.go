package invalidate

import (
	"context"
	"log/slog"
	"sync"
)

var _ Invalidator = (*Memory)(nil)

// Memory delivers invalidations synchronously to subscribers in the same
// process.
type Memory struct {
	logger *slog.Logger

	mu     sync.RWMutex
	nextID uint64
	subs   map[Topic][]subscription
}

type subscription struct {
	id uint64
	fn func(key string)
}

// NewMemory returns an in-process Invalidator. A nil logger uses slog.Default.
func NewMemory(logger *slog.Logger) *Memory {
	if logger == nil {
		logger = slog.Default()
	}
	return &Memory{logger: logger, subs: make(map[Topic][]subscription)}
}

// Publish calls every current subscriber of the topic in subscription order.
// The context is not consulted: in-process delivery cannot be abandoned
// halfway without leaving caches stale. A panicking subscriber is recovered
// and logged so it cannot affect the publisher or other subscribers.
func (m *Memory) Publish(_ context.Context, topic Topic, key string) error {
	m.mu.RLock()
	current := m.subs[topic]
	m.mu.RUnlock()

	for _, s := range current {
		m.deliver(topic, key, s.fn)
	}
	return nil
}

func (m *Memory) deliver(topic Topic, key string, fn func(string)) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("invalidation subscriber panicked",
				slog.String("topic", string(topic)),
				slog.String("key", key),
				slog.Any("panic", r),
			)
		}
	}()
	fn(key)
}

// Subscribe registers fn for the topic. The subscriber list is copied on
// every change, so Publish iterates a stable snapshot without holding the
// lock and callbacks may subscribe or unsubscribe freely.
func (m *Memory) Subscribe(topic Topic, fn func(key string)) func() {
	m.mu.Lock()
	m.nextID++
	id := m.nextID
	next := make([]subscription, len(m.subs[topic]), len(m.subs[topic])+1)
	copy(next, m.subs[topic])
	m.subs[topic] = append(next, subscription{id: id, fn: fn})
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			old := m.subs[topic]
			next := make([]subscription, 0, len(old))
			for _, s := range old {
				if s.id != id {
					next = append(next, s)
				}
			}
			if len(next) == 0 {
				delete(m.subs, topic)
				return
			}
			m.subs[topic] = next
		})
	}
}
