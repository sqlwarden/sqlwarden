package jobs

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/settings"
)

type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&lockedWriter{mu: &c.mu, w: &c.buf}, nil))
}

func (c *logCapture) count(msg string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Count(c.buf.String(), `msg="`+msg+`"`)
}

func testWorkerConfig(id string) WorkerConfig {
	return WorkerConfig{WorkerID: id, WorkerCount: 1, PollInterval: 5 * time.Millisecond, ClaimLease: time.Minute}
}

func jobStatus(t *testing.T, store *DatabaseStore, id string) string {
	t.Helper()
	var record Record
	if err := store.db.NewSelect().Model(&record).Where("id = ?", id).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return record.Status
}

func noopRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(Definition{Type: "noop", Handler: HandlerFunc(func(context.Context, Runtime) (any, error) { return nil, nil })})
	return registry
}

func TestSupervisorApplyStartsRunner(t *testing.T) {
	store, _ := newTestStore(t)
	s := NewSupervisor(store, noopRegistry(), nil)
	defer s.Stop()
	job, err := store.Enqueue(context.Background(), EnqueueInput{Type: "noop", Visibility: VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	s.Apply(context.Background(), testWorkerConfig("sup-a"))
	waitFor(t, 5*time.Second, func() bool { return jobStatus(t, store, job.ID) == StatusSucceeded })
}

func TestSupervisorRestartsOnlyOnConfigChange(t *testing.T) {
	store, _ := newTestStore(t)
	var logs logCapture
	s := NewSupervisor(store, noopRegistry(), logs.logger())
	ctx := context.Background()
	cfg := testWorkerConfig("sup-b")
	s.Apply(ctx, cfg)
	s.Apply(ctx, cfg)
	waitFor(t, 2*time.Second, func() bool { return logs.count("job runner started") == 1 })
	time.Sleep(20 * time.Millisecond)
	if got := logs.count("job runner started"); got != 1 {
		t.Fatalf("runner starts after equal config = %d, want 1", got)
	}

	changed := cfg
	changed.WorkerCount = 2
	s.Apply(ctx, changed)
	waitFor(t, 2*time.Second, func() bool { return logs.count("job runner started") == 2 })
	if got := logs.count("job runner stopped"); got != 1 {
		t.Fatalf("runner stops after config change = %d, want 1", got)
	}
	s.Stop()
	if got := logs.count("job runner stopped"); got != 2 {
		t.Fatalf("runner stops after Stop = %d, want 2", got)
	}
}

func TestSupervisorRestartDrainsInFlightJob(t *testing.T) {
	store, _ := newTestStore(t)
	started, release := make(chan struct{}), make(chan struct{})
	registry := NewRegistry()
	registry.Register(Definition{Type: "drain", Handler: HandlerFunc(func(ctx context.Context, _ Runtime) (any, error) {
		close(started)
		select {
		case <-release:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})})
	job, err := store.Enqueue(context.Background(), EnqueueInput{Type: "drain", Visibility: VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	s := NewSupervisor(store, registry, nil)
	defer s.Stop()
	cfg := testWorkerConfig("sup-c")
	s.Apply(context.Background(), cfg)
	<-started

	applied := make(chan struct{})
	go func() {
		cfg.WorkerCount = 2
		s.Apply(context.Background(), cfg)
		close(applied)
	}()
	select {
	case <-applied:
		t.Fatal("restart returned before the in-flight job finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	<-applied
	if status := jobStatus(t, store, job.ID); status != StatusSucceeded {
		t.Fatalf("drained job status = %s, want succeeded", status)
	}
}

func blockingRegistry(started chan<- struct{}, returned *atomic.Bool) *Registry {
	registry := NewRegistry()
	registry.Register(Definition{Type: "blocking", Handler: HandlerFunc(func(ctx context.Context, _ Runtime) (any, error) {
		defer returned.Store(true)
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})})
	return registry
}

func TestSupervisorStopCancelsAndWaitsForInFlightJob(t *testing.T) {
	store, _ := newTestStore(t)
	started := make(chan struct{}, 1)
	var returned atomic.Bool
	s := NewSupervisor(store, blockingRegistry(started, &returned), nil)
	if _, err := store.Enqueue(context.Background(), EnqueueInput{Type: "blocking", Visibility: VisibilityInternal}); err != nil {
		t.Fatal(err)
	}
	s.Apply(context.Background(), testWorkerConfig("sup-d"))
	<-started

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop blocked on an in-flight job")
	}
	if !returned.Load() {
		t.Fatal("Stop returned before the in-flight job returned")
	}
}

func TestSupervisorStopUnblocksRestartWaitingOnJob(t *testing.T) {
	store, _ := newTestStore(t)
	started := make(chan struct{}, 1)
	var returned atomic.Bool
	s := NewSupervisor(store, blockingRegistry(started, &returned), nil)
	if _, err := store.Enqueue(context.Background(), EnqueueInput{Type: "blocking", Visibility: VisibilityInternal}); err != nil {
		t.Fatal(err)
	}
	cfg := testWorkerConfig("sup-e")
	s.Apply(context.Background(), cfg)
	<-started
	next := cfg
	next.WorkerCount = 3
	go s.Apply(context.Background(), next)
	time.Sleep(30 * time.Millisecond)

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop blocked behind a restart waiting on an in-flight job")
	}
}

func TestSupervisorApplyContextCancelUnblocksRestart(t *testing.T) {
	store, _ := newTestStore(t)
	started := make(chan struct{}, 1)
	var returned atomic.Bool
	var logs logCapture
	s := NewSupervisor(store, blockingRegistry(started, &returned), logs.logger())
	defer s.Stop()
	if _, err := store.Enqueue(context.Background(), EnqueueInput{Type: "blocking", Visibility: VisibilityInternal}); err != nil {
		t.Fatal(err)
	}
	cfg := testWorkerConfig("sup-f")
	s.Apply(context.Background(), cfg)
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	applied := make(chan struct{})
	go func() {
		next := cfg
		next.WorkerCount = 2
		s.Apply(ctx, next)
		close(applied)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-applied:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled Apply stayed blocked on the in-flight job")
	}
	if got := logs.count("job runner started"); got != 1 {
		t.Fatalf("runner starts = %d, want 1: a cancelled Apply must not start a runner", got)
	}
	s.Apply(context.Background(), cfg)
	waitFor(t, 2*time.Second, func() bool { return logs.count("job runner started") == 2 })
}

func TestSupervisorApplyAfterStopIsNoop(t *testing.T) {
	store, _ := newTestStore(t)
	var logs logCapture
	s := NewSupervisor(store, noopRegistry(), logs.logger())
	s.Stop()
	job, err := store.Enqueue(context.Background(), EnqueueInput{Type: "noop", Visibility: VisibilityInternal})
	if err != nil {
		t.Fatal(err)
	}
	s.Apply(context.Background(), testWorkerConfig("sup-g"))
	time.Sleep(50 * time.Millisecond)
	if got := logs.count("job runner started"); got != 0 {
		t.Fatalf("runner started after Stop")
	}
	if status := jobStatus(t, store, job.ID); status != StatusQueued {
		t.Fatalf("job status = %s, want queued", status)
	}
	s.Stop()
}

func TestTwoSupervisorsClaimDisjointJobs(t *testing.T) {
	store, _ := newTestStore(t)
	var mu sync.Mutex
	runs := map[string]int{}
	registry := NewRegistry()
	registry.Register(Definition{Type: "count", Handler: HandlerFunc(func(_ context.Context, rt Runtime) (any, error) {
		mu.Lock()
		runs[rt.Job.ID]++
		mu.Unlock()
		time.Sleep(time.Millisecond)
		return nil, nil
	})})
	const total = 30
	ids := make([]string, 0, total)
	for range total {
		job, err := store.Enqueue(context.Background(), EnqueueInput{Type: "count", Visibility: VisibilityInternal})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, job.ID)
	}
	a := NewSupervisor(store, registry, nil)
	b := NewSupervisor(store, registry, nil)
	cfgA, cfgB := testWorkerConfig(NewWorkerID()), testWorkerConfig(NewWorkerID())
	cfgA.WorkerCount, cfgB.WorkerCount = 3, 3
	a.Apply(context.Background(), cfgA)
	b.Apply(context.Background(), cfgB)
	waitFor(t, 10*time.Second, func() bool {
		for _, id := range ids {
			if jobStatus(t, store, id) != StatusSucceeded {
				return false
			}
		}
		return true
	})
	a.Stop()
	b.Stop()
	mu.Lock()
	defer mu.Unlock()
	for _, id := range ids {
		if runs[id] != 1 {
			t.Fatalf("job %s ran %d times, want 1", id, runs[id])
		}
	}
}

func TestNewWorkerIDIsUniqueAndWellFormed(t *testing.T) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	pattern := regexp.MustCompile("^" + regexp.QuoteMeta(host) + "-" + strconv.Itoa(os.Getpid()) + "-[0-9a-f]{8}$")
	seen := map[string]bool{}
	for range 100 {
		id := NewWorkerID()
		if !pattern.MatchString(id) {
			t.Fatalf("worker id %q does not match %s", id, pattern)
		}
		if seen[id] {
			t.Fatalf("worker id %q repeated", id)
		}
		seen[id] = true
	}
}

func TestWorkerConfigFromSettings(t *testing.T) {
	in := settings.Instance{JobsWorkerCount: 4, JobsPollIntervalSeconds: 2, JobsClaimLeaseSeconds: 300, JobsCompletedRetentionSeconds: 60}
	got := WorkerConfigFromSettings(in, "w-1")
	want := WorkerConfig{WorkerID: "w-1", WorkerCount: 4, PollInterval: 2 * time.Second, ClaimLease: 5 * time.Minute, CompletedRetention: time.Minute}
	if got != want {
		t.Fatalf("config = %s, want %s", fmt.Sprint(got), fmt.Sprint(want))
	}
}
