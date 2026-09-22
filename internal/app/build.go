package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/cache"
	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/schema"
)

const (
	schemaCacheTTL          = 10 * time.Minute
	schemaCacheCapacity     = 256
	sessionIdleTimeout      = 30 * time.Minute
	queryCursorIdleTimeout  = 30 * time.Minute
	defaultShutdownDeadline = 30 * time.Second
)

// Options configures a [Build]. Only Config is required; the zero value of
// every other field yields a services-only application with no process kinds.
type Options struct {
	// Config is validated again during Build so programmatically assembled
	// configuration cannot bypass the rules config.Load enforces.
	Config config.Config
	// Logger receives lifecycle events. A nil logger discards them.
	Logger *slog.Logger
	// Edition supplies capability decorators and modules. A nil value selects
	// the Community edition.
	Edition edition.Edition

	// Prepare runs after the application database is open and migrated but
	// before any service is constructed. It is the seam for startup seeding and
	// invariants owned by a domain package rather than by the composition root.
	// Production wiring passes web.PrepareInstanceSettings here.
	Prepare []func(ctx context.Context, db *database.DB) error

	// ProcessKinds builds the process kinds for this process from the finished
	// service graph. It must not start background work; Build calls it last and
	// closes every acquired resource if it fails.
	ProcessKinds func(*Services) ([]ProcessKind, error)

	// ShutdownDeadline bounds Close. It defaults to 30 seconds.
	ShutdownDeadline time.Duration
}

// Build constructs the process dependency graph in dependency order and returns
// an application that has not started any background work yet.
//
// The construction order is: bootstrap validation, application database,
// migrations, database-backed startup invariants, encryption and authorization,
// file storage, live-session and query runtime, then process kinds. If any step
// fails, everything already acquired is closed in reverse order before the
// error is returned.
func Build(ctx context.Context, opts Options) (*Application, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	cfg := opts.Config
	if err := config.Normalize(&cfg); err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	selectedEdition := opts.Edition
	if selectedEdition == nil {
		selectedEdition = edition.NewCommunity()
	}
	if err := edition.Validate(selectedEdition, cfg); err != nil {
		return nil, err
	}
	if err := ensureSQLiteParentDir(cfg); err != nil {
		return nil, err
	}

	logger.Info("application configuration loaded",
		slog.Group("config",
			"log_format", cfg.Log.Format,
			"process_kinds", strings.Join(cfg.ProcessKinds, ","),
			"bootstrap_base_url_configured", strings.TrimSpace(cfg.BootstrapBaseURL) != "",
			"tls_enabled", cfg.TLS.Enabled,
		),
		slog.Group("database",
			"driver", cfg.DB.Driver,
			"automigrate", cfg.DB.Automigrate,
		),
		slog.Group("files",
			"storage_mode", cfg.Files.StorageMode,
			"active_backend", cfg.Files.ActiveStorageBackend,
		),
	)

	acquired := &resourceStack{logger: logger}
	fail := func(err error) (*Application, error) {
		acquired.closeAll(context.WithoutCancel(ctx))
		return nil, err
	}

	logger.Info("initializing database", slog.Group("database", "driver", cfg.DB.Driver, "automigrate", cfg.DB.Automigrate))
	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, logger)
	if err != nil {
		return nil, err
	}
	acquired.push("application database", func(context.Context) error {
		db.Close()
		return nil
	})

	if cfg.DB.Automigrate {
		logger.Info("running database migrations")
		if err := db.MigrateUp(); err != nil {
			return fail(err)
		}
		if err := edition.Migrate(ctx, selectedEdition, db); err != nil {
			return fail(err)
		}
		logger.Info("database migrations complete")
	}
	for _, prepare := range opts.Prepare {
		if err := prepare(ctx, db); err != nil {
			return fail(err)
		}
	}

	enforcer, err := access.New(db.DB)
	if err != nil {
		return fail(fmt.Errorf("enforcer init: %w", err))
	}

	fileStores, err := NewFileStores(cfg)
	if err != nil {
		return fail(fmt.Errorf("file storage init: %w", err))
	}
	if err := validateReferencedFileStores(ctx, db, fileStores); err != nil {
		return fail(err)
	}
	logger.Info("file storage initialized", slog.Group("files", "storage_mode", cfg.Files.StorageMode, "active_backend", fileStores.ActiveBackendID()))

	keyring, err := encrypt.NewKeyring(cfg.Encryption.Key, cfg.Encryption.PreviousKeys...)
	if err != nil {
		return fail(fmt.Errorf("encryption keyring init: %w", err))
	}

	connManager := connection.NewUnstarted(sessionIdleTimeout)
	acquired.push("database connection sessions", func(context.Context) error {
		connManager.Close()
		return nil
	})
	queryCursors := connection.NewUnstartedQueryCursorManager(queryCursorIdleTimeout)
	acquired.push("query cursors", func(context.Context) error {
		queryCursors.Close()
		return nil
	})

	schemaService := schema.NewServiceWithLogger(cache.NewMemCache(schemaCacheCapacity), schemaCacheTTL, logger)
	completionService := completion.NewService()
	WireCacheInvalidation(connManager, schemaService, completionService)

	services := &Services{
		Config:            cfg,
		Logger:            logger,
		DB:                db,
		Enforcer:          enforcer,
		PolicyEvaluator:   selectedEdition.PolicyEvaluator(enforcer),
		Keyring:           keyring,
		ConnManager:       connManager,
		QueryCursors:      queryCursors,
		SchemaService:     schemaService,
		SchemaSnapshots:   schema.NewSnapshotStore(db),
		CompletionService: completionService,
		FileStores:        fileStores,
		JobStore:          jobs.NewStore(db),
		Edition:           selectedEdition,
	}

	var kinds []ProcessKind
	if opts.ProcessKinds != nil {
		kinds, err = opts.ProcessKinds(services)
		if err != nil {
			return fail(err)
		}
	}
	if err := validateProcessKindSelection(cfg, kinds); err != nil {
		return fail(err)
	}

	shutdownDeadline := opts.ShutdownDeadline
	if shutdownDeadline <= 0 {
		shutdownDeadline = defaultShutdownDeadline
	}

	return &Application{
		Services:         services,
		ProcessKinds:     kinds,
		logger:           logger,
		resources:        acquired,
		shutdownDeadline: shutdownDeadline,
	}, nil
}

