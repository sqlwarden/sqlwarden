package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

// Driver is the PostgreSQL engine.Driver implementation. Fields are
// unexported; compatible engines (e.g. a future Supabase/Neon/CockroachDB
// package) embed Driver by value and reach the live connection through DB(),
// never through the field directly.
type Driver struct {
	db           *sql.DB
	currentTx    *sql.Tx
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
}

// DB returns the underlying connection pool. Catalog/DDL introspection always
// runs directly against it (never through an open transaction), so this is
// the handle compatible engines pass into the exported catalog.go functions.
func (d *Driver) DB() *sql.DB {
	return d.db
}

// DefaultScope returns the connection's configured default scope (e.g. the
// selected schema). Compatible engines that override InspectDirectory or
// InspectObjects need this to reproduce the same default-scope resolution
// the embedded implementation performs.
func (d *Driver) DefaultScope() metadata.ScopePath {
	return d.defaultScope
}

// execer is satisfied by both *sql.DB and *sql.Tx, letting every statement
// path resolve through the same accessor whether or not a transaction is open.
type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (d *Driver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
}

// buildPgxConfig parses the DSN and folds in the default schema, the structured
// TLS material, and an optional SSH tunnel dialer. When TLS material is present
// it becomes the single source of truth: pgx's own libpq-style ssl* knobs are
// dropped so a stale DSN value cannot override the configured verification mode.
// pgx runs DialFunc before the TLS handshake, so an SSH tunnel and DB TLS
// compose: the handshake still targets the real ServerName, not the bastion.
func buildPgxConfig(cfg engine.ConnectionConfig) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	if selectedSchema := cfg.DefaultScope.Name("schema"); selectedSchema != "" {
		// search_path is a PostgreSQL identifier list, not a query parameter.
		// Quote it as one identifier so punctuation cannot alter the path.
		config.RuntimeParams["search_path"] = `"` + strings.ReplaceAll(selectedSchema, `"`, `""`) + `"`
	}
	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return nil, fmt.Errorf("postgres: tls config: %w", err)
	}
	switch {
	case tlsCfg != nil:
		if tlsCfg.ServerName == "" {
			tlsCfg.ServerName = config.Host
		}
		config.TLSConfig = tlsCfg
		for _, k := range []string{"sslmode", "sslrootcert", "sslcert", "sslkey", "sslpassword"} {
			delete(config.RuntimeParams, k)
		}
	case cfg.TLS != nil && cfg.TLS.Mode == engine.TLSModeDisable:
		config.TLSConfig = nil
		config.Fallbacks = nil
		for _, k := range []string{"sslmode", "sslrootcert", "sslcert", "sslkey", "sslpassword"} {
			delete(config.RuntimeParams, k)
		}
	}
	if cfg.SSHDialer != nil {
		config.DialFunc = pgconn.DialFunc(cfg.SSHDialer)
	}
	return config, nil
}

func (d *Driver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	config, err := buildPgxConfig(cfg)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*config)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	d.db = db
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	d.defaultScope = cfg.DefaultScope
	return nil
}

func (d *Driver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *Driver) Close() error {
	return d.db.Close()
}

func (d *Driver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.QueryWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *Driver) QueryWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *Driver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *Driver) ExecuteWithOptions(ctx context.Context, query string, _ cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	execResult, err := d.conn().ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: execute: %w", err)
	}
	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return &result.ResultSet{}, nil
	}
	return result.NewExecutionResult(rowsAffected), nil
}

func (d *Driver) Dialect() engine.Dialect {
	return engine.DialectPostgres
}
