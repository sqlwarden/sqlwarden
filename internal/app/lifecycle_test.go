package app

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// recordingKind is a process kind that records its lifecycle calls into a
// shared log so tests can assert start and close ordering.
type recordingKind struct {
	name     string
	events   *[]string
	startErr error
	closeErr error
	readyErr error
}

func newRecordingKind(name string, events *[]string) *recordingKind {
	return &recordingKind{name: name, events: events}
}

func (k *recordingKind) Name() string { return k.name }

func (k *recordingKind) Start(context.Context) error {
	*k.events = append(*k.events, "start:"+k.name)
	return k.startErr
}

func (k *recordingKind) Ready(context.Context) error { return k.readyErr }

func (k *recordingKind) Close(context.Context) error {
	*k.events = append(*k.events, "close:"+k.name)
	return k.closeErr
}

// servingKind reports a listener result, so tests can drive Run the way an HTTP
// process kind does.
type servingKind struct {
	*recordingKind
	done chan error
}

func newServingKind(name string, events *[]string) *servingKind {
	return &servingKind{recordingKind: newRecordingKind(name, events), done: make(chan error, 1)}
}

func (k *servingKind) Done() <-chan error { return k.done }

// newKindsApplication returns an application that owns only the given process
// kinds, so lifecycle ordering can be tested without building resources.
func newKindsApplication(kinds ...ProcessKind) *Application {
	logger := discardLogger()
	return &Application{
		logger:           logger,
		resources:        &resourceStack{logger: logger},
		shutdownDeadline: 5 * time.Second,
		kinds:            kinds,
	}
}

func TestStartAndCloseRunInReverseOrder(t *testing.T) {
	var events []string
	built := newKindsApplication(newRecordingKind("http", &events), newRecordingKind("runtime", &events))

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := "start:http,start:runtime,close:runtime,close:http"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("lifecycle events = %q, want %q", got, want)
	}
}

func TestCloseReleasesResourcesInReverseOrder(t *testing.T) {
	var events []string
	built := newKindsApplication(newRecordingKind("http", &events))
	for _, name := range []string{"database", "sessions", "cursors", "web"} {
		built.resources.push(name, func(context.Context) error {
			events = append(events, "release:"+name)
			return nil
		})
	}
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := "start:http,close:http,release:web,release:cursors,release:sessions,release:database"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("lifecycle events = %q, want %q", got, want)
	}
}

func TestStartClosesAlreadyStartedKindsWhenOneFails(t *testing.T) {
	var events []string
	failing := newRecordingKind("runtime", &events)
	failing.startErr = errors.New("listener bind failed")
	built := newKindsApplication(newRecordingKind("http", &events), failing, newRecordingKind("extra", &events))

	err := built.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to fail")
	}
	if !strings.Contains(err.Error(), `start process kind "runtime"`) {
		t.Fatalf("error = %v, want the failing process kind named", err)
	}

	want := "start:http,start:runtime,close:http"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("lifecycle events = %q, want %q", got, want)
	}
}

func TestCloseIsIdempotentAndBlocksRestart(t *testing.T) {
	var events []string
	built := newKindsApplication(newRecordingKind("http", &events))

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
	if got := strings.Join(events, ","); got != "start:http,close:http" {
		t.Fatalf("lifecycle events = %q, want a single close", got)
	}
	if err := built.Start(context.Background()); err == nil {
		t.Fatal("expected Start after Close to fail")
	}
}

func TestStartCannotRunProcessKindsTwice(t *testing.T) {
	var events []string
	built := newKindsApplication(newRecordingKind("http", &events))
	t.Cleanup(func() { _ = built.Close(context.Background()) })

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Start(context.Background()); err == nil {
		t.Fatal("expected a second Start call to fail")
	}
	if got := strings.Join(events, ","); got != "start:http" {
		t.Fatalf("lifecycle events = %q, want one start", got)
	}
}

func TestCloseReportsProcessKindFailure(t *testing.T) {
	var events []string
	failing := newRecordingKind("http", &events)
	failing.closeErr = errors.New("drain failed")
	built := newKindsApplication(failing)

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := built.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), `close process kind "http"`) {
		t.Fatalf("Close error = %v, want the failing process kind named", err)
	}
}

func TestReadyReportsLifecycleState(t *testing.T) {
	var events []string
	kind := newRecordingKind("http", &events)
	built := newKindsApplication(kind)

	if err := built.Ready(context.Background()); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Ready before Start = %v, want %v", err, ErrNotStarted)
	}
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Ready(context.Background()); err != nil {
		t.Fatalf("Ready after Start = %v, want nil", err)
	}

	kind.readyErr = errors.New("database unreachable")
	err := built.Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), `process kind "http" is not ready`) {
		t.Fatalf("Ready = %v, want the unready process kind named", err)
	}

	kind.readyErr = nil
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Ready(context.Background()); err == nil {
		t.Fatal("expected Ready to fail after Close")
	}
}

func TestLiveReportsShutdown(t *testing.T) {
	health := NewHealth()
	if err := health.Live(); err != nil {
		t.Fatalf("Live before bind = %v, want nil", err)
	}
	built := newKindsApplication()
	health.bind(built)
	if err := health.Live(); err != nil {
		t.Fatalf("Live after bind = %v, want nil", err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := health.Live(); err == nil {
		t.Fatal("expected Live to fail after Close")
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	var events []string
	built := newKindsApplication(newServingKind("http", &events))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := built.Run(ctx); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if got := strings.Join(events, ","); got != "start:http,close:http" {
		t.Fatalf("lifecycle events = %q", got)
	}
}

func TestRunStopsWhenServingProcessKindFails(t *testing.T) {
	var events []string
	kind := newServingKind("http", &events)
	built := newKindsApplication(kind)

	kind.done <- errors.New("listener failed")

	err := built.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), `process kind "http" stopped`) {
		t.Fatalf("Run = %v, want the failed listener reported", err)
	}
	if got := strings.Join(events, ","); got != "start:http,close:http" {
		t.Fatalf("lifecycle events = %q", got)
	}
}

// settledGoroutines waits for goroutine teardown to finish before counting, so
// a reaper that has been told to stop is not mistaken for a leak.
func settledGoroutines() int {
	count := runtime.NumGoroutine()
	for range 50 {
		time.Sleep(10 * time.Millisecond)
		next := runtime.NumGoroutine()
		if next >= count {
			return count
		}
		count = next
	}
	return count
}
