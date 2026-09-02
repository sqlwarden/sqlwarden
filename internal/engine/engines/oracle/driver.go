package oracle

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/sijms/go-ora/v2"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

type oracleDriver struct {
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

func (d *oracleDriver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
}

func (d *oracleDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	db, err := sql.Open("oracle", cfg.DSN)
	if err != nil {
		return fmt.Errorf("oracle: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("oracle: ping: %w", err)
	}
	if schema := cfg.DefaultScope.Name("schema"); schema != "" {
		// schema is an identifier from stored connection config, quoted here so
		// punctuation cannot alter the statement.
		// codeql[go/sql-injection]
		if _, err := db.ExecContext(ctx, "ALTER SESSION SET CURRENT_SCHEMA = "+oracleQuoteIdent(schema)); err != nil {
			db.Close()
			return fmt.Errorf("oracle: set current schema: %w", err)
		}
	}
	d.db = db
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	d.defaultScope = cfg.DefaultScope
	return nil
}

func (d *oracleDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *oracleDriver) Close() error {
	return d.db.Close()
}

func (d *oracleDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.QueryWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *oracleDriver) QueryWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *oracleDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *oracleDriver) ExecuteWithOptions(ctx context.Context, query string, _ cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	execResult, err := d.conn().ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: execute: %w", err)
	}
	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return &result.ResultSet{}, nil
	}
	return result.NewExecutionResult(rowsAffected), nil
}

func (d *oracleDriver) Dialect() engine.Dialect {
	return engine.DialectOracle
}
