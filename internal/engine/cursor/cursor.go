// Package cursor is the database/sql layer for streaming query results: the
// QueryCursor interface for forward-only, page-at-a-time reads, its default
// database/sql-backed implementation (SQLRowsCursor), and the shared row-scanning
// helpers (ScanRows, NormalizeValue) every engine uses to turn driver rows into a
// normalized result.ResultSet. An engine opts into server-side cursors by
// implementing QueryCursorDriver.
package cursor

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
