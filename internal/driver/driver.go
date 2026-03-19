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
	Tables(ctx context.Context, database, schema string) ([]TableMeta, error)
	Columns(ctx context.Context, database, schema, table string) ([]ColumnMeta, error)
	Dialect() Dialect
}

// ConnectionConfig holds the configuration needed to open a driver connection.
type ConnectionConfig struct {
	DSN    string
	Driver string
}

// Dialect identifies the SQL dialect of a driver.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectSQLite   Dialect = "sqlite"
)

// TableMeta describes a table in the target database.
type TableMeta struct {
	Name   string
	Schema string
}

// ColumnMeta describes a column in the target database.
type ColumnMeta struct {
	Name     string
	Type     string
	Nullable bool
}
