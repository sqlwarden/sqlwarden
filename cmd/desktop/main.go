//go:build !bindings

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
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
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
	openPaths := append([]string(nil), flags.Args()...)

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
	// Queue file-association launch arguments before Wails starts. Waiting for
	// OnDomReady creates a race with the frontend's first DrainOpenRequests call.
	for _, openPath := range openPaths {
		handleNativeOpenPath(bridge, openPath)
	}
	window := loadWindowState(paths)
	apiHandler := unavailableAPI(startupErr)
	if app != nil && startupErr == nil {
		apiHandler = app.Handler()
	}

	appOptions := &options.App{
		Title:       "SQLWarden",
		Width:       window.Width,
		Height:      window.Height,
		MinWidth:    1024,
		MinHeight:   680,
		AssetServer: &assetserver.Options{Assets: staticFiles, Middleware: desktopRoutingMiddleware(apiHandler)},
		OnStartup:   bridge.startup,
		OnDomReady: func(ctx context.Context) {
			applyWindowPosition(ctx, window)
			wailsruntime.OnFileDrop(ctx, func(_, _ int, paths []string) {
				for _, path := range paths {
					handleNativeOpenPath(bridge, path)
				}
			})
		},
		OnShutdown: func(ctx context.Context) {
			saveWindowState(ctx, paths)
			bridge.shutdown(ctx)
		},
		OnBeforeClose: func(ctx context.Context) bool {
			if !bridge.hasUnsavedChanges() {
				return false
			}
			choice, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
				Type:          wailsruntime.QuestionDialog,
				Title:         "Quit SQLWarden?",
				Message:       "You have unsaved SQL changes. Quit and discard them?",
				Buttons:       []string{"Cancel", "Quit"},
				DefaultButton: "Cancel",
				CancelButton:  "Cancel",
			})
			return err != nil || choice != "Quit"
		},
		Bind:             []interface{}{bridge},
		Menu:             desktopMenu(bridge),
		WindowStartState: window.startState(),
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.sqlwarden.desktop",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				bridge.focusWindow()
				for _, path := range data.Args {
					handleNativeOpenPath(bridge, path)
				}
			},
		},
		BackgroundColour: options.NewRGB(10, 10, 12),
	}
	applyDesktopBranding(appOptions)
	appOptions.Windows.WebviewUserDataPath = filepath.Join(paths.Cache, "webview")
	if appOptions.Mac == nil {
		appOptions.Mac = &mac.Options{}
	}
	appOptions.Mac.OnFileOpen = func(path string) { handleNativeOpenPath(bridge, path) }
	err = wails.Run(appOptions)
	if err != nil {
		return fmt.Errorf("run desktop window: %w", err)
	}
	return nil
}

func desktopRoutingMiddleware(api http.Handler) assetserver.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'self'")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
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
