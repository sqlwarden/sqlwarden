package database

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testMigrationDB(t *testing.T) *DB {
	t.Helper()
	db, err := New("sqlite", filepath.Join(t.TempDir(), "sqlwarden.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateLockedAppliesMigrations(t *testing.T) {
	db := testMigrationDB(t)

	if err := db.MigrateLocked(context.Background(), func(context.Context) error { return db.MigrateUp() }); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("migration history is empty after a locked migration run")
	}
}

func TestMigrateLockedTimeoutDoesNotBoundTheMigration(t *testing.T) {
	db := testMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var callbackCtxErr error
	err := db.MigrateLocked(ctx, func(migrateCtx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		callbackCtxErr = migrateCtx.Err()
		return nil
	})
	if err != nil {
		t.Fatalf("MigrateLocked() error = %v, want nil for a slow migration that succeeds", err)
	}
	if callbackCtxErr != nil {
		t.Fatalf("migration context was canceled: %v", callbackCtxErr)
	}
}

func TestMigrationLockForRejectsUnknownDriver(t *testing.T) {
	if _, err := MigrationLockFor("mysql", nil); err == nil {
		t.Fatal("MigrationLockFor() accepted an unsupported driver")
	}
}

func TestPostgresMigrationLockSerializesConnections(t *testing.T) {
	db := newTestDB(t)
	first, err := MigrationLockFor("postgres", db.DB.DB)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MigrationLockFor("postgres", db.DB.DB)
	if err != nil {
		t.Fatal(err)
	}
	releaseFirst, err := first.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := second.Acquire(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Acquire() error = %v, want deadline exceeded", err)
	}
	if err := releaseFirst(context.Background()); err != nil {
		t.Fatal(err)
	}

	releaseSecond, err := second.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseSecond(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLockedSerializesPostgresRuns(t *testing.T) {
	db := newTestDB(t, "postgres")
	var running, overlaps atomic.Int32
	run := func(context.Context) error {
		if running.Add(1) > 1 {
			overlaps.Add(1)
		}
		time.Sleep(200 * time.Millisecond)
		running.Add(-1)
		return nil
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.MigrateLocked(context.Background(), run); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if overlaps.Load() != 0 {
		t.Fatal("migration callbacks overlapped")
	}
}

func TestMigrateLockedRunsCallbackOnceAndReturnsItsError(t *testing.T) {
	db := testMigrationDB(t)
	sentinel := errors.New("migration failed")
	var count int

	err := db.MigrateLocked(context.Background(), func(context.Context) error {
		count++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("MigrateLocked() error = %v, want sentinel", err)
	}
	if count != 1 {
		t.Fatalf("callback ran %d times, want 1", count)
	}
}
