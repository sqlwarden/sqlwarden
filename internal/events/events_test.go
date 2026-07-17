package events

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type captureSink struct {
	mu     sync.Mutex
	events []Event
	err    error
	panic  bool
}

func (s *captureSink) HandleEvent(_ context.Context, ev Event) error {
	if s.panic {
		panic("sink panic")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return s.err
}

func (s *captureSink) snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

func TestBusEmitFansOutAsynchronouslyWithDefensiveCopies(t *testing.T) {
	bus := NewBus()
	a := &captureSink{}
	b := &captureSink{}
	if err := bus.Subscribe(a); err != nil {
		t.Fatal(err)
	}
	if err := bus.Subscribe(b); err != nil {
		t.Fatal(err)
	}

	metadata := map[string]string{"subject": "original"}
	bus.Emit(context.Background(), Event{Action: "auth.login", Outcome: "success", ActorID: 7, Metadata: metadata})
	metadata["subject"] = "mutated"
	bus.Close()

	for _, sink := range []*captureSink{a, b} {
		events := sink.snapshot()
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Action != "auth.login" || events[0].ActorID != 7 {
			t.Fatalf("unexpected event: %+v", events[0])
		}
		if events[0].Time.IsZero() || events[0].Metadata["subject"] != "original" {
			t.Fatalf("event was not stamped and copied: %+v", events[0])
		}
	}
}

func TestBusContainsSinkFailuresAndPanics(t *testing.T) {
	bus := NewBus()
	if err := bus.Subscribe(&captureSink{panic: true}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Subscribe(&captureSink{err: errors.New("failed")}); err != nil {
		t.Fatal(err)
	}
	healthy := &captureSink{}
	if err := bus.Subscribe(healthy); err != nil {
		t.Fatal(err)
	}

	bus.Emit(context.Background(), Event{Action: "test"})
	bus.Close()
	if len(healthy.snapshot()) != 1 {
		t.Fatal("healthy sink did not receive event after other sink failures")
	}
}

func TestBusRejectsInvalidOrLateSubscriptions(t *testing.T) {
	bus := NewBus()
	if err := bus.Subscribe(nil); err == nil {
		t.Fatal("expected nil sink error")
	}
	bus.Close()
	if err := bus.Subscribe(&captureSink{}); !errors.Is(err, ErrBusClosed) {
		t.Fatalf("Subscribe after Close error = %v, want ErrBusClosed", err)
	}
	bus.Emit(context.Background(), Event{Action: "ignored"})
}
