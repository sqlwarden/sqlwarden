package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sqlwarden/internal/access"
	completionapp "github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/filestore"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/orgs"
	schemaapp "github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/smtp"
)

const (
	fileReaperInterval = 15 * time.Minute
	fileReaperRetry    = time.Minute
)

type App = application

// FileStores resolves the storage backend for workspace file content.
type FileStores interface {
	ActiveBackendID() string
	Store(ctx context.Context, backendID string) (filestore.Store, error)
}

// Dependencies are the already-constructed resources the web application
// uses. The caller owns their lifecycle: closing the App never closes them.
type Dependencies struct {
	Config     config.Config
	DB         *database.DB
	Logger     *slog.Logger
	Keyring    *encrypt.Keyring
	Enforcer   *access.Enforcer
	FileStores FileStores
	Sessions   *connection.Manager
	Cursors    *connection.QueryCursorManager

	Setup       identity.SetupStrategy
	Invitations orgs.InvitationPolicy
}

type application struct {
	config            config.Config
	db                *database.DB
	logger            *slog.Logger
	mailer            *smtp.Mailer
	mailerMu          sync.RWMutex
	wg                sync.WaitGroup
	connManager       *connection.Manager
	queryCursors      *connection.QueryCursorManager
	schemaNavigator   *schemaapp.Navigator
	completionService *completionapp.Service
	keyring           *encrypt.Keyring
	enforcer          *access.Enforcer
	fileStores        FileStores
	fileLocks         sync.Map
	fileReaperCancel  context.CancelFunc
	jobStore          *jobs.Store
	jobRegistry       *jobs.Registry
	runtimeCancel     context.CancelFunc
	runtimeStarted    atomic.Bool
	runtimeUpdates    chan database.InstanceSettings
	runtimeSettings   *runtimeSettingsService
	initialSettings   database.InstanceSettings
	accessLogsEnabled atomic.Bool
	setupStrategy     identity.SetupStrategy
	invitationPolicy  orgs.InvitationPolicy
}

// NewApplication wires the web application from its dependencies. It applies
// the stored instance settings but starts no background work; call
// StartRuntime for that.
func NewApplication(deps Dependencies) (*App, error) {
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if deps.Setup == nil || deps.Invitations == nil {
		return nil, errors.New("web: setup strategy and invitation policy are required")
	}
	app := &application{
		setupStrategy:     deps.Setup,
		invitationPolicy:  deps.Invitations,
		config:            deps.Config,
		db:                deps.DB,
		logger:            logger,
		mailer:            smtp.NewDisabledMailer(""),
		connManager:       deps.Sessions,
		queryCursors:      deps.Cursors,
		schemaNavigator:   schemaapp.NewNavigator(deps.DB, logger),
		completionService: completionapp.NewService(),
		keyring:           deps.Keyring,
		enforcer:          deps.Enforcer,
		fileStores:        deps.FileStores,
		jobStore:          jobs.NewStore(deps.DB),
		runtimeSettings:   newRuntimeSettingsService(deps.DB),
		runtimeUpdates:    make(chan database.InstanceSettings, 1),
	}
	initialSettings, err := app.instanceSettings(context.Background())
	if err != nil {
		return nil, err
	}
	if err := app.applyRuntimeOperations(initialSettings); err != nil {
		return nil, err
	}
	app.initialSettings = initialSettings
	app.configureConnectionCacheInvalidation()
	if _, err := app.backfillConnectionTLSConfig(context.Background()); err != nil {
		logger.Warn("connection tls backfill failed; will retry next boot", slog.Any("error", err))
	}
	app.jobRegistry = app.defaultJobRegistry()
	return app, nil
}

// StartRuntime starts the runtime supervisor. Every serving process runs it,
// because it applies instance settings changes made by any replica. With
// runJobs it also owns the job runner, and the file content deletion reaper
// starts beside it. Only the first call has an effect.
func (app *application) StartRuntime(runJobs bool) {
	if !app.runtimeStarted.CompareAndSwap(false, true) {
		app.logger.Warn("runtime already started; ignoring repeated start", "run_jobs", runJobs)
		return
	}
	app.logger.Info("runtime starting", "run_jobs", runJobs)
	app.startRuntimeSupervisor(app.initialSettings, runJobs)
	if runJobs {
		app.startFileContentDeletionReaper()
	}
}

// StopRuntime cancels the runtime goroutines and waits for them to return.
func (app *application) StopRuntime() {
	if app.fileReaperCancel != nil {
		app.fileReaperCancel()
	}
	if app.runtimeCancel != nil {
		app.runtimeCancel()
	}
	app.wg.Wait()
}

// Close stops the runtime. Sessions, cursors, and the database belong to the
// caller that created them.
func (app *application) Close() {
	startedAt := time.Now()
	app.StopRuntime()
	app.logger.Info("background workers stopped", "duration_ms", time.Since(startedAt).Milliseconds())
}

func (app *application) configureConnectionCacheInvalidation() {
	if app.connManager == nil {
		return
	}
	app.connManager.SetOnConnectionEmpty(func(connectionID string) {
		if app.schemaNavigator != nil {
			if id, err := strconv.ParseInt(connectionID, 10, 64); err == nil {
				app.schemaNavigator.ForgetConnection(id)
			}
		}
	})
}

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
