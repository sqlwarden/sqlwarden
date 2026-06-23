package driver

import (
	"context"

	"github.com/sqlwarden/pkg/result"
)

// Driver is the interface that all target database drivers must implement.
type Driver interface {
	Connect(ctx context.Context, cfg ConnectionConfig) error
	Ping(ctx context.Context) error
	Close() error
	Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
	Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
	Dialect() Dialect
}

type QueryRequest struct {
	SQL  string
	Args []any
}

type QueryCursorState struct {
	Exhausted     bool
	RowsReturned  int
	BytesReturned int64
}

type QueryCursor interface {
	Columns() []result.Column
	Fetch(ctx context.Context, opts ScanOptions) (*result.ResultSet, QueryCursorState, error)
	Close() error
}

type QuerySessionDriver interface {
	StartQuery(ctx context.Context, req QueryRequest) (QueryCursor, error)
}

// ConnectionConfig holds the configuration needed to open a driver connection.
type ConnectionConfig struct {
	DSN            string
	Driver         string
	MaxResultRows  int
	MaxResultBytes int64
}

// Dialect identifies the SQL dialect of a driver.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectSQLite   Dialect = "sqlite"
)
