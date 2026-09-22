package execution

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/pkg/result"
)

// SessionHandle is an opaque capability identifying one live target-database
// session. It is unrelated to an HTTP authentication session.
type SessionHandle string

// CursorHandle is an opaque capability identifying one result cursor owned by
// a session.
type CursorHandle string

// Scope identifies the principal and catalog resources a session belongs to.
// IDs are strings at this boundary so the runtime remains independent of the
// metadata database's key representation.
type Scope struct {
	TenantID     string `json:"tenant_id"`
	AccountID    string `json:"account_id"`
	WorkspaceID  string `json:"workspace_id"`
	ConnectionID string `json:"connection_id"`
}

// Grant is the complete authorization envelope carried to an execution
// runtime. LocalRuntime trusts its in-process caller; remote runtimes validate
// the same shape without changing the Runtime contract.
type Grant struct {
	ID                   string    `json:"id"`
	Issuer               string    `json:"issuer"`
	Audience             string    `json:"audience"`
	Scope                Scope     `json:"scope"`
	Permissions          []string  `json:"permissions"`
	IssuedAt             time.Time `json:"issued_at"`
	NotBefore            time.Time `json:"not_before"`
	ExpiresAt            time.Time `json:"expires_at"`
	RevocationGeneration uint64    `json:"revocation_generation"`
	Signature            string    `json:"signature"`
}

// Limits are the resource ceilings fixed when an operation begins.
type Limits struct {
	MaxRows  int   `json:"max_rows"`
	MaxBytes int64 `json:"max_bytes"`
}

// SSHConfig is the wire-safe SSH tunnel configuration used to reach a target.
type SSHConfig struct {
	Host                string `json:"host"`
	Port                int    `json:"port"`
	User                string `json:"user"`
	AuthMethod          string `json:"auth_method"`
	Password            string `json:"password,omitempty"`
	PrivateKeyPEM       string `json:"private_key_pem,omitempty"`
	Passphrase          string `json:"passphrase,omitempty"`
	KnownHostsEntry     string `json:"known_hosts_entry,omitempty"`
	Fingerprint         string `json:"fingerprint,omitempty"`
	InsecureSkipHostKey bool   `json:"insecure_skip_host_key"`
}

// Target describes how a runtime opens the target database. Credential
// resolution moves behind its own provider in SQLW-170; keeping it in this
// request today preserves current behavior and gives WorkerRuntime a serializable
// request from its first implementation.
type Target struct {
	Driver       string             `json:"driver"`
	DSN          string             `json:"dsn"`
	DefaultScope metadata.ScopePath `json:"default_scope,omitempty"`
	TLS          *engine.TLSConfig  `json:"tls,omitempty"`
	SSH          *SSHConfig         `json:"ssh,omitempty"`
	Limits       Limits             `json:"limits"`
}

