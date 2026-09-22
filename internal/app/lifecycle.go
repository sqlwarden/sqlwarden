package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ErrNotStarted is returned by [Application.Ready] before [Application.Start].
var ErrNotStarted = errors.New("application has not started")

// Application is a built process: the shared service graph plus the process
// kinds this process runs. Build it with [Build], start background work with
// [Application.Start] or [Application.Run], and always close it.
type Application struct {
	Services     *Services
	ProcessKinds []ProcessKind

	logger           *slog.Logger
	resources        *resourceStack
	shutdownDeadline time.Duration

	mu          sync.Mutex
	started     []ProcessKind
	closed      bool
	startCalled bool
	running     bool
	startedAt   time.Time
}

// Start starts every process kind in construction order. If one fails to start,
// the already-started kinds are closed in reverse order and the error is
// returned; the caller still owns [Application.Close] for the service graph.
func (a *Application) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("application is closed")
	}
	if a.startCalled {
		return errors.New("application start was already attempted")
	}
	a.startCalled = true
	a.startedAt = time.Now()

	// Idle reapers belong to the start phase so Build leaves no goroutine
	// running behind a caller that never starts the process.
	a.Services.ConnManager.StartReaper()
	a.Services.QueryCursors.StartReaper()

	for _, kind := range a.ProcessKinds {
		if err := kind.Start(ctx); err != nil {
			a.logger.Error("process kind failed to start", "process_kind", kind.Name(), "error", err)
			a.closeStartedLocked(context.WithoutCancel(ctx))
			return fmt.Errorf("start process kind %q: %w", kind.Name(), err)
		}
		a.started = append(a.started, kind)
		a.logger.Info("process kind started", "process_kind", kind.Name())
	}
	a.running = true
	return nil
}

// Ready reports whether every started process kind can perform its critical
// work. It returns [ErrNotStarted] before Start and an error naming the first
// process kind that is not ready otherwise.
func (a *Application) Ready(ctx context.Context) error {
	a.mu.Lock()
	started := append([]ProcessKind(nil), a.started...)
	closed := a.closed
	notStarted := !a.startCalled
	running := a.running
	a.mu.Unlock()

	if closed {
		return errors.New("application is closed")
	}
	if notStarted {
		return ErrNotStarted
	}
	if !running {
		return errors.New("application is not running")
	}
	for _, kind := range started {
		if err := kind.Ready(ctx); err != nil {
			return fmt.Errorf("process kind %q is not ready: %w", kind.Name(), err)
		}
	}
	return nil
}

// Run starts the application, waits until ctx is cancelled or a serving process
// kind stops on its own, then closes everything. It returns the first error
// reported by a process kind, or the error from closing.
func (a *Application) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		closeErr := a.Close(context.WithoutCancel(ctx))
		return errors.Join(err, closeErr)
	}

	serveErr := a.waitForStop(ctx)
	closeErr := a.Close(context.WithoutCancel(ctx))
	return errors.Join(serveErr, closeErr)
}

// waitForStop blocks until the context is cancelled or a serving process kind
// reports that it stopped.
func (a *Application) waitForStop(ctx context.Context) error {
	a.mu.Lock()
	started := append([]ProcessKind(nil), a.started...)
	a.mu.Unlock()

	stopped := make(chan error, len(started))
	for _, kind := range started {
		serving, ok := kind.(Serving)
		if !ok {
			continue
		}
		name := kind.Name()
		go func(done <-chan error) {
			err, open := <-done
			if !open {
				stopped <- nil
				return
			}
			if err != nil {
				err = fmt.Errorf("process kind %q stopped: %w", name, err)
			}
			stopped <- err
		}(serving.Done())
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-stopped:
		return err
	}
}

// Close stops started process kinds in reverse start order, then releases the
// service graph in reverse construction order. It is safe to call more than
// once and bounds the whole shutdown by the configured deadline.
func (a *Application) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	a.running = false

	startedAt := time.Now()
	a.logger.Info("stopping application")

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.shutdownDeadline)
	defer cancel()

	err := a.closeStartedLocked(shutdownCtx)
	a.resources.closeAll(shutdownCtx)

	if deadlineErr := shutdownCtx.Err(); errors.Is(deadlineErr, context.DeadlineExceeded) {
		a.logger.Warn("application shutdown exceeded deadline", "deadline_ms", a.shutdownDeadline.Milliseconds())
		err = errors.Join(err, fmt.Errorf("shutdown exceeded %s deadline", a.shutdownDeadline))
	}

	a.logger.Info("application stopped", "duration_ms", time.Since(startedAt).Milliseconds())
	return err
}

// closeStartedLocked closes started process kinds in reverse order. The caller
// must hold a.mu.
func (a *Application) closeStartedLocked(ctx context.Context) error {
	var errs []error
	for i := len(a.started) - 1; i >= 0; i-- {
		kind := a.started[i]
		startedAt := time.Now()
		if err := kind.Close(ctx); err != nil {
			a.logger.Warn("process kind shutdown failed", "process_kind", kind.Name(), "error", err)
			errs = append(errs, fmt.Errorf("close process kind %q: %w", kind.Name(), err))
			continue
		}
		a.logger.Info("process kind stopped", "process_kind", kind.Name(), "duration_ms", time.Since(startedAt).Milliseconds())
	}
	a.started = nil
	return errors.Join(errs...)
}

// resourceStack releases acquired resources in reverse acquisition order. It
// serves both partial-build teardown and normal shutdown so there is only one
// ordering to reason about.
type resourceStack struct {
	logger    *slog.Logger
	resources []namedResource
}

type namedResource struct {
	name  string
	close func(context.Context) error
}

func (s *resourceStack) push(name string, close func(context.Context) error) {
	s.resources = append(s.resources, namedResource{name: name, close: close})
}

// closeAll releases every resource in reverse order. A failing resource is
// logged and does not stop the remaining releases.
func (s *resourceStack) closeAll(ctx context.Context) {
	for i := len(s.resources) - 1; i >= 0; i-- {
		resource := s.resources[i]
		startedAt := time.Now()
		if err := resource.close(ctx); err != nil {
			s.logger.Warn("resource shutdown failed", "resource", resource.name, "error", err)
			continue
		}
		s.logger.Info("resource closed", "resource", resource.name, "duration_ms", time.Since(startedAt).Milliseconds())
	}
	s.resources = nil
}

// ResourceOrder returns the construction order of the resources this
// application owns. Shutdown releases them in reverse.
func (a *Application) ResourceOrder() []string {
	names := make([]string, 0, len(a.resources.resources))
	for _, resource := range a.resources.resources {
		names = append(names, resource.name)
	}
	return names
}
