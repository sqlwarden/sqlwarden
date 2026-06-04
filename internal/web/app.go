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
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/filestore"
	"github.com/sqlwarden/internal/smtp"
)

type App = application

type application struct {
	config           Config
	db               *database.DB
	logger           *slog.Logger
	mailer           *smtp.Mailer
	wg               sync.WaitGroup
	connManager      *connection.Manager
	encKey           []byte
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

	db, err := database.New(cfg.DB.Driver, cfg.DB.DSN, logger, cfg.DB.LogQueries)
	if err != nil {
		return nil, err
	}

	if cfg.DB.Automigrate {
		if err := db.MigrateUp(); err != nil {
			db.Close()
			return nil, err
		}
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

	app := &application{
		config:      cfg,
		db:          db,
		logger:      logger,
		mailer:      mailer,
		connManager: connection.New(30 * time.Minute),
		encKey:      encrypt.DeriveKey(cfg.Encryption.Key),
		enforcer:    enforcer,
		fileStores:  fileStores,
	}
	app.startFileContentDeletionReaper()
	return app, nil
}

func (app *application) Handler() http.Handler {
	return app.routes()
}

func (app *application) Close() error {
	if app.fileReaperCancel != nil {
		app.fileReaperCancel()
	}
	app.wg.Wait()

	if app.connManager != nil {
		app.connManager.Close()
	}

	if app.db != nil {
		app.db.Close()
	}

	return nil
}

func (app *application) startFileContentDeletionReaper() {
	ctx, cancel := context.WithCancel(context.Background())
	app.fileReaperCancel = cancel
	service := app.workspaceFileService()
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		service.RunContentDeletionReaper(ctx, time.Minute, 100, time.Minute)
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
