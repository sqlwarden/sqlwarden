package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"

	_ "modernc.org/sqlite"
)

type sqliteDriver struct {
	db           *sql.DB
	currentTx    *sql.Tx
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
}

type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (d *sqliteDriver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
}

func (d *sqliteDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	db, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return fmt.Errorf("sqlite: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("sqlite: ping: %w", err)
	}
	// Enable WAL mode for better concurrency.
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return fmt.Errorf("sqlite: WAL mode: %w", err)
	}
	d.db = db
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	d.defaultScope = cfg.DefaultScope
	return nil
}

func (d *sqliteDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *sqliteDriver) Close() error {
	return d.db.Close()
}

func (d *sqliteDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.QueryWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *sqliteDriver) QueryWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *sqliteDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *sqliteDriver) ExecuteWithOptions(ctx context.Context, query string, _ cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	execResult, err := d.conn().ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: execute: %w", err)
	}
	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return &result.ResultSet{}, nil
	}
	return result.NewExecutionResult(rowsAffected), nil
}

func (d *sqliteDriver) Dialect() engine.Dialect {
	return engine.DialectSQLite
}
