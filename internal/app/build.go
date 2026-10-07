package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/web"
)

// sessionIdleTimeout bounds how long an idle target session or query cursor
// survives before its reaper closes it.
const sessionIdleTimeout = 30 * time.Minute

// Options configures Build. Only Config is required.
type Options struct {
	Config config.Config
	Logger *slog.Logger
	// Command selects a one-shot command. One-shot commands build no process
	// kinds, so they never listen or start background work.
	Command Command
}

// Command selects what the process does after Build.
type Command int

const (
	CommandServe Command = iota
	CommandMigrate
	CommandRotateKeys
)

var (
	// failWebConstruction and openDatabases let tests check cleanup on a
	// failed Build without a real construction failure.
	failWebConstruction error
	openDatabases       atomic.Int32
)

// Build constructs the process in dependency order: configuration, database,
// migrations, database-backed startup checks, enforcer, file stores, keyring,
// the web application, then process kinds. If a step fails, every resource
// already acquired is closed in reverse order.
func Build(ctx context.Context, opts Options) (*Application, error) {
	logger := opts.Logger
	if logger == nil {
		logger = discardLogger()
	}
	cfg := opts.Config
	if err := config.Normalize(&cfg); err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	if err := ensureSQLiteParentDir(cfg); err != nil {
		return nil, err
	}
	logConfiguration(logger, cfg)

	application := &Application{
		logger:           logger,
		resources:        &resourceStack{logger: logger},
		shutdownDeadline: cfg.ShutdownTimeout,
	}
	fail := func(err error) (*Application, error) {
		application.resources.closeAll(context.WithoutCancel(ctx))
		return nil, err
	}

	logger.Info("initializing database", slog.Group("database", "driver", cfg.DB.Driver, "automigrate", cfg.DB.Automigrate))
	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, logger)
	if err != nil {
		return nil, err
	}
	openDatabases.Add(1)
	application.resources.push("database", func(context.Context) error {
		db.Close()
		openDatabases.Add(-1)
		return nil
	})

	if opts.Command == CommandMigrate || cfg.DB.Automigrate {
		logger.Info("running database migrations", "timeout_ms", cfg.DB.MigrationTimeout.Milliseconds())
		migrateCtx, cancel := context.WithTimeout(ctx, cfg.DB.MigrationTimeout)
		err := db.MigrateLocked(migrateCtx, func(context.Context) error { return db.MigrateUp() })
		cancel()
		if err != nil {
			return fail(err)
		}
		logger.Info("database migrations complete")
	}
	if opts.Command == CommandMigrate {
		return application, nil
	}

	if err := web.InitializeInstanceBaseURL(ctx, db, cfg.BootstrapBaseURL); err != nil {
		return fail(err)
	}
	if err := web.ValidateRuntimeSettingsInvariant(ctx, db); err != nil {
		return fail(err)
	}
	enforcer, err := access.New(db.DB)
	if err != nil {
		return fail(fmt.Errorf("enforcer init: %w", err))
	}
	stores, err := NewFileStores(cfg)
	if err != nil {
		return fail(fmt.Errorf("file storage init: %w", err))
	}
	if err := validateReferencedFileStores(ctx, db, stores); err != nil {
		return fail(err)
	}
	logger.Info("file storage initialized", slog.Group("files", "storage_mode", cfg.Files.StorageMode, "active_backend", stores.ActiveBackendID()))
	keyring, err := encrypt.NewKeyring(cfg.Encryption.Key, cfg.Encryption.PreviousKeys...)
	if err != nil {
		return fail(fmt.Errorf("encryption keyring init: %w", err))
	}

	sessions := connection.New(sessionIdleTimeout)
	application.resources.push("sessions", func(context.Context) error { sessions.Close(); return nil })
	cursors := connection.NewQueryCursorManager(sessionIdleTimeout)
	application.resources.push("cursors", func(context.Context) error { cursors.Close(); return nil })

	if failWebConstruction != nil {
		return fail(failWebConstruction)
	}
	webApp, err := web.NewApplication(web.Dependencies{
		Config:     cfg,
		DB:         db,
		Logger:     logger,
		Keyring:    keyring,
		Enforcer:   enforcer,
		FileStores: stores,
		Sessions:   sessions,
		Cursors:    cursors,
	})
	if err != nil {
		return fail(err)
	}
	application.web = webApp
	application.resources.push("web", func(context.Context) error { webApp.Close(); return nil })

	if opts.Command != CommandServe {
		return application, nil
	}

	health := NewHealth()
	serveAPI, runJobs := selectKinds(cfg)
	var api http.Handler
	if serveAPI {
		api = webApp.Handler()
	}
	handler := httpHandler(api, health)
	application.handler = handler
	tls := web.TLSFiles{Enabled: cfg.TLS.Enabled, CertFile: cfg.TLS.CertFile, KeyFile: cfg.TLS.KeyFile}
	// Kinds close in reverse, so HTTP drains in-flight requests before the
	// runtime and its job runner stop.
	application.kinds = append(application.kinds,
		newRuntimeKind(webApp, runJobs),
		newHTTPKind(web.NewServer(cfg.HTTPPort, handler, logger), tls, logger),
	)
	health.bind(application)
	logger.Info("process kinds built",
		"process_kinds", application.ProcessKindNames(),
		"serve_api", serveAPI,
		"run_jobs", runJobs,
	)
	return application, nil
}

// RotateEncryptionKeys re-encrypts application-encrypted data with the
// primary key. It is available only on an application built for
// CommandRotateKeys or CommandServe.
func (a *Application) RotateEncryptionKeys(ctx context.Context) (web.EncryptionRotationReport, error) {
	if a.web == nil {
		return web.EncryptionRotationReport{}, fmt.Errorf("application was built without the web application")
	}
	return a.web.RotateEncryptionKeys(ctx)
}

func (a *Application) httpHandler() http.Handler { return a.handler }

func logConfiguration(logger *slog.Logger, cfg config.Config) {
	logger.Info("application configuration loaded",
		slog.Group("config",
			"log_format", cfg.Log.Format,
			"bootstrap_base_url_configured", strings.TrimSpace(cfg.BootstrapBaseURL) != "",
			"tls_enabled", cfg.TLS.Enabled,
			"process_kinds", cfg.ProcessKinds,
		),
		slog.Group("database", "driver", cfg.DB.Driver, "automigrate", cfg.DB.Automigrate),
		slog.Group("files", "storage_mode", cfg.Files.StorageMode, "active_backend", cfg.Files.ActiveStorageBackend),
	)
}

// ensureSQLiteParentDir creates the directory that holds a file-backed SQLite
// database, so a first run on an empty volume can open it.
func ensureSQLiteParentDir(cfg config.Config) error {
	if cfg.DB.Driver != "sqlite" || cfg.DB.DSN == ":memory:" || strings.HasPrefix(cfg.DB.DSN, "file:") {
		return nil
	}
	dir := filepath.Dir(cfg.DB.DSN)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create sqlite database directory: %w", err)
	}
	return nil
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }
