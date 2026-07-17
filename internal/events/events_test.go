package events

import (
	"context"
	"sync"
	"testing"
)

type captureSink struct {
	mu     sync.Mutex
	events []Event
}

func (s *captureSink) HandleEvent(_ context.Context, ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func TestBusEmitFansOutToAllSinks(t *testing.T) {
	bus := NewBus()
	a := &captureSink{}
	b := &captureSink{}
	bus.Subscribe(a)
	bus.Subscribe(b)

	bus.Emit(context.Background(), Event{Action: "auth.login", Outcome: "success", ActorID: 7})

	for _, s := range []*captureSink{a, b} {
		if len(s.events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(s.events))
		}
		if s.events[0].Action != "auth.login" || s.events[0].ActorID != 7 {
			t.Fatalf("unexpected event: %+v", s.events[0])
		}
		if s.events[0].Time.IsZero() {
			t.Fatal("expected Emit to stamp Time")
		}
	}
}

func TestBusEmitWithNoSinksIsNoOp(t *testing.T) {
	NewBus().Emit(context.Background(), Event{Action: "auth.login"})
}
