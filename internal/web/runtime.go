package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
)

// ProcessKinds builds the process kinds this binary can serve from an
// already-constructed service graph. It is passed to app.Build, which owns the
// services and calls this once construction has finished.
//
// Only config.ProcessKindAll is implemented. The remaining kinds are named in
// configuration so the topology can be described before the processes are
// split apart, and selecting one of them fails at startup rather than silently
// running fewer responsibilities than the operator asked for.
func ProcessKinds(services *coreapp.Services) ([]coreapp.ProcessKind, error) {
	kinds := make([]coreapp.ProcessKind, 0, len(services.Config.ProcessKinds))
	for _, name := range services.Config.ProcessKinds {
		switch name {
		case config.ProcessKindAll:
			kinds = append(kinds, newAllProcessKind(NewApplication(services)))
		default:
			return nil, fmt.Errorf("process kind %q is not implemented yet; only %q is available", name, config.ProcessKindAll)
		}
	}
	return kinds, nil
}

// allProcessKind runs every runtime responsibility in a single process: the
// HTTP transport, the job worker pool and its runtime-settings supervisor, and
// the file content deletion reaper.
//
// Start brings them up in dependency order and Close takes them down in the
// reverse order, so in-flight HTTP requests finish before the workers they
// depend on stop.
type allProcessKind struct {
	app    *application
	server *http.Server
	done   chan error
	// serving tracks whether the successfully bound listener is still running.
	serving atomic.Bool
}

func newAllProcessKind(app *application) *allProcessKind {
	return &allProcessKind{app: app, done: make(chan error, 1)}
}

// Name implements coreapp.ProcessKind.
func (k *allProcessKind) Name() string {
	return config.ProcessKindAll
}

// Start applies database-backed runtime settings, starts the background
// workers, and begins serving HTTP. It returns as soon as the listener
// goroutine is running; [allProcessKind.Done] reports the listener result.
func (k *allProcessKind) Start(ctx context.Context) error {
	app := k.app

	initialSettings, err := app.instanceSettings(ctx)
	if err != nil {
		return err
	}
	if err := app.applyRuntimeOperations(initialSettings); err != nil {
		return err
	}
	if _, err := app.backfillConnectionTLSConfig(ctx); err != nil {
		app.logger.Warn("connection tls backfill failed; will retry next boot", slog.Any("error", err))
	}
	listener, err := app.listen()
	if err != nil {
		return fmt.Errorf("bind HTTP listener: %w", err)
	}

	app.jobRegistry = app.defaultJobRegistry()
	app.startRuntimeSupervisor(initialSettings)
	app.startFileContentDeletionReaper()

	k.server = app.newHTTPServer()
	k.serving.Store(true)
	go func() {
		err := app.serve(k.server, listener)
		k.serving.Store(false)
		k.done <- err
	}()
	return nil
}

// Ready reports whether the process can serve traffic: the listener goroutine
// is running and database-backed runtime settings are readable.
func (k *allProcessKind) Ready(ctx context.Context) error {
	if !k.serving.Load() {
		return errors.New("http listener is not serving")
	}
	if _, err := k.app.instanceSettings(ctx); err != nil {
		return fmt.Errorf("runtime settings unavailable: %w", err)
	}
	return nil
}

// Close drains HTTP requests first, then stops the background workers the
// handlers depend on. The context deadline bounds the whole drain.
func (k *allProcessKind) Close(ctx context.Context) error {
	app := k.app
	var errs []error

	if k.server != nil {
		startedAt := time.Now()
		app.logger.Info("server shutdown started", slog.Group("server", "addr", k.server.Addr))
		if err := k.server.Shutdown(ctx); err != nil {
			app.logger.Warn("server shutdown failed", slog.Group("server", "addr", k.server.Addr), "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
			errs = append(errs, err)
		} else {
			app.logger.Info("server shutdown completed", slog.Group("server", "addr", k.server.Addr), "duration_ms", time.Since(startedAt).Milliseconds())
		}
		k.server = nil
	}

	workersStartedAt := time.Now()
	if app.fileReaperCancel != nil {
		app.fileReaperCancel()
		app.fileReaperCancel = nil
	}
	if app.runtimeCancel != nil {
		app.runtimeCancel()
		app.runtimeCancel = nil
	}
	app.wg.Wait()
	app.logger.Info("background workers stopped", "duration_ms", time.Since(workersStartedAt).Milliseconds())

	return errors.Join(errs...)
}

// Done reports the listener result once serving stops.
func (k *allProcessKind) Done() <-chan error {
	return k.done
}
