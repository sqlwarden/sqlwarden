package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
)

// ProcessKinds builds the process kinds this binary can serve from an
// already-constructed service graph. It is passed to app.Build, which owns the
// services and calls this once construction has finished.
func ProcessKinds(services *coreapp.Services) ([]coreapp.ProcessKind, error) {
	kinds := make([]coreapp.ProcessKind, 0, len(services.Config.ProcessKinds))
	for _, name := range services.Config.ProcessKinds {
		switch name {
		case config.ProcessKindAll:
			kinds = append(kinds, newAllProcessKind(NewApplication(services)))
		case config.ProcessKindAPI:
			kinds = append(kinds, newAPIProcessKind(NewApplication(services)))
		case config.ProcessKindConnector:
			kinds = append(kinds, newConnectorProcessKind(services))
		default:
			return nil, fmt.Errorf("process kind %q is not implemented yet", name)
		}
	}
	return kinds, nil
}

// apiProcessKind serves the public HTTP API and runtime-settings supervisor.
// Target database work is delegated through services.Execution when the
// connector process kind is not co-located.
type apiProcessKind struct {
	app     *application
	server  *http.Server
	done    chan error
	serving atomic.Bool
}

func newAPIProcessKind(app *application) *apiProcessKind {
	return &apiProcessKind{app: app, done: make(chan error, 1)}
}

func (k *apiProcessKind) Name() string { return config.ProcessKindAPI }

func (k *apiProcessKind) Start(ctx context.Context) error {
	initialSettings, err := k.app.instanceSettings(ctx)
	if err != nil {
		return err
	}
	if err := k.app.applyRuntimeOperations(initialSettings); err != nil {
		return err
	}
	if _, err := k.app.backfillConnectionTLSConfig(ctx); err != nil {
		k.app.logger.Warn("connection tls backfill failed; will retry next boot", slog.Any("error", err))
	}
	listener, err := k.app.listen()
	if err != nil {
		return fmt.Errorf("bind HTTP listener: %w", err)
	}
	k.app.jobRegistry = k.app.defaultJobRegistry()
	k.app.startRuntimeSupervisor(initialSettings)
	k.app.startFileContentDeletionReaper()
	server := k.app.newHTTPServer()
	k.server = server
	k.serving.Store(true)
	go func() {
		err := k.app.serve(server, listener)
		k.serving.Store(false)
		k.done <- err
	}()
	return nil
}

func (k *apiProcessKind) Ready(ctx context.Context) error {
	if !k.serving.Load() {
		return errors.New("http listener is not serving")
	}
	if _, err := k.app.instanceSettings(ctx); err != nil {
		return fmt.Errorf("runtime settings unavailable: %w", err)
	}
	return nil
}

func (k *apiProcessKind) Close(ctx context.Context) error {
	var errs []error
	if k.server != nil {
		errs = append(errs, k.server.Shutdown(ctx))
		k.server = nil
	}
	if k.app.fileReaperCancel != nil {
		k.app.fileReaperCancel()
		k.app.fileReaperCancel = nil
	}
	if k.app.runtimeCancel != nil {
		k.app.runtimeCancel()
		k.app.runtimeCancel = nil
	}
	k.app.wg.Wait()
	return errors.Join(errs...)
}

func (k *apiProcessKind) Done() <-chan error { return k.done }

// connectorProcessKind owns the internal execution listener and the local
// runtime behind it. It intentionally exposes no public HTTP routes.
// It serves probes on a second plain HTTP listener because the execution
// listener may require transport credentials a probe client cannot present.
type connectorProcessKind struct {
	services     *coreapp.Services
	server       *http.Server
	healthServer *http.Server
	done         chan error
	serving      atomic.Bool
}

func newConnectorProcessKind(services *coreapp.Services) *connectorProcessKind {
	return &connectorProcessKind{services: services, done: make(chan error, 1)}
}

func (k *connectorProcessKind) Name() string { return config.ProcessKindConnector }

func (k *connectorProcessKind) Start(context.Context) error {
	listener, err := net.Listen("tcp", k.services.Config.Connector.ListenAddress)
	if err != nil {
		return fmt.Errorf("bind connector listener: %w", err)
	}
	server := &http.Server{
		Addr:         k.services.Config.Connector.ListenAddress,
		Handler:      k.services.ExecutionServer.Handler(),
		ErrorLog:     slog.NewLogLogger(k.services.Logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: 0,
	}
	healthListener, err := net.Listen("tcp", k.services.Config.Connector.HealthAddress)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("bind connector health listener: %w", err)
	}
	healthServer := &http.Server{
		Handler:           healthHandler(k.services.Health, k.services.Config.ProcessKinds),
		ErrorLog:          slog.NewLogLogger(k.services.Logger.Handler(), slog.LevelWarn),
		ReadHeaderTimeout: defaultReadTimeout,
	}
	k.healthServer = healthServer
	go func() {
		if err := healthServer.Serve(healthListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			k.services.Logger.Warn("connector health listener stopped", slog.Any("error", err))
		}
	}()

	k.server = server
	k.serving.Store(true)
	go func() {
		err := k.services.ConnectorServerCredentials.Serve(server, listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		k.serving.Store(false)
		k.done <- err
	}()
	return nil
}

func (k *connectorProcessKind) Ready(context.Context) error {
	if !k.serving.Load() {
		return errors.New("connector listener is not serving")
	}
	return nil
}

func (k *connectorProcessKind) Close(ctx context.Context) error {
	var errs []error
	if k.server != nil {
		errs = append(errs, k.server.Shutdown(ctx))
		k.server = nil
	}
	if k.healthServer != nil {
		errs = append(errs, k.healthServer.Shutdown(ctx))
		k.healthServer = nil
	}
	return errors.Join(errs...)
}

func (k *connectorProcessKind) Done() <-chan error { return k.done }

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

	server := app.newHTTPServer()
	k.server = server
	k.serving.Store(true)
	go func() {
		err := app.serve(server, listener)
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