// OpenRequest describes a target session to open. Ephemeral sessions never
// reuse an interactive session and are intended for bounded background work.
type OpenRequest struct {
	Scope     Scope  `json:"scope"`
	Target    Target `json:"target"`
	Grant     Grant  `json:"grant"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
}

// OpenResult identifies the opened session and reports whether an existing
// pooled session was reused.
type OpenResult struct {
	Handle SessionHandle `json:"session_handle"`
	Reused bool          `json:"reused"`
}

// QueryRequest runs a row-producing statement, optionally backed by a cursor.
type QueryRequest struct {
	Handle    SessionHandle `json:"session_handle"`
	SQL       string        `json:"sql"`
	Args      []any         `json:"args,omitempty"`
	Limits    Limits        `json:"limits"`
	UseCursor bool          `json:"use_cursor"`
	PageSize  int           `json:"page_size,omitempty"`
	Explain   *explain.Plan `json:"explain,omitempty"`
	Grant     Grant         `json:"grant"`
}

// QueryResult contains the first result page and current transaction state.
type QueryResult struct {
	Result      *result.ResultSet `json:"result"`
	Cursor      CursorHandle      `json:"cursor_handle,omitempty"`
	Exhausted   bool              `json:"exhausted"`
	Transaction TransactionStatus `json:"transaction"`
}

// FetchRequest retrieves the next page from a session-owned cursor.
type FetchRequest struct {
	Handle   SessionHandle `json:"session_handle"`
	Cursor   CursorHandle  `json:"cursor_handle"`
	PageSize int           `json:"page_size"`
	Limits   Limits        `json:"limits"`
	Grant    Grant         `json:"grant"`
}

// FetchResult contains one cursor page and whether it was the final page.
type FetchResult struct {
	Result    *result.ResultSet `json:"result"`
	Exhausted bool              `json:"exhausted"`
}

// ExecuteRequest runs a non-cursor statement, explain plan, or structured DDL.
type ExecuteRequest struct {
	Handle  SessionHandle `json:"session_handle"`
	SQL     string        `json:"sql,omitempty"`
	Args    []any         `json:"args,omitempty"`
	Limits  Limits        `json:"limits"`
	Explain *explain.Plan `json:"explain,omitempty"`
	DDL     *ddl.Request  `json:"ddl,omitempty"`
	Grant   Grant         `json:"grant"`
}

// ExecuteResult contains any returned rows and current transaction state.
type ExecuteResult struct {
	Result      *result.ResultSet `json:"result,omitempty"`
	Transaction TransactionStatus `json:"transaction"`
}

// SessionRequest selects a session for an operation without additional input.
type SessionRequest struct {
	Handle SessionHandle `json:"session_handle"`
	Grant  Grant         `json:"grant"`
}

// CloseRequest closes either an entire session or one cursor within it.
type CloseRequest struct {
	Handle SessionHandle `json:"session_handle"`
	Cursor CursorHandle  `json:"cursor_handle,omitempty"`
	Grant  Grant         `json:"grant"`
}

// TransactionMode controls whether statements commit independently or join a
// manual transaction.
type TransactionMode string

const (
	TransactionModeAuto   TransactionMode = "auto"
	TransactionModeManual TransactionMode = "manual"
)

// TransactionStatus is a transport-safe snapshot of a session transaction.
type TransactionStatus struct {
	Mode              TransactionMode `json:"mode"`
	Open              bool            `json:"open"`
	PendingStatements int             `json:"pending_statements"`
	Statements        []string        `json:"statements"`
}

// SessionInfo is safe routing and ownership metadata; it never contains a
// driver, credential, SQL string, or result row.
type SessionInfo struct {
	Handle        SessionHandle `json:"session_handle"`
	Scope         Scope         `json:"scope"`
	TunnelHealthy *bool         `json:"tunnel_healthy,omitempty"`
}

// SchemaRequest selects one live metadata operation.
type SchemaRequest struct {
	Handle SessionHandle        `json:"session_handle"`
	Scope  metadata.ScopePath   `json:"scope,omitempty"`
	Refs   []metadata.ObjectRef `json:"refs,omitempty"`
	Ref    *metadata.ObjectRef  `json:"ref,omitempty"`
	Grant  Grant                `json:"grant"`
}

// SessionCapabilities describes optional operations implemented by the live
// session's concrete driver. Specs are returned by value so callers never need
// access to the driver itself.
type SessionCapabilities struct {
	Schema        *metadata.SchemaSpec `json:"schema,omitempty"`
	DDL           *ddl.Spec            `json:"ddl,omitempty"`
	Relationships bool                 `json:"relationships"`
	Definitions   bool                 `json:"definitions"`
}

// Runtime is the stable local/remote execution contract. Every operation is
// keyed by an opaque SessionHandle; request contexts only bound the individual
// operation and never become the lifetime of a session or cursor.
type Runtime interface {
	Open(context.Context, OpenRequest) (OpenResult, error)
	Query(context.Context, QueryRequest) (QueryResult, error)
	Fetch(context.Context, FetchRequest) (FetchResult, error)
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
	TransactionStatus(context.Context, SessionRequest) (TransactionStatus, error)
	Commit(context.Context, SessionRequest) (TransactionStatus, error)
	Rollback(context.Context, SessionRequest) (TransactionStatus, error)
	Cancel(context.Context, SessionRequest) error
	Close(context.Context, CloseRequest) error
}

// SessionRuntime contains the additional typed operations used by current
// application services. WorkerRuntime implements the same wire-safe surface.
type SessionRuntime interface {
	Runtime
	SetTransactionMode(context.Context, SessionRequest, TransactionMode) (TransactionStatus, error)
	Session(context.Context, SessionHandle) (SessionInfo, bool, error)
	Sessions(context.Context, Scope) ([]SessionInfo, error)
	CloseMatching(context.Context, Scope) (int, error)
	CountForConnection(string) int
	RemoveForConnection(string) int
	RemoveForOrgAccount(string, string) int
	RemoveForWorkspaceAccount(string, string) int
	Capabilities(context.Context, SessionRequest) (SessionCapabilities, error)
	SchemaDirectory(context.Context, SchemaRequest) (*metadata.Directory, error)
	SchemaObjects(context.Context, SchemaRequest) ([]metadata.Object, error)
	SchemaRelationships(context.Context, SchemaRequest) (*metadata.RelationshipGraph, error)
	SchemaDefinition(context.Context, SchemaRequest) (*metadata.Descriptor, error)
	Stream(context.Context, SessionRequest, io.Writer, exports.StreamOptions) (exports.StreamResult, error)
}

// FailureCode is stable across local errors and RPC status mapping.
type FailureCode string

const (
	FailureSessionLost             FailureCode = "session_lost"
	FailureCursorLost              FailureCode = "cursor_lost"
	FailureTransactionLost         FailureCode = "transaction_lost"
	FailureExecutionOutcomeUnknown FailureCode = "execution_outcome_unknown"
	FailureGrantInvalid            FailureCode = "grant_invalid"
	FailureLimitExceeded           FailureCode = "limit_exceeded"
)

// Failure carries a stable machine-readable runtime failure code while
// retaining the underlying error for errors.Is and errors.As checks.
type Failure struct {
	Code      FailureCode
	Retryable bool
	Err       error
}

func (e *Failure) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

// Unwrap exposes the underlying local or transport error.
func (e *Failure) Unwrap() error { return e.Err }

var (
	ErrSessionLost     = errors.New("execution session is no longer available")
	ErrCursorLost      = errors.New("execution cursor is no longer available")
	ErrTransactionLost = errors.New("execution transaction is no longer available")
	ErrOutcomeUnknown  = errors.New("execution outcome is unknown")
	ErrGrantInvalid    = errors.New("execution grant is invalid")
	ErrLimitExceeded   = errors.New("execution limit exceeded")

	ErrTransactionOpen          = errors.New("cannot switch to auto-commit while a transaction is open")
	ErrNoOpenTransaction        = transaction.ErrNoOpenTransaction
	ErrQueryCursorUnsupported   = errors.New("driver does not support query cursors")
	ErrSchemaUnsupported        = errors.New("driver does not support schema inspection")
	ErrRelationshipsUnsupported = errors.New("driver does not support schema relationships")
)

func scanOptions(limits Limits, maxRows int) cursor.ScanOptions {
	rows := limits.MaxRows
	if maxRows > 0 {
		rows = maxRows
	}
	return cursor.ScanOptions{MaxRows: rows, MaxBytes: limits.MaxBytes}
}
