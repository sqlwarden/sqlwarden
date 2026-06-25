package web

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/dbengine/schema"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/filestore"
	"github.com/sqlwarden/internal/smtp"
)

const (
	schemaCacheTTL      = 10 * time.Minute
	schemaCacheCapacity = 256
)

type App = application

type application struct {
	config           Config
	db               *database.DB
	logger           *slog.Logger
	mailer           *smtp.Mailer
	wg               sync.WaitGroup
	connManager      *connection.Manager
	queryCursors     *queryCursorManager
	schemaService    *schema.Service
	keyring          *encrypt.Keyring
	enforcer         *access.Enforcer
	fileStores       *fileStoreRegistry
	fileLocks        sync.Map
	fileReaperCancel context.CancelFunc
}

type fileStoreRegistry struct {
	activeBackendID string
	stores          map[string]filestore.Store
}

func (r *fileStoreRegistry) ActiveBackendID() string {
	return r.activeBackendID
}

func (r *fileStoreRegistry) Store(_ context.Context, backendID string) (filestore.Store, error) {
	if backendID == "" {
		backendID = database.DefaultFileStorageBackendID
	}
	store, ok := r.stores[backendID]
	if !ok {
		return nil, files.ErrStorageBackendUnavailable
	}
	return store, nil
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if err := normalizeConfigPaths(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if err := ensureSQLiteParentDir(cfg); err != nil {
		return nil, err
	}

	logger.Info("application configuration loaded",
		slog.Group("config",
			"log_level", cfg.Log.Level,
			"log_format", cfg.Log.Format,
			"base_url_configured", strings.TrimSpace(cfg.BaseURL) != "",
			"personal_spaces_enabled", cfg.PersonalSpacesEnabled,
			"sessions_revocation_enabled", cfg.Sessions.RevocationEnabled,
			"tls_enabled", cfg.TLS.Enabled,
		),
		slog.Group("database",
			"driver", cfg.DB.Driver,
			"automigrate", cfg.DB.Automigrate,
			"log_queries", cfg.DB.LogQueries,
		),
		slog.Group("files",
			"storage_mode", cfg.Files.StorageMode,
			"active_backend", cfg.Files.ActiveStorageBackend,
			"revisions_enabled", cfg.Files.Revisions.Enabled,
		),
		slog.Group("drivers",
			"sqlite_allowed_sources", cfg.Drivers.SQLite.AllowedSources,
		),
	)

	logger.Info("initializing database", slog.Group("database", "driver", cfg.DB.Driver, "automigrate", cfg.DB.Automigrate))
	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, logger, cfg.DB.LogQueries)
	if err != nil {
		return nil, err
	}

	if cfg.DB.Automigrate {
		logger.Info("running database migrations")
		if err := db.MigrateUp(); err != nil {
			db.Close()
			return nil, err
		}
		logger.Info("database migrations complete")
	}

	mailer, err := smtp.NewMailer(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From)
	if err != nil {
		db.Close()
		return nil, err
	}

	enforcer, err := access.New(db.DB)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("enforcer init: %w", err)
	}

	fileStores, err := newFileStoreRegistry(cfg)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("file storage init: %w", err)
	}
	if err := validateConfiguredFileStorageBackends(context.Background(), db, fileStores); err != nil {
		db.Close()
		return nil, err
	}
	logger.Info("file storage initialized", slog.Group("files", "storage_mode", cfg.Files.StorageMode, "active_backend", fileStores.ActiveBackendID()))

	keyring, err := encrypt.NewKeyring(cfg.Encryption.Key, cfg.Encryption.PreviousKeys...)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("encryption keyring init: %w", err)
	}

	app := &application{
		config:        cfg,
		db:            db,
		logger:        logger,
		mailer:        mailer,
		connManager:   connection.New(30 * time.Minute),
		queryCursors:  newQueryCursorManager(30 * time.Minute),
		schemaService: schema.NewServiceWithLogger(schema.NewMemCache(schemaCacheCapacity), schemaCacheTTL, logger),
		keyring:       keyring,
		enforcer:      enforcer,
		fileStores:    fileStores,
	}
	app.startFileContentDeletionReaper()
	return app, nil
}

func (app *application) Handler() http.Handler {
	return app.routes()
}

func (app *application) Close() error {
	startedAt := time.Now()
	app.logger.Info("stopping application")
	if app.fileReaperCancel != nil {
		app.fileReaperCancel()
	}
	app.wg.Wait()
	app.logger.Info("background workers stopped", "duration_ms", time.Since(startedAt).Milliseconds())

	if app.queryCursors != nil {
		app.queryCursors.Close()
	}
	if app.connManager != nil {
		connCloseStartedAt := time.Now()
		app.connManager.Close()
		app.logger.Info("database connection sessions closed", "duration_ms", time.Since(connCloseStartedAt).Milliseconds())
	}

	if app.db != nil {
		dbCloseStartedAt := time.Now()
		app.db.Close()
		app.logger.Info("application database closed", "duration_ms", time.Since(dbCloseStartedAt).Milliseconds())
	}

	app.logger.Info("application stopped", "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func (app *application) startFileContentDeletionReaper() {
	ctx, cancel := context.WithCancel(context.Background())
	app.fileReaperCancel = cancel
	service := app.workspaceFileService()
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		app.logger.Info("file content deletion reaper started")
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			processed, err := service.ReapContentDeletionsOnce(ctx, 100, time.Minute)
			if err != nil {
				app.logger.ErrorContext(ctx, "file content deletion reaper failed", "error", err)
			}
			if processed > 0 {
				app.logger.InfoContext(ctx, "file content deletion reaper processed batch", "processed", processed)
			}
			select {
			case <-ctx.Done():
				app.logger.Info("file content deletion reaper stopped")
				return
			case <-ticker.C:
			}
		}
	}()
}

func ensureSQLiteParentDir(cfg Config) error {
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

func newFileStoreRegistry(cfg Config) (*fileStoreRegistry, error) {
	activeBackendID := cfg.Files.ActiveStorageBackend
	if cfg.Files.StorageMode == FilesStorageModeFile || strings.TrimSpace(activeBackendID) == "" {
		activeBackendID = database.DefaultFileStorageBackendID
	}
	registry := &fileStoreRegistry{
		activeBackendID: activeBackendID,
		stores:          make(map[string]filestore.Store, len(cfg.Files.StorageBackends)),
	}
	for id, backend := range cfg.Files.StorageBackends {
		switch backend.Type {
		case FilesStorageBackendFilesystem:
			store, err := filestore.NewFilesystem(backend.RootDir)
			if err != nil {
				return nil, fmt.Errorf("backend %q: %w", id, err)
			}
			registry.stores[id] = store
		default:
			return nil, fmt.Errorf("backend %q type %q is not implemented", id, backend.Type)
		}
	}
	return registry, nil
}

// validateConfiguredFileStorageBackends fails startup when saved file content
// references a backend that is not configured for this process.
func validateConfiguredFileStorageBackends(ctx context.Context, db *database.DB, stores *fileStoreRegistry) error {
	referenced, err := db.ListWorkspaceFileStorageBackendIDs(ctx)
	if err != nil {
		return err
	}
	for _, backendID := range referenced {
		if _, ok := stores.stores[backendID]; !ok {
			return fmt.Errorf("file storage backend %q is referenced by saved file content but is not configured", backendID)
		}
	}
	return nil
}
