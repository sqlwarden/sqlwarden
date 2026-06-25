package dbengine

import (
	"context"

	"github.com/sqlwarden/pkg/result"
)

// Driver is the connection capability every engine must implement. An engine
// type also implements whichever optional capability interfaces it supports
// (classifier.Classifier, parser.Parser, rewriter.Rewriter, completer.Completer,
// schema.SchemaInspector, cursor.QueryCursorDriver), resolved by type assertion.
type Driver interface {
	Connect(ctx context.Context, cfg ConnectionConfig) error
	Ping(ctx context.Context) error
	Close() error
	Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
	Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
	Dialect() Dialect
}

// Dialect identifies the SQL dialect of an engine. It is also the canonical
// engine name used as the registry key.
type Dialect string

// The dialects SQLWarden ships engines for.
const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectSQLite   Dialect = "sqlite"
)

// ConnectionConfig holds the configuration an engine needs to open a connection.
type ConnectionConfig struct {
	DSN            string
	Driver         string
	MaxResultRows  int
	MaxResultBytes int64
}

// NormalizeName returns the canonical engine name for a user-facing name or
// known alias ("postgresql" -> "postgres", "sqlite3" -> "sqlite", "mariadb" ->
// "mysql").
func NormalizeName(name string) string {
	switch name {
	case "postgresql":
		return "postgres"
	case "sqlite3":
		return "sqlite"
	case "mariadb":
		return "mysql"
	default:
		return name
	}
}
