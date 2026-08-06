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
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
}

func (d *postgresDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	config, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return fmt.Errorf("postgres: parse config: %w", err)
	}
	if selectedSchema := cfg.DefaultScope.Name("schema"); selectedSchema != "" {
		// search_path is a PostgreSQL identifier list, not a query parameter.
		// Quote it as one identifier so punctuation cannot alter the path.
		config.RuntimeParams["search_path"] = `"` + strings.ReplaceAll(selectedSchema, `"`, `""`) + `"`
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
	// SQL is intentionally user-authored IDE input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *postgresDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *postgresDriver) ExecuteWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored IDE input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: execute: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *postgresDriver) Dialect() engine.Dialect {
	return engine.DialectPostgres
}
