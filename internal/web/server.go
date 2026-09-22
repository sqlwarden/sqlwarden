package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	defaultIdleTimeout  = time.Minute
	defaultReadTimeout  = 5 * time.Second
	defaultWriteTimeout = 10 * time.Second
)

// newHTTPServer builds the process HTTP server. Starting and shutting it down
// belongs to the process kind that owns it.
func (app *application) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.HTTPPort),
		Handler:      app.Handler(),
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}
}

// listen binds the configured HTTP address without accepting traffic. Process
// kinds use it during Start so a bind failure is synchronous and readiness is
// never reported for a listener that failed to acquire its port.
func (app *application) listen() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", app.config.HTTPPort))
}

// serve accepts requests on an already-bound listener until the server stops.
// A shutdown requested by the owner is reported as success; anything else is a
// listener failure.
func (app *application) serve(srv *http.Server, listener net.Listener) error {
	scheme := "http"
	if app.config.TLS.Enabled {
		scheme = "https"
	}
	app.logger.Info("starting server", slog.Group("server", "addr", srv.Addr, "scheme", scheme))

	var err error
	if app.config.TLS.Enabled {
		err = srv.ServeTLS(listener, app.config.TLS.CertFile, app.config.TLS.KeyFile)
	} else {
		err = srv.Serve(listener)
	}
	if errors.Is(err, http.ErrServerClosed) {
		app.logger.Info("stopped server", slog.Group("server", "addr", srv.Addr))
		return nil
	}
	return err
}
