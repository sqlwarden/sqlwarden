package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/settings"
)

func TestSelectKinds(t *testing.T) {
	tests := []struct {
		selected []string
		wantAPI  bool
		wantJobs bool
	}{
		{[]string{config.ProcessKindAll}, true, true},
		{[]string{config.ProcessKindAPI}, true, false},
		{[]string{config.ProcessKindJobs}, false, true},
		{[]string{config.ProcessKindAPI, config.ProcessKindJobs}, true, true},
	}
	for _, tt := range tests {
		cfg := config.Default()
		cfg.ProcessKinds = tt.selected
		api, jobs := selectKinds(cfg)
		if api != tt.wantAPI || jobs != tt.wantJobs {
			t.Errorf("selectKinds(%v) = api %v jobs %v", tt.selected, api, jobs)
		}
	}
}

func TestJobsOnlyHandlerServesHealthOnly(t *testing.T) {
	health := NewHealth()
	handler := httpHandler(nil, health)
	for path, want := range map[string]int{
		"/livez":            http.StatusOK,
		"/api/setup/status": http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

// kindFakes records the lifecycle calls of fake watcher, supervisor, and
// scheduler parts into one ordered log.
type kindFakes struct {
	mu       sync.Mutex
	events   []string
	appliers []settings.Applier
	applied  []jobs.WorkerConfig
	newJobs  int
}

func (f *kindFakes) record(event string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *kindFakes) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

type fakeWatcher struct{ f *kindFakes }

func (w fakeWatcher) Seed(in settings.Instance) {
	w.f.record(fmt.Sprintf("watcher.seed workers=%d", in.JobsWorkerCount))
}
func (w fakeWatcher) Start(context.Context) { w.f.record("watcher.start") }
func (w fakeWatcher) Stop()                 { w.f.record("watcher.stop") }

type fakeSupervisor struct{ f *kindFakes }

func (s fakeSupervisor) Apply(_ context.Context, cfg jobs.WorkerConfig) {
	s.f.mu.Lock()
	s.f.applied = append(s.f.applied, cfg)
	s.f.mu.Unlock()
	s.f.record(fmt.Sprintf("supervisor.apply workers=%d", cfg.WorkerCount))
}
func (s fakeSupervisor) Stop() { s.f.record("supervisor.stop") }

type fakeScheduler struct{ f *kindFakes }

func (s fakeScheduler) Start(context.Context) { s.f.record("scheduler.start") }
func (s fakeScheduler) Stop()                 { s.f.record("scheduler.stop") }

type fakeHTTPKind struct{ f *kindFakes }

func (k fakeHTTPKind) Name() string                { return "http" }
func (k fakeHTTPKind) Start(context.Context) error { k.f.record("http.start"); return nil }
func (k fakeHTTPKind) Ready(context.Context) error { return nil }
func (k fakeHTTPKind) Close(context.Context) error { k.f.record("http.close"); return nil }

func fakePlan(f *kindFakes, runJobs bool) serveKindsPlan {
	base := func(context.Context, settings.Instance) error { f.record("web.apply"); return nil }
	return serveKindsPlan{
		runJobs:  runJobs,
		initial:  settings.Instance{JobsWorkerCount: 2},
		appliers: []settings.Applier{base},
		workerID: "worker-1",
		newJobs: func() (jobSupervisor, jobScheduler) {
			f.newJobs++
			return fakeSupervisor{f}, fakeScheduler{f}
		},
		newWatcher: func(appliers []settings.Applier) settingsWatcher {
			f.appliers = appliers
			return fakeWatcher{f}
		},
		http: fakeHTTPKind{f},
	}
}

func kindNames(kinds []ProcessKind) []string {
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, kind.Name())
	}
	return names
}

func TestServeKindsForProcessKinds(t *testing.T) {
	tests := []struct {
		selected     []string
		wantKinds    []string
		wantAppliers int
	}{
		{[]string{config.ProcessKindAll}, []string{"jobs", "settings", "http"}, 2},
		{[]string{config.ProcessKindAPI}, []string{"settings", "http"}, 1},
		{[]string{config.ProcessKindJobs}, []string{"jobs", "settings", "http"}, 2},
	}
	for _, tt := range tests {
		cfg := config.Default()
		cfg.ProcessKinds = tt.selected
		_, runJobs := selectKinds(cfg)
		f := &kindFakes{}
		kinds := serveKinds(fakePlan(f, runJobs))
		if got := kindNames(kinds); !slices.Equal(got, tt.wantKinds) {
			t.Errorf("%v: kinds = %v, want %v", tt.selected, got, tt.wantKinds)
		}
		if len(f.appliers) != tt.wantAppliers {
			t.Errorf("%v: watcher appliers = %d, want %d", tt.selected, len(f.appliers), tt.wantAppliers)
		}
		wantBuilt := 0
		if runJobs {
			wantBuilt = 1
		}
		if f.newJobs != wantBuilt {
			t.Errorf("%v: supervisor and scheduler built %d times, want %d", tt.selected, f.newJobs, wantBuilt)
		}
	}
}

func TestAPIProcessStartsNoSupervisorOrScheduler(t *testing.T) {
	f := &kindFakes{}
	built := newKindsApplication(serveKinds(fakePlan(f, false))...)
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"watcher.seed workers=2", "watcher.start", "http.start", "http.close", "watcher.stop"}
	if got := f.log(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestJobsProcessStartAndCloseOrder(t *testing.T) {
	f := &kindFakes{}
	built := newKindsApplication(serveKinds(fakePlan(f, true))...)
	built.resources.push("database", func(context.Context) error { f.record("database.close"); return nil })
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := built.Ready(context.Background()); err != nil {
		t.Fatalf("Ready after Start: %v", err)
	}
	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"supervisor.apply workers=2", "scheduler.start", "watcher.seed workers=2", "watcher.start", "http.start",
		"http.close", "watcher.stop", "scheduler.stop", "supervisor.stop", "database.close",
	}
	if got := f.log(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestSettingsChangeReachesSupervisorInJobsProcess(t *testing.T) {
	f := &kindFakes{}
	kinds := serveKinds(fakePlan(f, true))
	for _, kind := range kinds {
		if err := kind.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	changed := settings.Instance{JobsWorkerCount: 5, JobsPollIntervalSeconds: 3}
	for _, apply := range f.appliers {
		if err := apply(context.Background(), changed); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.applied) != 2 {
		t.Fatalf("supervisor applies = %d, want initial and changed", len(f.applied))
	}
	if want := jobs.WorkerConfigFromSettings(changed, "worker-1"); f.applied[1] != want {
		t.Fatalf("applied config = %+v, want %+v", f.applied[1], want)
	}
	if !slices.Contains(f.log(), "web.apply") {
		t.Fatal("web appliers did not run before the supervisor applier")
	}
}

func TestSettingsAndJobsKindsReadyOnlyAfterStart(t *testing.T) {
	f := &kindFakes{}
	for _, kind := range serveKinds(fakePlan(f, true)) {
		if kind.Name() == "http" {
			continue
		}
		if err := kind.Ready(context.Background()); err == nil {
			t.Errorf("%s ready before Start", kind.Name())
		}
		if err := kind.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := kind.Ready(context.Background()); err != nil {
			t.Errorf("%s not ready after Start: %v", kind.Name(), err)
		}
	}
}
