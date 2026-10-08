package execution

import (
	"context"
	"io"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/exports"
)

// TargetPolicy vets a target before any connection to it is attempted.
type TargetPolicy interface {
	Check(ctx context.Context, driver, dsn string) error
}

// Prober connects to and pings a target, then runs fn against the short-lived
// inspector, which is closed afterwards.
type Prober interface {
	Probe(ctx context.Context, scope Scope, creds credentials.Credentials, limits Limits, fn func(metadata.SchemaInspector) error) error
}

type Sessions interface {
	Open(ctx context.Context, req OpenRequest) (SessionInfo, error)
	Get(ctx context.Context, scope Scope, id SessionID) (SessionInfo, error)
	List(ctx context.Context, filter SessionFilter) ([]SessionInfo, error)
	Close(ctx context.Context, scope Scope, id SessionID) error
}

type Queries interface {
	Query(ctx context.Context, scope Scope, req QueryRequest) (QueryResult, error)
	Fetch(ctx context.Context, scope Scope, req FetchRequest) (QueryResult, error)
	CloseCursor(ctx context.Context, scope Scope, id SessionID, cursor CursorID) error
	Execute(ctx context.Context, scope Scope, req ExecuteRequest) (ExecuteResult, error)
	Cancel(ctx context.Context, scope Scope, id SessionID) error
	Stream(ctx context.Context, scope Scope, id SessionID, req StreamRequest, w io.Writer) (exports.StreamResult, error)
}

type Transactions interface {
	Status(ctx context.Context, scope Scope, id SessionID) (TxStatus, error)
	SetMode(ctx context.Context, scope Scope, id SessionID, mode TxMode) (TxStatus, error)
	Commit(ctx context.Context, scope Scope, id SessionID) (TxStatus, error)
	Rollback(ctx context.Context, scope Scope, id SessionID) (TxStatus, error)
}

type Metadata interface {
	LoadChildren(ctx context.Context, scope Scope, id SessionID, req ChildrenRequest) (map[metadata.ScopePath][]metadata.Child, error)
	InspectObjects(ctx context.Context, scope Scope, id SessionID, refs []metadata.ObjectRef) ([]metadata.Object, error)
	InspectRelationships(ctx context.Context, scope Scope, id SessionID, path metadata.ScopePath) (*metadata.RelationshipGraph, error)
	InspectDefinition(ctx context.Context, scope Scope, id SessionID, ref metadata.ObjectRef) (*metadata.Descriptor, error)
	CurrentScope(ctx context.Context, scope Scope, id SessionID) (metadata.ScopePath, error)
}

type Capabilities interface {
	Capabilities(ctx context.Context, scope Scope, id SessionID) (SessionCapabilities, error)
	ApplyDDL(ctx context.Context, scope Scope, id SessionID, req ddl.Request) (TxStatus, error)
}

// Revoker closes sessions when access to them is withdrawn and reports how
// many were affected.
type Revoker interface {
	CountForConnection(ctx context.Context, connID string) (int, error)
	RevokeConnection(ctx context.Context, connID string) (int, error)
	RevokeWorkspaceAccount(ctx context.Context, wsID, accountID string) (int, error)
	RevokeOrgAccount(ctx context.Context, orgID, accountID string) (int, error)
}

// Runtime is the full execution contract. Every operation is keyed by an
// opaque SessionID and re-checks Scope; request contexts bound only the
// individual operation, never the lifetime of a session or cursor.
type Runtime interface {
	Sessions
	Queries
	Transactions
	Metadata
	Capabilities
	Revoker
	Prober
}
