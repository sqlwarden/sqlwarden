package dbengine

import (
	"context"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"
)

// EngineID is the canonical identifier for a database engine (e.g. "postgres").
type EngineID string

// Dialect re-exports the driver dialect so callers can depend on dbengine
// without importing internal/driver directly.
type Dialect = driver.Dialect

const (
	DialectPostgres = driver.DialectPostgres
	DialectMySQL    = driver.DialectMySQL
	DialectSQLite   = driver.DialectSQLite
)

// ConnectionConfig re-exports driver.ConnectionConfig.
type ConnectionConfig = driver.ConnectionConfig

// EngineDescriptor is the static identity of an engine, safe to serialize and
// to report without opening a target connection.
type EngineDescriptor struct {
	ID          EngineID `json:"id"`
	DisplayName string   `json:"display_name"`
	Dialect     Dialect  `json:"dialect"`
}

// Connection is a live target-database session: a driver returned by New on
// which Connect has been called. Optional capabilities (schema inspection,
// cursors) are resolved by asserting the concrete connection against
// schema.SchemaInspector / dbsql.QueryCursorDriver.
type Connection interface {
	Ping(ctx context.Context) error
	Close() error
	Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
	Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
}
