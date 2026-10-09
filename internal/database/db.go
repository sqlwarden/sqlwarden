package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"strings"
	"sync/atomic"
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

// CoreMigrationVersion is the latest ordered migration shipped by core.
// Independently-versioned edition streams declare compatibility with it.
const CoreMigrationVersion uint = 46

type DB struct {
	logger       *slog.Logger
	driver       string
	dsn          string
	queryTracing atomic.Bool
	*bun.DB
}

func New(driver, dsn string, logger *slog.Logger) (*DB, error) {
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

	result := &DB{driver: driver, dsn: dsn, DB: db, logger: logger}
	db.AddQueryHook(&debugQueryLoggerHook{logger: logger, enabled: &result.queryTracing})
	db.AddQueryHook(&slowQueryDetectorHook{threshold: 100, logger: logger})

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

	return result, nil
}

func (db *DB) SetQueryTracing(enabled bool) {
	db.queryTracing.Store(enabled)
}

func (db *DB) MigrateUp() error {
	return db.MigrateStream(assets.EmbeddedFiles, "migrations_postgres", "migrations_sqlite", "schema_migrations")
}

// MigrateStream applies an independently-versioned migration stream using its
// own history table.
func (db *DB) MigrateStream(files fs.FS, postgresPath, sqlitePath, historyTable string) error {
	if !validMigrationTable(historyTable) {
		return fmt.Errorf("invalid migration history table %q", historyTable)
	}
	migrationPath := "migrations_postgres"
	if db.driver == "sqlite" {
		migrationPath = sqlitePath
	} else {
		migrationPath = postgresPath
	}

	iofsDriver, err := iofs.New(files, migrationPath)
	if err != nil {
		return err
	}

	var databaseURL string
	switch db.driver {
	case "postgres":
		databaseURL = db.dsn
		if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
			databaseURL = "postgres://" + databaseURL
		}
	case "sqlite":
		databaseURL = "sqlite://" + db.dsn
	default:
		return fmt.Errorf("unsupported database driver for migrations: %s", db.driver)
	}
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	databaseURL += separator + "x-migrations-table=" + url.QueryEscape(historyTable)

	migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		sourceErr, dbErr := migrator.Close()
		if closeErr := errors.Join(sourceErr, dbErr); closeErr != nil {
			db.logger.Warn("migrator shutdown failed", "error", closeErr)
		}
	}()

	err = migrator.Up()
	switch {
	case errors.Is(err, migrate.ErrNoChange):
		return nil
	default:
		return err
	}
}

func validMigrationTable(table string) bool {
	if table == "" {
		return false
	}
	for _, character := range table {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

// sqlSortDirection converts an API sort direction into one of the only two SQL
// tokens the database layer is allowed to emit.
func sqlSortDirection(order string) string {
	if order == "asc" {
		return "ASC"
	}
	return "DESC"
}
