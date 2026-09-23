package app

import (
	"context"
	"log/slog"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/settings"
)

// Services is the constructed dependency graph shared by every process kind.
// Required fields are non-nil once [Build] returns, and none of them own
// background goroutines that [Application.Close] does not stop.
//
// Services holds infrastructure and cross-cutting capabilities only. Domain
// behavior that needs request context, such as the job handler registry, is
// built by the process kind that runs it.
type Services struct {
	// Config is the validated bootstrap configuration this graph was built
	// from. It is read-only; runtime settings live in the database.
	Config config.Config
	Logger *slog.Logger

	// Health answers process liveness and readiness probes for whichever
	// process kinds this process runs.
	Health *Health

	// DB is the SQLWarden metadata database, not a target database.
	DB       *database.DB
	Enforcer *access.Enforcer
	// PolicyEvaluator is the edition-decorated authorization decision path.
	// Enforcer remains available for core role and policy administration.
	PolicyEvaluator access.PolicyEvaluator

	// Access is the application service for role and policy administration and
	// for effective-permission reads. Transports map its errors to their own
	// protocol; they do not reimplement its rules.
	Access *access.Service
	// Catalog is the application service for the resource catalog:
	// organizations, workspaces, environments, and connections. It owns the
	// resource invariants — hierarchy and policy seeding, workspace ownership
	// of environments and connections, ancestry cache invalidation, credential
	// sealing, and connection lifecycle rules.
	Catalog *catalog.Service
	// Identity is the application service for registration, authentication,
	// profile, and credential changes. Session and token issuance remain a
	// transport concern.
	Identity *identity.Service

	// Audit is the edition-composed durable audit writer. Application services
	// emit audit intent through it; transports do not write audit records of
	// their own.
	Audit audit.Writer

	Keyring *encrypt.Keyring

	// ConnManager owns live target-database sessions and QueryCursors owns the
	// cursors opened on them. Cursors reference sessions, so cursors close
	// first during shutdown.
	ConnManager  *connection.Manager
	QueryCursors *connection.QueryCursorManager
	// Execution is the process-independent target-database runtime. HTTP and
	// future RPC adapters use this port rather than concrete session managers.
	Execution execution.SessionRuntime
	// LocalExecution owns target sessions in all-in-one and connector processes.
	// It is nil in an API-only process, where Execution is a WorkerRuntime and
	// no credential provider is constructed.
	LocalExecution execution.SessionRuntime
	// ExecutionServer and ConnectorServerCredentials are non-nil only when the
	// connector process kind is explicitly selected. All-in-one mode creates no
	// internal RPC transport.
	ExecutionServer            *execution.RuntimeServer
	ConnectorServerCredentials execution.ServerTransportCredentials
	// SessionDirectory owns opaque-handle routing leases.
	SessionDirectory execution.SessionDirectory

	SchemaService     *schema.Service
	SchemaSnapshots   *schema.SnapshotStore
	CompletionService *completion.Service

	// FileStores resolves workspace-file content backends by backend ID.
	FileStores *FileStores

	// JobStore is the durable job queue. Job handlers are registered by the
	// process kind that runs the worker.
	JobStore *jobs.Store

	// Settings reads validated instance settings and resolves the effective
	// settings for an organization or workspace. Config holds bootstrap
	// configuration only; operational settings are read through this service.
	Settings *settings.Service

	// Edition exposes capability decorators and modules selected by the
	// composition root. Services consume only its narrow contracts.
	Edition edition.Edition
}

// ProcessKind is one runtime responsibility of a process: an HTTP transport, a
// job worker pool, a realtime backplane. Implementations receive already-built
// [Services] and must not construct infrastructure of their own.
//
// Start must not block. A process kind that owns a listener also implements
// [Serving] so the composition root can observe it stopping on its own.
type ProcessKind interface {
	// Name is the stable process-kind identifier, one of the config.ProcessKind
	// constants.
	Name() string
	// Start begins background work. The context governs startup only, not the
	// lifetime of the work started.
	Start(ctx context.Context) error
	// Ready reports whether the process kind can perform its critical work. It
	// returns an error describing what is missing when it cannot.
	Ready(ctx context.Context) error
	// Close stops background work and drains in-flight work within the
	// context deadline.
	Close(ctx context.Context) error
}

// Serving is implemented by process kinds that run until they fail or are
// closed, such as an HTTP listener. The composition root treats a value sent on
// the returned channel as a reason to shut the whole process down.
type Serving interface {
	Done() <-chan error
}
