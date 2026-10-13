package database

import (
	"context"
	"testing"
	"time"

	"github.com/uptrace/bun"
)

func TestSetupLockForRejectsUnknownDriver(t *testing.T) {
	if _, err := SetupLockFor("mysql"); err == nil {
		t.Fatal("SetupLockFor() accepted an unsupported driver")
	}
}

func TestLockFirstRunSetupSQLite(t *testing.T) {
	db := newTestDB(t, "sqlite")
	err := db.RunInTx(context.Background(), nil, func(ctx context.Context, tx bun.Tx) error {
		return db.LockFirstRunSetup(ctx, tx)
	})
	if err != nil {
		t.Fatalf("LockFirstRunSetup() error = %v", err)
	}
}

func TestLockFirstRunSetupPostgresSerializesTransactions(t *testing.T) {
	db := newTestDB(t)
	db.SetMaxOpenConns(3)
	ctx := context.Background()

	first, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback() }()
	if err := db.LockFirstRunSetup(ctx, first); err != nil {
		t.Fatal(err)
	}

	acquired := make(chan error, 1)
	go func() {
		acquired <- db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			return db.LockFirstRunSetup(ctx, tx)
		})
	}()

	select {
	case err := <-acquired:
		t.Fatalf("second transaction took the setup lock while the first held it (err=%v)", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second LockFirstRunSetup() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("setup lock was not released when the holding transaction committed")
	}
}

func TestLockFirstRunSetupPostgresHonorsContext(t *testing.T) {
	db := newTestDB(t)
	db.SetMaxOpenConns(3)
	ctx := context.Background()

	first, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback() }()
	if err := db.LockFirstRunSetup(ctx, first); err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	err = db.RunInTx(waitCtx, nil, func(ctx context.Context, tx bun.Tx) error {
		return db.LockFirstRunSetup(ctx, tx)
	})
	if err == nil {
		t.Fatal("LockFirstRunSetup() acquired a held lock")
	}
}
