//go:build !bindings

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sqlwarden/assets"
	desktopconfig "github.com/sqlwarden/internal/desktop"
	"github.com/sqlwarden/internal/web"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("sqlwarden-desktop", flag.ContinueOnError)
	dataDir := flags.String("data-dir", "", "override the desktop data directory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, paths, configErr := desktopconfig.LoadWebConfig(*dataDir)
	logger, closeLog, logErr := desktopLogger(paths)
	if logErr != nil {
		return logErr
	}
	defer closeLog()

	var app *web.App
	startupErr := configErr
	if startupErr == nil {
		app, startupErr = web.New(cfg, logger)
	}
	if app != nil {
		defer app.Close()
		if _, err := app.BootstrapDesktop(context.Background()); err != nil {
			startupErr = err
		}
	}
	if startupErr != nil {
		logger.Error("desktop startup failed", "error", startupErr)
	}

	staticFiles, err := fs.Sub(assets.EmbeddedFiles, "static")
	if err != nil {
		return fmt.Errorf("load desktop assets: %w", err)
	}
	bridge := newDesktopBridge(app, paths, startupErr)
	apiHandler := unavailableAPI(startupErr)
	if app != nil && startupErr == nil {
		apiHandler = app.Handler()
	}

	appOptions := &options.App{
		Title:       "SQLWarden",
		Width:       1440,
		Height:      900,
		MinWidth:    1024,
		MinHeight:   680,
		AssetServer: &assetserver.Options{Assets: staticFiles, Middleware: desktopRoutingMiddleware(apiHandler)},
		OnStartup:   bridge.startup,
		OnShutdown:  bridge.shutdown,
		Bind:        []interface{}{bridge},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.sqlwarden.desktop",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				bridge.focusWindow()
			},
		},
		BackgroundColour: options.NewRGB(10, 10, 12),
	}
	applyDesktopBranding(appOptions)
	err = wails.Run(appOptions)
	if err != nil {
		return fmt.Errorf("run desktop window: %w", err)
	}
	return nil
}

func desktopLogger(paths desktopconfig.Paths) (*slog.Logger, func(), error) {
	writers := []io.Writer{os.Stderr}
	closeLog := func() {}
	if paths.Logs != "" {
		if err := os.MkdirAll(paths.Logs, 0o700); err != nil {
			return nil, closeLog, fmt.Errorf("create desktop log directory: %w", err)
		}
		file, err := os.OpenFile(filepath.Join(paths.Logs, "sqlwarden.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, closeLog, fmt.Errorf("open desktop log: %w", err)
		}
		writers = append(writers, file)
		closeLog = func() { _ = file.Close() }
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo})), closeLog, nil
}

func desktopRoutingMiddleware(api http.Handler) assetserver.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
				api.ServeHTTP(w, r)
				return
			}
			if isSPARoute(r) {
				request := r.Clone(r.Context())
				request.URL.Path = "/"
				next.ServeHTTP(w, request)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSPARoute(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.URL.Path == "/" || path.Ext(r.URL.Path) != "" {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func unavailableAPI(startupErr error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		message := "Desktop services are unavailable."
		if startupErr != nil {
			message = startupErr.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "desktop_startup_failed", "message": message}})
	})
}
