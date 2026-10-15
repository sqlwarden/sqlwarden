package execution

import (
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/pkg/result"
)

// SessionID is an opaque capability naming one live target-database session.
// It is unrelated to an HTTP authentication session.
type SessionID string

// CursorID is an opaque capability naming one result cursor owned by a session.
type CursorID string

// Scope identifies the principal and catalog resources a session belongs to.
// IDs are strings so the runtime stays independent of the metadata database's
// key representation.
type Scope struct {
	OrgID        string
	WorkspaceID  string
	AccountID    string
	ConnectionID string
}

// Valid reports whether every identifier is present.
func (s Scope) Valid() bool {
	return s.OrgID != "" && s.WorkspaceID != "" && s.AccountID != "" && s.ConnectionID != ""
}

// Limits are the resource ceilings fixed when an operation begins.
type Limits struct {
	MaxRows  int
	MaxBytes int64
}

type OpenRequest struct {
	Scope     Scope
	Ephemeral bool
	// Limits cap result size for statements run on the session; zero values
	// leave the driver defaults.
	Limits Limits
}

type SessionInfo struct {
	ID            SessionID
	Scope         Scope
	Driver        string
	TunnelHealthy *bool
	Reused        bool
}

// SessionFilter selects sessions. AccountID and WorkspaceID together match one
// account's sessions in that workspace; AccountID alone matches that account's
// sessions in every workspace; WorkspaceID alone matches every account's
// sessions in the workspace (callers must authorize that view). A filter with
// neither is invalid.
type SessionFilter struct {
	AccountID   string
	WorkspaceID string
}

// TxMode is "auto" or "manual".
type TxMode string

const (
	TxModeAuto   TxMode = "auto"
	TxModeManual TxMode = "manual"
)

type TxStatus struct {
	Mode              TxMode
	Open              bool
	PendingStatements int
	Statements        []string
}

type QueryRequest struct {
	SessionID SessionID
	SQL       string
	Args      []any
	Limits    Limits
	UseCursor bool
	// RequireCursor makes a UseCursor query fail with ErrCursorsUnsupported
	// instead of falling back to a buffered read.
	RequireCursor bool
	PageSize      int
	Explain       *explain.Plan
}

type FetchRequest struct {
	// SessionID may be empty: the cursor ID alone then addresses the cursor,
	// which must still belong to a session within the caller's scope.
	SessionID SessionID
	CursorID  CursorID
	PageSize  int
	Limits    Limits
}

type QueryResult struct {
	Result      *result.ResultSet
	CursorID    CursorID
	Exhausted   bool
	Transaction TxStatus
}

type ExecuteRequest struct {
	SessionID SessionID
	SQL       string
	Args      []any
	Limits    Limits
	Explain   *explain.Plan
	DDL       *ddl.Request
}

type ExecuteResult struct {
	Result      *result.ResultSet
	Transaction TxStatus
}

type StreamRequest struct {
	SQL        string
	Format     string
	Limits     Limits
	OnProgress func(rows, bytes int64)
}

type ChildrenRequest struct {
	NodeKind   string
	FolderKind string
	Database   string
	Parents    []metadata.ScopePath
}

// SessionCapabilities describes what the session's driver supports. Schema is
// the driver's static navigator grammar. Objects reports object inspection,
// which a driver can support without a navigator grammar.
type SessionCapabilities struct {
	Schema        *metadata.Tree
	DDL           *ddl.Spec
	Statements    *statement.Spec
	Objects       bool
	Relationships bool
	Definitions   bool
}
