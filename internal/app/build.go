package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/cache"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/settings"
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

	// Prepare runs after the application database is open, migrated, and
	// seeded with valid runtime settings, but before any service is
	// constructed. It is the seam for additional startup seeding and invariants
	// owned by a domain package rather than by the composition root.
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
// migrations, runtime settings seeding, database-backed startup invariants,
// encryption and authorization,
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
	if err := settings.Prepare(ctx, db, cfg.BootstrapBaseURL); err != nil {
		return fail(err)
	}
	settingsService := settings.New(db)

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
	var sessionDirectory execution.SessionDirectory = execution.NewMemorySessionDirectory()
	var localExecution execution.SessionRuntime
	var executionRuntime execution.SessionRuntime
	var executionServer *execution.RuntimeServer
	var serverCredentials execution.ServerTransportCredentials
	apiSelected := explicitlySelectsProcessKind(cfg, config.ProcessKindAPI)
	connectorSelected := explicitlySelectsProcessKind(cfg, config.ProcessKindConnector)
	targetPolicy := catalog.NewTargetPolicy(settingsService)
	if !apiSelected || connectorSelected {
		credentialProvider := credentials.NewEncryptedColumnProvider(db, keyring)
		localExecution = execution.NewLocalRuntime(connManager, queryCursors, sessionDirectory, credentialProvider, targetPolicy, sessionIdleTimeout, execution.WithLogger(logger))
		executionRuntime = localExecution
	}
	if apiSelected || connectorSelected {
		grantAuthority, grantErr := execution.NewGrantAuthority([]byte(cfg.Connector.GrantSigningKey), "sqlwarden-api", "sqlwarden-connector", 0)
		if grantErr != nil {
			return fail(fmt.Errorf("execution grants: %w", grantErr))
		}
		clientCredentials, configuredServerCredentials, credentialsErr := connectorTransportCredentials(cfg)
		if credentialsErr != nil {
			return fail(credentialsErr)
		}
		if connectorSelected {
			serverCredentials = configuredServerCredentials
			executionServer, err = execution.NewRuntimeServer(localExecution, grantAuthority, execution.WithLogger(logger))
			if err != nil {
				return fail(fmt.Errorf("execution server: %w", err))
			}
		}
		if apiSelected && !connectorSelected {
			staticDirectory := execution.NewStaticSessionDirectory(cfg.Connector.Address)
			workerRuntime, workerErr := execution.NewWorkerRuntime(staticDirectory, grantAuthority, clientCredentials, execution.WithLogger(logger))
			if workerErr != nil {
				return fail(fmt.Errorf("worker execution runtime: %w", workerErr))
			}
			executionRuntime = workerRuntime
			sessionDirectory = staticDirectory
		}
	}

	schemaService := schema.NewServiceWithLogger(cache.NewMemCache(schemaCacheCapacity), schemaCacheTTL, logger)
	completionService := completion.NewService()
	WireCacheInvalidation(connManager, schemaService, completionService)

	auditStore := audit.NewSQLStore(db.DB)
	editionDeps := edition.Dependencies{
		DB:          db,
		Grants:      enforcer,
		AuditEvents: auditStore,
		Now:         time.Now,
		Logger:      logger,
	}.Normalize()
	identityProvider := selectedEdition.IdentityProvider(identity.NewCoreProvider(identity.NewDatabaseStore(db)), editionDeps)
	policyEvaluator := selectedEdition.PolicyEvaluator(enforcer, editionDeps)
	auditWriter := selectedEdition.AuditWriter(audit.NewCoreWriter(auditStore, editionDeps.Now), editionDeps)

	services := &Services{
		Config:          cfg,
		Health:          NewHealth(),
		Logger:          logger,
		DB:              db,
		Enforcer:        enforcer,
		PolicyEvaluator: policyEvaluator,
		Catalog: catalog.NewService(
			catalog.NewDatabaseStore(db, enforcer),
			enforcer,
			executionRuntime,
			keyring,
			targetPolicy,
			auditWriter,
		),
		Access:                     access.NewService(access.NewSQLStore(db.DB), enforcer, policyEvaluator, auditWriter),
		Identity:                   identity.NewService(identity.NewDatabaseStore(db), identityProvider, auditWriter),
		Audit:                      auditWriter,
		Keyring:                    keyring,
		ConnManager:                connManager,
		QueryCursors:               queryCursors,
		Execution:                  executionRuntime,
		LocalExecution:             localExecution,
		ExecutionServer:            executionServer,
		ConnectorServerCredentials: serverCredentials,
		SessionDirectory:           sessionDirectory,
		SchemaService:              schemaService,
		SchemaSnapshots:            schema.NewSnapshotStore(db),
		CompletionService:          completionService,
		FileStores:                 fileStores,
		JobStore:                   jobs.NewStore(db),
		Settings:                   settingsService,
		Edition:                    selectedEdition,
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
		shutdownDeadline = cfg.ShutdownTimeout
	}
	if shutdownDeadline <= 0 {
		shutdownDeadline = defaultShutdownDeadline
	}

	application := &Application{
		Services:         services,
		ProcessKinds:     kinds,
		logger:           logger,
		resources:        acquired,
		shutdownDeadline: shutdownDeadline,
	}
	services.Health.bind(application)
	return application, nil
}

func connectorTransportCredentials(cfg config.Config) (execution.ClientTransportCredentials, execution.ServerTransportCredentials, error) {
	if cfg.Connector.Transport == config.ConnectorTransportInsecure {
		credentials := execution.InsecureTransportCredentials{}
		return credentials, credentials, nil
	}

	clientConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Connector.TLS.ServerName}
	if cfg.Connector.TLS.CAFile != "" {
		contents, readErr := os.ReadFile(cfg.Connector.TLS.CAFile)
		if readErr != nil {
			return nil, nil, fmt.Errorf("read connector TLS CA file: %w", readErr)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(contents) {
			return nil, nil, fmt.Errorf("connector.tls.ca_file contains no certificates")
		}
		clientConfig.RootCAs = roots
	}
	var serverCredentials execution.ServerTransportCredentials = execution.InsecureTransportCredentials{}
	if explicitlySelectsProcessKind(cfg, config.ProcessKindConnector) {
		certificate, loadErr := tls.LoadX509KeyPair(cfg.Connector.TLS.CertFile, cfg.Connector.TLS.KeyFile)
		if loadErr != nil {
			return nil, nil, fmt.Errorf("load connector TLS certificate: %w", loadErr)
		}
		serverConfig := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
		serverCredentials = execution.TLSServerTransportCredentials{Config: serverConfig}
	}
	return execution.TLSClientTransportCredentials{Config: clientConfig}, serverCredentials, nil
}

func explicitlySelectsProcessKind(cfg config.Config, kind string) bool {
	for _, selected := range cfg.ProcessKinds {
		if selected == kind {
			return true
		}
	}
	return false
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
