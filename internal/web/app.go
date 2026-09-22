package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sqlwarden/internal/access"
	coreapp "github.com/sqlwarden/internal/app"
	completionapp "github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/jobs"
	schemaapp "github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/smtp"
)

const (
	fileReaperInterval = 15 * time.Minute
	fileReaperRetry    = time.Minute
)

// App is the HTTP transport layer built on top of an already-constructed
// service graph. It owns no infrastructure: every dependency is borrowed from
// internal/app, which constructs and releases them.
type App = application

type application struct {
	config            config.Config
	db                *database.DB
	logger            *slog.Logger
	mailer            *smtp.Mailer
	mailerMu          sync.RWMutex
	wg                sync.WaitGroup
	connManager       *connection.Manager
	queryCursors      *connection.QueryCursorManager
	schemaService     *schemaapp.Service
	schemaSnapshots   *schemaapp.SnapshotStore
	completionService *completionapp.Service
	keyring           *encrypt.Keyring
	enforcer          *access.Enforcer
	policyEvaluator   access.PolicyEvaluator
	edition           edition.Edition
	fileStores        *coreapp.FileStores
	fileLocks         sync.Map
	fileReaperCancel  context.CancelFunc
	jobStore          *jobs.Store
	jobRegistry       *jobs.Registry
	runtimeCancel     context.CancelFunc
	runtimeUpdates    chan database.InstanceSettings
	runtimeSettings   *runtimeSettingsService
	accessLogsEnabled atomic.Bool
}

// NewApplication adapts a built service graph to the HTTP transport layer. It
// starts no background work; [ProcessKinds] wraps the result in the process
// kind that does.
func NewApplication(services *coreapp.Services) *App {
	return &application{
		config:            services.Config,
		db:                services.DB,
		logger:            services.Logger,
		mailer:            smtp.NewDisabledMailer(""),
		connManager:       services.ConnManager,
		queryCursors:      services.QueryCursors,
		schemaService:     services.SchemaService,
		schemaSnapshots:   services.SchemaSnapshots,
		completionService: services.CompletionService,
		keyring:           services.Keyring,
		enforcer:          services.Enforcer,
		policyEvaluator:   services.PolicyEvaluator,
		edition:           services.Edition,
		fileStores:        services.FileStores,
		jobStore:          services.JobStore,
		runtimeSettings:   newRuntimeSettingsService(services.DB),
		runtimeUpdates:    make(chan database.InstanceSettings, 1),
	}
}

// Handler returns the HTTP handler serving the API and the embedded SPA.
func (app *application) Handler() http.Handler {
	return app.routes()
}

func (app *application) defaultJobRegistry() *jobs.Registry {
	registry := jobs.NewRegistry()
	registry.Register(jobs.Definition{
		Type:        "noop",
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
			return map[string]any{"ok": true}, nil
		}),
	})
	registry.Register(jobs.Definition{
		Type:        jobs.TypeFileContentReap,
		MaxAttempts: 3,
		Backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Minute
		},
		Handler: jobs.HandlerFunc(func(ctx context.Context, _ jobs.Runtime) (any, error) {
			processed, err := app.workspaceFileService().ReapContentDeletionsOnce(ctx, 100, fileReaperRetry)
			if err != nil {
				return nil, jobs.Retryable("file_content_reap_failed", err.Error())
			}
			if processed > 0 {
				app.logger.InfoContext(ctx, "file content deletion job processed batch", "processed", processed)
			} else {
				app.logger.DebugContext(ctx, "file content deletion job found no work")
			}
			return map[string]any{"processed": processed}, nil
		}),
	})
	registry.Register(jobs.Definition{
		Type:        jobs.TypeExportQueryCSV,
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(ctx context.Context, runtime jobs.Runtime) (any, error) {
			return app.handleExportJob(ctx, runtime)
		}),
	})
	registry.Register(jobs.Definition{
		Type:        jobs.TypeSchemaSync,
		MaxAttempts: 3,
		Backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * time.Minute
		},
		Handler: jobs.HandlerFunc(func(ctx context.Context, runtime jobs.Runtime) (any, error) {
			return app.handleSchemaSyncJob(ctx, runtime)
		}),
	})
	return registry
}

func (app *application) startFileContentDeletionReaper() {
	ctx, cancel := context.WithCancel(context.Background())
	app.fileReaperCancel = cancel
	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		app.logger.Info("file content deletion reaper started")
		ticker := time.NewTicker(fileReaperInterval)
		defer ticker.Stop()
		for {
			if err := app.enqueueFileContentReapJob(ctx); err != nil {
				app.logger.ErrorContext(ctx, "file content deletion reaper job enqueue failed", "error", err)
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

func (app *application) enqueueFileContentReapJob(ctx context.Context) error {
	if app.jobStore == nil {
		app.jobStore = jobs.NewStore(app.db)
	}
	active, err := app.jobStore.HasActiveJobType(ctx, jobs.VisibilityInternal, jobs.TypeFileContentReap)
	if err != nil {
		return err
	}
	if active {
		app.logger.DebugContext(ctx, "file content deletion reaper job already active", "job.type", jobs.TypeFileContentReap)
		return nil
	}
	job, created, err := app.jobStore.EnqueueSingleton(ctx, jobs.EnqueueInput{
		Type:         jobs.TypeFileContentReap,
		SingletonKey: jobs.TypeFileContentReap,
		Visibility:   jobs.VisibilityInternal,
		Priority:     jobs.PriorityLow,
		MaxAttempts:  3,
	})
	if errors.Is(err, jobs.ErrActiveExists) {
		app.logger.DebugContext(ctx, "file content deletion reaper job already active", "job.type", jobs.TypeFileContentReap)
		return nil
	}
	if err == nil {
		if created {
			app.logger.InfoContext(ctx, "file content deletion reaper job queued", "job.id", job.ID, "job.type", job.Type, "job.priority", job.Priority)
		}
	}
	return err
}
