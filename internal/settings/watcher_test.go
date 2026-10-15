package settings_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/platform/invalidate"
	"github.com/sqlwarden/internal/settings"
)

// lockedStore lets the watcher goroutine read the instance row while the
// test changes it.
type lockedStore struct {
	mu sync.Mutex
	*fakeStore
}

func (s *lockedStore) GetInstanceSettings(ctx context.Context) (database.InstanceSettings, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeStore.GetInstanceSettings(ctx)
}

func (s *lockedStore) UpsertInstanceSettings(ctx context.Context, in database.InstanceSettings) (database.InstanceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeStore.UpsertInstanceSettings(ctx, in)
}

func (s *lockedStore) mutate(fn func(*database.InstanceSettings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.instance)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type watcherFixture struct {
	store  *lockedStore
	svc    *settings.Service
	bus    *invalidate.Memory
	logs   *syncBuffer
	logger *slog.Logger
}

func newWatcherFixture() *watcherFixture {
	store := &lockedStore{fakeStore: newFakeStore()}
	bus := invalidate.NewMemory(nil)
	logs := &syncBuffer{}
	return &watcherFixture{
		store: store,
		svc: settings.New(settings.Deps{
			Store:       store,
			Requirer:    access.NewRequirer(fakeDirectory{}, fakePolicy{}),
			Sealer:      fakeSealer{},
			Invalidator: bus,
		}),
		bus:    bus,
		logs:   logs,
		logger: slog.New(slog.NewTextHandler(logs, nil)),
	}
}

// watch builds a seeded watcher with the given poll and stops it on cleanup.
func (f *watcherFixture) watch(t *testing.T, poll time.Duration, inv invalidate.Invalidator, appliers ...settings.Applier) *settings.Watcher {
	t.Helper()
	w := settings.NewWatcher(f.svc, inv, f.logger, appliers...)
	settings.SetWatcherPoll(w, poll)
	initial, err := f.svc.Instance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w.Seed(initial)
	w.Start(context.Background())
	t.Cleanup(w.Stop)
	return w
}

func recordInto(ch chan settings.Instance) settings.Applier {
	return func(_ context.Context, in settings.Instance) error {
		ch <- in
		return nil
	}
}

func receive(t *testing.T, ch chan settings.Instance, within time.Duration) settings.Instance {
	t.Helper()
	select {
	case in := <-ch:
		return in
	case <-time.After(within):
		t.Fatalf("applier not called within %s", within)
		return settings.Instance{}
	}
}

func expectNone(t *testing.T, ch chan settings.Instance, within time.Duration) {
	t.Helper()
	select {
	case in := <-ch:
		t.Fatalf("applier called unexpectedly with log level %q", in.LogLevel)
	case <-time.After(within):
	}
}

func publish(t *testing.T, bus invalidate.Invalidator) {
	t.Helper()
	if err := bus.Publish(context.Background(), invalidate.TopicInstanceSettings, settings.InstanceKey); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherPublishWakesImmediately(t *testing.T) {
	f := newWatcherFixture()
	applied := make(chan settings.Instance, 4)
	f.watch(t, time.Hour, f.bus, recordInto(applied))

	f.store.mutate(func(in *database.InstanceSettings) { in.LogLevel = "debug" })
	publish(t, f.bus)
	if got := receive(t, applied, time.Second); got.LogLevel != "debug" {
		t.Fatalf("applied log level %q, want debug", got.LogLevel)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(f.logs.String(), "runtime operations updated") {
		if time.Now().After(deadline) {
			t.Fatalf("success not logged: %s", f.logs.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWatcherSkipsUnchangedSettings(t *testing.T) {
	f := newWatcherFixture()
	applied := make(chan settings.Instance, 4)
	f.watch(t, 5*time.Millisecond, f.bus, recordInto(applied))

	publish(t, f.bus)
	expectNone(t, applied, 100*time.Millisecond)

	f.store.mutate(func(in *database.InstanceSettings) { in.AccessLogsEnabled = !in.AccessLogsEnabled })
	receive(t, applied, time.Second)
	publish(t, f.bus)
	expectNone(t, applied, 100*time.Millisecond)
}

func TestWatcherIgnoresTimestampOnlyChanges(t *testing.T) {
	f := newWatcherFixture()
	applied := make(chan settings.Instance, 4)
	f.watch(t, 5*time.Millisecond, f.bus, recordInto(applied))

	f.store.mutate(func(in *database.InstanceSettings) { in.UpdatedAt = in.UpdatedAt.Add(time.Minute) })
	publish(t, f.bus)
	expectNone(t, applied, 100*time.Millisecond)
}

func TestWatcherApplierErrorKeepsPreviousState(t *testing.T) {
	f := newWatcherFixture()
	var failures atomic.Int32
	failures.Store(1)
	attempts := make(chan settings.Instance, 4)
	applied := make(chan settings.Instance, 4)
	failing := func(_ context.Context, in settings.Instance) error {
		attempts <- in
		if failures.Add(-1) >= 0 {
			return errors.New("smtp rejected")
		}
		return nil
	}
	f.watch(t, time.Hour, f.bus, failing, recordInto(applied))

	f.store.mutate(func(in *database.InstanceSettings) {
		in.LogLevel = "warn"
		in.SMTPPasswordEncrypted = "sealed:hunter2"
	})
	publish(t, f.bus)
	receive(t, attempts, time.Second)
	expectNone(t, applied, 50*time.Millisecond)
	logs := f.logs.String()
	if !strings.Contains(logs, "runtime operations update rejected") || !strings.Contains(logs, "smtp rejected") {
		t.Fatalf("applier error not logged: %s", logs)
	}
	if strings.Contains(logs, "hunter2") {
		t.Fatal("log contains the SMTP password")
	}

	publish(t, f.bus)
	receive(t, attempts, time.Second)
	if got := receive(t, applied, time.Second); got.LogLevel != "warn" {
		t.Fatalf("retry applied log level %q, want warn", got.LogLevel)
	}
}

func TestWatcherPollAppliesWithoutMessage(t *testing.T) {
	f := newWatcherFixture()
	applied := make(chan settings.Instance, 4)
	f.watch(t, 10*time.Millisecond, nil, recordInto(applied))

	f.store.mutate(func(in *database.InstanceSettings) { in.DatabaseQueryTracingEnabled = !in.DatabaseQueryTracingEnabled })
	receive(t, applied, time.Second)
}

func TestWatcherAppliesServiceUpdate(t *testing.T) {
	f := newWatcherFixture()
	applied := make(chan settings.Instance, 4)
	f.watch(t, time.Hour, f.bus, recordInto(applied))

	if _, err := f.svc.UpdateInstance(context.Background(), instanceAdmin(adminID), settings.UpdateInstanceInput{LogLevel: ptr("error")}); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, applied, time.Second); got.LogLevel != "error" {
		t.Fatalf("applied log level %q, want error", got.LogLevel)
	}
}

func TestWatcherStopWaitsAndLeavesNoGoroutine(t *testing.T) {
	f := newWatcherFixture()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	blocking := func(context.Context, settings.Instance) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	}
	before := runtime.NumGoroutine()
	w := settings.NewWatcher(f.svc, f.bus, f.logger, blocking)
	settings.SetWatcherPoll(w, time.Hour)
	w.Start(context.Background())
	w.Start(context.Background())

	publish(t, f.bus)
	<-entered
	stopped := make(chan struct{})
	go func() {
		w.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned while an applier was running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after the applier finished")
	}
	w.Stop()

	f.store.mutate(func(in *database.InstanceSettings) { in.LogLevel = "debug" })
	publish(t, f.bus)
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("%d goroutines still running after Stop", after-before)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("applier called %d times, want 1", got)
	}
}
