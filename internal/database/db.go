package database

import (
	"context"
	"errors"
	"time"

	"github.com/sqlwarden/assets"
	"github.com/sqlwarden/internal/query"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const defaultTimeout = 3 * time.Second

type DB struct {
	dsn string
	*pgxpool.Pool
	*query.Queries
}

func New(dsn string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	config, err := pgxpool.ParseConfig("postgres://" + dsn)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 25
	config.MinConns = 25
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 2 * time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}

	queries := query.New(pool)

	return &DB{dsn: dsn, Pool: pool, Queries: queries}, nil
}

func (db *DB) MigrateUp() error {
	iofsDriver, err := iofs.New(assets.EmbeddedFiles, "migrations")
	if err != nil {
		return err
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, "postgres://"+db.dsn)
	if err != nil {
		return err
	}

	err = migrator.Up()
	switch {
	case errors.Is(err, migrate.ErrNoChange):
		return nil
	default:
		return err
	}
}
