package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	defaultIdleTimeout    = time.Minute
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultShutdownPeriod = 30 * time.Second
)

func (app *application) ServeHTTP(ctx context.Context) error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.HTTPPort),
		Handler:      app.Handler(),
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
		IdleTimeout:  defaultIdleTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	shutdownErrorChan := make(chan error)

	go func() {
		<-ctx.Done()

		startedAt := time.Now()
		app.logger.Info("server shutdown started", slog.Group("server", "addr", srv.Addr), "timeout_ms", defaultShutdownPeriod.Milliseconds())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
		defer cancel()

		err := srv.Shutdown(shutdownCtx)
		if err != nil {
			app.logger.Warn("server shutdown failed", slog.Group("server", "addr", srv.Addr), "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		} else {
			app.logger.Info("server shutdown completed", slog.Group("server", "addr", srv.Addr), "duration_ms", time.Since(startedAt).Milliseconds())
		}
		shutdownErrorChan <- err
	}()

	scheme := "http"
	if app.config.TLS.Enabled {
		scheme = "https"
	}
	app.logger.Info("starting server", slog.Group("server", "addr", srv.Addr, "scheme", scheme))

	var err error
	if app.config.TLS.Enabled {
		err = srv.ListenAndServeTLS(app.config.TLS.CertFile, app.config.TLS.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdownErrorChan
	if err != nil {
		return err
	}

	app.logger.Info("stopped server", slog.Group("server", "addr", srv.Addr))

	return nil
}

func (app *application) serveHTTP() error {
	return app.ServeHTTP(context.Background())
}
