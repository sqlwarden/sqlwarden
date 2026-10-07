package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// migrationLockID identifies the SQLWarden schema migration lock. It is an
// arbitrary but fixed value; every process that migrates the same database must
// use it, and no other advisory lock in the application may reuse it.
const migrationLockID int64 = 7238105124099001

const migrationLockPollInterval = 500 * time.Millisecond

// MigrationLock serializes migration runs across every process that shares one
// application database, so a rollout that starts several migrate runs applies
// migrations exactly once.
//
// Each database driver implements it with the locking primitive that driver
// actually has; there is no shared conditional on the driver name.
type MigrationLock interface {
	// Acquire blocks until the lock is held or ctx ends. The returned release
	// function frees the lock and the resources held with it.
	Acquire(ctx context.Context) (release func(context.Context) error, err error)
}

// MigrationLockFor returns the migration lock implementation for a driver.
func MigrationLockFor(driver string, db *sql.DB) (MigrationLock, error) {
	switch driver {
	case "postgres":
		return &postgresMigrationLock{db: db}, nil
	case "sqlite":
		return sqliteMigrationLock{}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver for migrations: %s", driver)
	}
}

// MigrateLocked runs migrate while holding the migration lock. The context
// bounds lock acquisition and is passed to migrate so a context-aware runner
// can stop at the deadline. If migrate does not return promptly after the
// context ends, MigrateLocked keeps waiting: releasing the lock while schema
// changes are still executing would allow a second migrator to interleave.
func (db *DB) MigrateLocked(ctx context.Context, migrate func(context.Context) error) error {
	lock, err := MigrationLockFor(db.driver, db.DB.DB)
	if err != nil {
		return err
	}
	release, err := lock.Acquire(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := release(context.WithoutCancel(ctx)); releaseErr != nil {
			db.logger.Warn("migration lock release failed", "error", releaseErr)
		}
	}()

	done := make(chan error, 1)
	go func() { done <- migrate(ctx) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		migrationErr := <-done
		return errors.Join(fmt.Errorf("database migration did not finish in time: %w", ctx.Err()), migrationErr)
	}
}

// postgresMigrationLock holds a session-scoped advisory lock on a dedicated
// connection, so the lock survives for exactly as long as the migration run.
type postgresMigrationLock struct {
	db *sql.DB
}

func (l *postgresMigrationLock) Acquire(ctx context.Context) (func(context.Context) error, error) {
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open migration lock connection: %w", err)
	}
	release := func(releaseCtx context.Context) error {
		_, unlockErr := conn.ExecContext(releaseCtx, "SELECT pg_advisory_unlock($1)", migrationLockID)
		return errors.Join(unlockErr, conn.Close())
	}

	ticker := time.NewTicker(migrationLockPollInterval)
	defer ticker.Stop()
	for {
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrationLockID).Scan(&acquired); err != nil {
			return nil, errors.Join(fmt.Errorf("acquire migration lock: %w", err), conn.Close())
		}
		if acquired {
			return release, nil
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(fmt.Errorf("wait for migration lock: %w", ctx.Err()), conn.Close())
		case <-ticker.C:
		}
	}
}

// sqliteMigrationLock is a no-op. A SQLite application database is a single
// file served by a single process, and concurrent writers are already
// serialized by the file lock the driver takes.
type sqliteMigrationLock struct{}

func (sqliteMigrationLock) Acquire(context.Context) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
