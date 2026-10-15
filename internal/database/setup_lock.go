package database

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// setupLockID identifies the first-run setup lock. It is an arbitrary but
// fixed value distinct from migrationLockID.
const setupLockID int64 = 7238105124099002

// SetupLock serializes first-run setup transactions, so the "no instance admin
// exists yet" check and the writes that follow it cannot interleave with a
// concurrent setup.
//
// Each database driver implements it with the locking primitive that driver
// actually has; there is no shared conditional on the driver name.
type SetupLock interface {
	// Acquire blocks until tx holds the lock or ctx ends. The lock is released
	// when tx commits or rolls back.
	Acquire(ctx context.Context, tx bun.Tx) error
}

// SetupLockFor returns the first-run setup lock implementation for a driver.
func SetupLockFor(driver string) (SetupLock, error) {
	switch driver {
	case "postgres":
		return postgresSetupLock{}, nil
	case "sqlite":
		return sqliteSetupLock{}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver for setup lock: %s", driver)
	}
}

// LockFirstRunSetup takes the first-run setup lock inside tx. Callers must take
// it before reading whether the instance is already configured.
func (db *DB) LockFirstRunSetup(ctx context.Context, tx bun.Tx) error {
	lock, err := SetupLockFor(db.driver)
	if err != nil {
		return err
	}
	return lock.Acquire(ctx, tx)
}

// postgresSetupLock takes a transaction-scoped advisory lock. Under READ
// COMMITTED each statement after it sees rows a previous holder committed, so
// a waiting setup observes the instance admin the winner created. There is no
// always-present singleton row to lock with SELECT ... FOR UPDATE instead.
type postgresSetupLock struct{}

func (postgresSetupLock) Acquire(ctx context.Context, tx bun.Tx) error {
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(?)", setupLockID); err != nil {
		return fmt.Errorf("acquire setup lock: %w", err)
	}
	return nil
}

// sqliteSetupLock is a no-op. The SQLite application database runs on a
// single pooled connection, so transactions in one process already run one at
// a time, and the file lock refuses a second concurrent writer.
type sqliteSetupLock struct{}

func (sqliteSetupLock) Acquire(context.Context, bun.Tx) error { return nil }