// WireCacheInvalidation drops cached schema and completion data for a
// connection once its last live session closes, so a reconnect cannot serve
// metadata captured before the target database changed underneath it.
func WireCacheInvalidation(connManager *connection.Manager, schemaService *schema.Service, completionService *completion.Service) {
	connManager.SetOnConnectionEmpty(func(connectionID string) {
		if schemaService != nil {
			schemaService.RefreshConnection(connectionID)
		}
		if completionService != nil {
			completionService.InvalidateConnection(connectionID)
		}
	})
}

// validateProcessKindSelection checks that the built process kinds match the
// selection in configuration, so a wiring mistake fails at boot instead of
// silently running fewer responsibilities than the operator asked for.
func validateProcessKindSelection(cfg config.Config, kinds []ProcessKind) error {
	if len(kinds) == 0 {
		return nil
	}
	built := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		name := kind.Name()
		if _, exists := built[name]; exists {
			return fmt.Errorf("process kind %q was built more than once", name)
		}
		built[name] = struct{}{}
	}
	for _, configured := range cfg.ProcessKinds {
		if _, ok := built[configured]; !ok {
			return fmt.Errorf("process kind %q is configured but was not built", configured)
		}
	}
	for name := range built {
		if !cfg.HasProcessKind(name) {
			return fmt.Errorf("process kind %q was built but is not configured", name)
		}
	}
	return nil
}

// ensureSQLiteParentDir creates the directory holding a file-backed SQLite
// database so a first run against a fresh volume does not fail to open it.
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
