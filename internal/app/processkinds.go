package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/settings"
	"github.com/sqlwarden/internal/web"
)

// ProcessKind is one runtime responsibility of a process.
type ProcessKind interface {
	// Name is a stable identifier used in logs and lifecycle tests.
	Name() string
	// Start begins background work. ctx governs startup only.
	Start(ctx context.Context) error
	// Ready reports whether the kind can do its critical work.
	Ready(ctx context.Context) error
	// Close stops work and drains within the ctx deadline.
	Close(ctx context.Context) error
}

// Serving is implemented by kinds that run until they fail or close. A value
// on Done is a reason to stop the whole process.
type Serving interface {
	Done() <-chan error
}

// selectKinds maps configured process kinds to the responsibilities built in
// this process.
func selectKinds(cfg config.Config) (api, jobs bool) {
	return cfg.HasProcessKind(config.ProcessKindAPI), cfg.HasProcessKind(config.ProcessKindJobs)
}

// httpHandler serves health on every process. api is nil when the process
// does not serve the API, so API paths return 404.
func httpHandler(api http.Handler, health *Health) http.Handler {
	r := chi.NewRouter()
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		writeProbe(w, health.Live())
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		writeProbe(w, health.Ready(req.Context()))
	})
	if api != nil {
		r.Mount("/", api)
	}
	return r
}

func writeProbe(w http.ResponseWriter, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type httpKind struct {
	server *http.Server
	tls    web.TLSFiles
	logger *slog.Logger
	done   chan error
}

func newHTTPKind(server *http.Server, tls web.TLSFiles, logger *slog.Logger) *httpKind {
	return &httpKind{server: server, tls: tls, logger: logger, done: make(chan error, 1)}
}

func (k *httpKind) Name() string { return "http" }

func (k *httpKind) Start(context.Context) error {
	go func() {
		scheme := "http"
		if k.tls.Enabled {
			scheme = "https"
		}
		k.logger.Info("starting server", slog.Group("server", "addr", k.server.Addr, "scheme", scheme))
		var err error
		if k.tls.Enabled {
			err = k.server.ListenAndServeTLS(k.tls.CertFile, k.tls.KeyFile)
		} else {
			err = k.server.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		k.done <- err
	}()
	return nil
}

func (k *httpKind) Ready(context.Context) error { return nil }

func (k *httpKind) Close(ctx context.Context) error {
	startedAt := time.Now()
	k.logger.Info("server shutdown started", slog.Group("server", "addr", k.server.Addr))
	if err := k.server.Shutdown(ctx); err != nil {
		k.logger.Warn("server shutdown failed", slog.Group("server", "addr", k.server.Addr), "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return err
	}
	k.logger.Info("server shutdown completed", slog.Group("server", "addr", k.server.Addr), "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func (k *httpKind) Done() <-chan error { return k.done }

type settingsWatcher interface {
	Seed(settings.Instance)
	Start(context.Context)
	Stop()
}

type jobSupervisor interface {
	Apply(context.Context, jobs.WorkerConfig)
	Stop()
}

type jobScheduler interface {
	Start(context.Context)
	Stop()
}

// serveKindsPlan describes the process kinds of a serving process. The
// watcher is built once every applier is known, because the jobs kind adds
// its supervisor applier to the same watcher.
type serveKindsPlan struct {
	runJobs  bool
	initial  settings.Instance
	appliers []settings.Applier
	workerID string
	// newJobs is called only when the process runs jobs.
	newJobs    func() (jobSupervisor, jobScheduler)
	newWatcher func(appliers []settings.Applier) settingsWatcher
	http       ProcessKind
}

// serveKinds returns the kinds in start order: jobs (when selected), settings,
// then http. The jobs kind applies the initial worker configuration before the
// watcher can apply a later one. Close runs in reverse, so HTTP drains first,
// then the watcher stops (cancelling the context of any supervisor restart it
// is waiting on, which cancels the job blocking that restart), then the
// scheduler and the supervisor stop, all before the database closes.
func serveKinds(plan serveKindsPlan) []ProcessKind {
	var kinds []ProcessKind
	appliers := plan.appliers
	if plan.runJobs {
		supervisor, scheduler := plan.newJobs()
		jk := newJobsKind(supervisor, scheduler, plan.workerID, plan.initial)
		appliers = append(append([]settings.Applier(nil), appliers...), jk.applier())
		kinds = append(kinds, jk)
	}
	kinds = append(kinds, newSettingsKind(plan.newWatcher(appliers), plan.initial))
	return append(kinds, plan.http)
}

// settingsKind applies instance settings changes made by any replica. It runs
// in every serving process.
type settingsKind struct {
	watcher settingsWatcher
	initial settings.Instance
	started atomic.Bool
}

func newSettingsKind(watcher settingsWatcher, initial settings.Instance) *settingsKind {
	return &settingsKind{watcher: watcher, initial: initial}
}

func (k *settingsKind) Name() string { return "settings" }

func (k *settingsKind) Start(ctx context.Context) error {
	k.watcher.Seed(k.initial)
	k.watcher.Start(context.WithoutCancel(ctx))
	k.started.Store(true)
	return nil
}

func (k *settingsKind) Ready(context.Context) error {
	if !k.started.Load() {
		return errors.New("settings watcher has not started")
	}
	return nil
}

func (k *settingsKind) Close(context.Context) error {
	k.watcher.Stop()
	return nil
}

// jobsKind runs the job supervisor and the job scheduler. It runs only in
// processes that select jobs.
type jobsKind struct {
	supervisor jobSupervisor
	scheduler  jobScheduler
	workerID   string
	initial    settings.Instance
	started    atomic.Bool
}

func newJobsKind(supervisor jobSupervisor, scheduler jobScheduler, workerID string, initial settings.Instance) *jobsKind {
	return &jobsKind{supervisor: supervisor, scheduler: scheduler, workerID: workerID, initial: initial}
}

func (k *jobsKind) Name() string { return "jobs" }

func (k *jobsKind) Start(ctx context.Context) error {
	runCtx := context.WithoutCancel(ctx)
	k.supervisor.Apply(runCtx, jobs.WorkerConfigFromSettings(k.initial, k.workerID))
	k.scheduler.Start(runCtx)
	k.started.Store(true)
	return nil
}

func (k *jobsKind) Ready(context.Context) error {
	if !k.started.Load() {
		return errors.New("job runner has not started")
	}
	return nil
}

// Close stops the scheduler before the supervisor, so no schedule enqueues
// into a process whose runner has stopped. Supervisor.Stop cancels in-flight
// jobs before it waits for them.
func (k *jobsKind) Close(context.Context) error {
	k.scheduler.Stop()
	k.supervisor.Stop()
	return nil
}

// applier reconfigures the supervisor from instance settings, keeping this
// process's worker id.
func (k *jobsKind) applier() settings.Applier {
	return func(ctx context.Context, in settings.Instance) error {
		k.supervisor.Apply(ctx, jobs.WorkerConfigFromSettings(in, k.workerID))
		return nil
	}
}
