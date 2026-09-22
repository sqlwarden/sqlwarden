package database

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
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

func TestMigrateLockedReportsTimeout(t *testing.T) {
	db := testMigrationDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := db.MigrateLocked(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("MigrateLocked() error = %v, want a deadline error", err)
	}
}

func TestMigrateLockedDoesNotReleaseBeforeTimedOutMigrationStops(t *testing.T) {
	db := testMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	stopped := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.MigrateLocked(ctx, func(context.Context) error {
			time.Sleep(75 * time.Millisecond)
			close(stopped)
			return nil
		})
	}()

	select {
	case err := <-done:
		t.Fatalf("MigrateLocked returned before migration stopped: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	<-stopped
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("MigrateLocked() error = %v, want deadline exceeded", err)
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
