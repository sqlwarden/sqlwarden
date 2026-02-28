package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/sqlwarden/assets"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
)

const defaultTimeout = 3 * time.Second

type DB struct {
	driver string
	dsn    string
	*bun.DB
}

func New(driver, dsn string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var sqldb *sql.DB
	var db *bun.DB
	var err error

	switch driver {
	case "postgres":
		pgDSN := dsn
		if !strings.HasPrefix(pgDSN, "postgres://") && !strings.HasPrefix(pgDSN, "postgresql://") {
			pgDSN = "postgres://" + pgDSN
		}

		sqldb = sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(pgDSN)))
		db = bun.NewDB(sqldb, pgdialect.New())
	case "sqlite":
		sqldb, err = sql.Open(sqliteshim.ShimName, dsn)
		if err != nil {
			return nil, err
		}

		db = bun.NewDB(sqldb, sqlitedialect.New())

		_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
		if err != nil {
			sqldb.Close()
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}

	sqldb.SetMaxOpenConns(25)
	sqldb.SetMaxIdleConns(25)
	sqldb.SetConnMaxIdleTime(5 * time.Minute)
	sqldb.SetConnMaxLifetime(2 * time.Hour)

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &DB{driver: driver, dsn: dsn, DB: db}, nil
}

func (db *DB) MigrateUp() error {
	migrationPath := "migrations_postgres"
	if db.driver == "sqlite" {
		migrationPath = "migrations_sqlite"
	}

	iofsDriver, err := iofs.New(assets.EmbeddedFiles, migrationPath)
	if err != nil {
		return err
	}

	var databaseURL string
	switch db.driver {
	case "postgres":
		databaseURL = "postgres://" + db.dsn
	case "sqlite":
		databaseURL = "sqlite://" + db.dsn
	default:
		return fmt.Errorf("unsupported database driver for migrations: %s", db.driver)
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, databaseURL)
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
