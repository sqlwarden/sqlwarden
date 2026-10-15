package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSchedulerEnqueuesOnStartAndEveryInterval(t *testing.T) {
	var calls atomic.Int32
	s := NewScheduler(nil, Schedule{Name: "tick", Every: 20 * time.Millisecond, Enqueue: func(context.Context) error {
		calls.Add(1)
		return nil
	}})
	s.Start(context.Background())
	defer s.Stop()
	waitFor(t, 2*time.Second, func() bool { return calls.Load() >= 3 })
}

func TestSchedulerFirstEnqueueIsImmediate(t *testing.T) {
	var calls atomic.Int32
	s := NewScheduler(nil, Schedule{Name: "slow", Every: time.Hour, Enqueue: func(context.Context) error {
		calls.Add(1)
		return nil
	}})
	s.Start(context.Background())
	defer s.Stop()
	waitFor(t, time.Second, func() bool { return calls.Load() == 1 })
}

func TestSchedulerEnqueueErrorDoesNotStopLoop(t *testing.T) {
	var logs bytes.Buffer
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(&lockedWriter{mu: &mu, w: &logs}, nil))
	var calls atomic.Int32
	s := NewScheduler(logger, Schedule{Name: "failing", Every: 10 * time.Millisecond, Enqueue: func(context.Context) error {
		calls.Add(1)
		return errors.New("queue unavailable")
	}})
	s.Start(context.Background())
	waitFor(t, 2*time.Second, func() bool { return calls.Load() >= 3 })
	s.Stop()
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(logs.String(), "job schedule enqueue failed") || !strings.Contains(logs.String(), "schedule=failing") {
		t.Fatalf("enqueue failure not logged: %s", logs.String())
	}
}

func TestSchedulerStopWaitsForEnqueueAndPreventsMore(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var returned atomic.Bool
	var calls atomic.Int32
	s := NewScheduler(nil, Schedule{Name: "blocking", Every: 5 * time.Millisecond, Enqueue: func(context.Context) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			returned.Store(true)
		}
		return nil
	}})
	s.Start(context.Background())
	<-entered
	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned while an enqueue was running")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	<-stopped
	if !returned.Load() {
		t.Fatal("Stop returned before the enqueue finished")
	}
	after := calls.Load()
	s.Start(context.Background())
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != after {
		t.Fatal("scheduler enqueued after Stop")
	}
}

func TestSchedulerStartIsIdempotent(t *testing.T) {
	var calls atomic.Int32
	s := NewScheduler(nil, Schedule{Name: "once", Every: time.Hour, Enqueue: func(context.Context) error {
		calls.Add(1)
		return nil
	}})
	s.Start(context.Background())
	s.Start(context.Background())
	defer s.Stop()
	waitFor(t, time.Second, func() bool { return calls.Load() >= 1 })
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("enqueues = %d, want 1", got)
	}
}

func TestSchedulerRunReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(nil, Schedule{Name: "run", Every: time.Hour, Enqueue: func(context.Context) error { return nil }})
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestSchedulerSingletonIsNotEnqueuedTwiceWhileActive(t *testing.T) {
	store, db := newTestStore(t)
	var calls atomic.Int32
	s := NewScheduler(nil, Schedule{Name: "reap", Every: 5 * time.Millisecond, Enqueue: func(ctx context.Context) error {
		calls.Add(1)
		_, _, err := store.EnqueueSingleton(ctx, EnqueueInput{
			Type: TypeFileContentReap, SingletonKey: TypeFileContentReap, Visibility: VisibilityInternal, Priority: PriorityLow,
		})
		if errors.Is(err, ErrActiveExists) {
			return nil
		}
		return err
	}})
	s.Start(context.Background())
	waitFor(t, 2*time.Second, func() bool { return calls.Load() >= 5 })
	s.Stop()
	count, err := db.NewSelect().Model((*Record)(nil)).Where("type = ?", TypeFileContentReap).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reap jobs = %d, want 1", count)
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
