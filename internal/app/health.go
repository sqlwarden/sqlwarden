package app

import (
	"context"
	"errors"
	"sync"
)

// ErrNotBuilt is returned by a [Health] that no application has claimed yet.
var ErrNotBuilt = errors.New("application has not finished building")

// Health answers infrastructure probes for the process that owns it. Transports
// borrow it from [Services] so an HTTP handler can report process state without
// depending on the application it belongs to.
//
// Liveness means the process is built and has not begun shutting down: a failing
// liveness check is only recoverable by restarting the process. Readiness means
// every started process kind can currently do its work, so a failing readiness
// check should remove the replica from load balancing without restarting it.
type Health struct {
	mu  sync.RWMutex
	app *Application
}

// NewHealth returns a Health that reports [ErrNotBuilt] until [Build] binds it
// to the finished application.
func NewHealth() *Health { return &Health{} }

func (h *Health) bind(app *Application) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.app = app
}

func (h *Health) application() *Application {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.app
}

// Live reports whether the process is built and still running.
func (h *Health) Live() error {
	app := h.application()
	if app == nil {
		return ErrNotBuilt
	}
	return app.live()
}

// Ready reports whether every started process kind can serve its traffic.
func (h *Health) Ready(ctx context.Context) error {
	app := h.application()
	if app == nil {
		return ErrNotBuilt
	}
	return app.Ready(ctx)
}

// live reports whether the application has not started shutting down.
func (a *Application) live() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("application is shutting down")
	}
	return nil
}
