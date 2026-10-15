package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/config"
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

// runtimeController is the part of web.App that the runtime kind controls.
type runtimeController interface {
	StartRuntime(runJobs bool)
	StopRuntime()
}

// runtimeKind applies instance settings changes in every serving process and
// runs background jobs only when the process selects jobs.
type runtimeKind struct {
	rt      runtimeController
	runJobs bool
}

func newRuntimeKind(rt runtimeController, runJobs bool) *runtimeKind {
	return &runtimeKind{rt: rt, runJobs: runJobs}
}

func (k *runtimeKind) Name() string                { return "runtime" }
func (k *runtimeKind) Start(context.Context) error { k.rt.StartRuntime(k.runJobs); return nil }
func (k *runtimeKind) Ready(context.Context) error { return nil }
func (k *runtimeKind) Close(context.Context) error { k.rt.StopRuntime(); return nil }
