package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source"
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

var validMigrationTable = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type DB struct {
	logger *slog.Logger
	driver string
	dsn    string
	*bun.DB
}

func New(driver, dsn string, logger *slog.Logger, logQueries bool) (*DB, error) {
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
		_, err = db.ExecContext(ctx, "PRAGMA busy_timeout = 5000")
		if err != nil {
			sqldb.Close()
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}

	if logQueries {
		db.AddQueryHook(&debugQueryLoggerHook{logger: logger})
	}
	db.AddQueryHook(&slowQueryDetectorHook{threshold: 100, includeQuery: logQueries, logger: logger})

	if driver == "sqlite" {
		sqldb.SetMaxOpenConns(1)
		sqldb.SetMaxIdleConns(1)
	} else {
		sqldb.SetMaxOpenConns(25)
		sqldb.SetMaxIdleConns(25)
	}
	sqldb.SetConnMaxIdleTime(5 * time.Minute)
	sqldb.SetConnMaxLifetime(2 * time.Hour)

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &DB{driver: driver, dsn: dsn, DB: db, logger: logger}, nil
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

	databaseURL, err := migrationDatabaseURL(db.driver, db.dsn)
	if err != nil {
		return errors.Join(err, iofsDriver.Close())
	}

	return migrateUp(iofsDriver, databaseURL)
}

// MigrateExtensionUp applies an extension's migration stream from src,
// tracking versions in the given table so extension streams can never
// collide with core migration numbering in schema_migrations.
func (db *DB) MigrateExtensionUp(src fs.FS, table string) error {
	if !validMigrationTable.MatchString(table) {
		return fmt.Errorf("invalid extension migration table %q", table)
	}

	iofsDriver, err := iofs.New(src, ".")
	if err != nil {
		return err
	}

	databaseURL, err := migrationDatabaseURL(db.driver, db.dsn)
	if err != nil {
		return errors.Join(err, iofsDriver.Close())
	}
	sep := "?"
	if strings.Contains(databaseURL, "?") {
		sep = "&"
	}
	databaseURL += sep + "x-migrations-table=" + table

	return migrateUp(iofsDriver, databaseURL)
}

func migrationDatabaseURL(driver, dsn string) (string, error) {
	switch driver {
	case "postgres":
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			return dsn, nil
		}
		return "postgres://" + dsn, nil
	case "sqlite":
		if strings.HasPrefix(dsn, "sqlite://") {
			return dsn, nil
		}
		return "sqlite://" + dsn, nil
	default:
		return "", fmt.Errorf("unsupported database driver for migrations: %s", driver)
	}
}

func migrateUp(iofsDriver source.Driver, databaseURL string) (retErr error) {
	migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, databaseURL)
	if err != nil {
		return errors.Join(err, iofsDriver.Close())
	}
	defer func() {
		sourceErr, databaseErr := migrator.Close()
		retErr = errors.Join(retErr, sourceErr, databaseErr)
	}()

	err = migrator.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// sqlSortDirection converts an API sort direction into one of the only two SQL
// tokens the database layer is allowed to emit.
func sqlSortDirection(order string) string {
	if order == "asc" {
		return "ASC"
	}
	return "DESC"
}
