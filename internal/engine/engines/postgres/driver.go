package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

type postgresDriver struct {
	db           *sql.DB
	currentTx    *sql.Tx
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
}

// execer is satisfied by both *sql.DB and *sql.Tx, letting every statement
// path resolve through the same accessor whether or not a transaction is open.
type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (d *postgresDriver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
}

// buildPgxConfig parses the DSN and folds in the default schema and the
// structured TLS material. When TLS material is present it becomes the single
// source of truth: pgx's own libpq-style ssl* knobs are dropped so a stale DSN
// value cannot override the configured verification mode.
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
	if tlsCfg != nil {
		if tlsCfg.ServerName == "" {
			tlsCfg.ServerName = config.Host
		}
		config.TLSConfig = tlsCfg
		for _, k := range []string{"sslmode", "sslrootcert", "sslcert", "sslkey", "sslpassword"} {
			delete(config.RuntimeParams, k)
		}
	}
	return config, nil
}

func (d *postgresDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
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

func (d *postgresDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *postgresDriver) Close() error {
	return d.db.Close()
}

func (d *postgresDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.QueryWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *postgresDriver) QueryWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *postgresDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *postgresDriver) ExecuteWithOptions(ctx context.Context, query string, _ cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
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

func (d *postgresDriver) Dialect() engine.Dialect {
	return engine.DialectPostgres
}
