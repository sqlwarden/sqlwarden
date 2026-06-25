package dbsql

import (
	"context"

	"github.com/sqlwarden/pkg/result"
)

// QueryRequest is a single SQL statement plus positional arguments to open a
// streaming cursor for.
type QueryRequest struct {
	SQL  string
	Args []any
}

// QueryCursorState is the per-page progress reported by a cursor Fetch.
type QueryCursorState struct {
	Exhausted     bool
	RowsReturned  int
	BytesReturned int64
}

// QueryCursor is a forward-only, page-at-a-time result stream. SQLRowsCursor is
// the default database/sql-backed implementation.
type QueryCursor interface {
	Columns() []result.Column
	Fetch(ctx context.Context, opts ScanOptions) (*result.ResultSet, QueryCursorState, error)
	Close() error
}

// QueryCursorDriver is the optional interface a driver implements to open
// server-side cursors. Capability discovery type-asserts against it.
type QueryCursorDriver interface {
	StartQuery(ctx context.Context, req QueryRequest) (QueryCursor, error)
}
