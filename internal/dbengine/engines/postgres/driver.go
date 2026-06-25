package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type postgresDriver struct {
	db          *sql.DB
	scanOptions cursor.ScanOptions
}

func (d *postgresDriver) Connect(ctx context.Context, cfg dbengine.ConnectionConfig) error {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return fmt.Errorf("postgres: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	d.db = db
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	return nil
}

func (d *postgresDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *postgresDriver) Close() error {
	return d.db.Close()
}

func (d *postgresDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	return cursor.ScanRows(rows, d.scanOptions)
}

func (d *postgresDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: execute: %w", err)
	}
	return cursor.ScanRows(rows, d.scanOptions)
}

func (d *postgresDriver) Dialect() dbengine.Dialect {
	return dbengine.DialectPostgres
}
