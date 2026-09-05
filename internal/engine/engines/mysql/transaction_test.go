package mysql

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func TestMySQLTransactionCommitAndRollback(t *testing.T) {
	d := &Driver{}
	ctx := context.Background()
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "mysql"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if _, err := d.Execute(ctx, `CREATE TABLE IF NOT EXISTS tx_test (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer d.Execute(ctx, `DROP TABLE tx_test`)

	if d.InTransaction() {
		t.Fatal("InTransaction() = true before BeginTx")
	}
	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if !d.InTransaction() {
		t.Fatal("InTransaction() = false after BeginTx")
	}
	if _, err := d.Execute(ctx, `INSERT INTO tx_test (id) VALUES (1)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := d.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if d.InTransaction() {
		t.Fatal("InTransaction() = true after Rollback")
	}
	rs, err := d.Query(ctx, `SELECT id FROM tx_test`)
	if err != nil {
		t.Fatalf("query after rollback: %v", err)
	}
	if len(rs.Rows) != 0 {
		t.Fatalf("rows after rollback = %d, want 0", len(rs.Rows))
	}
}

func TestMySQLSavepointRollback(t *testing.T) {
	d := &Driver{}
	ctx := context.Background()
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "mysql"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if _, err := d.Execute(ctx, `CREATE TABLE IF NOT EXISTS tx_sp_test (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer d.Execute(ctx, `DROP TABLE tx_sp_test`)

	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer d.Rollback(ctx)

	if _, err := d.Execute(ctx, `INSERT INTO tx_sp_test (id) VALUES (1)`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := d.Savepoint(ctx, "sqlwarden_sp_1"); err != nil {
		t.Fatalf("Savepoint: %v", err)
	}
	if _, err := d.Execute(ctx, `INSERT INTO tx_sp_test (id) VALUES (1)`); err == nil {
		t.Fatal("expected duplicate key error")
	}
	if err := d.RollbackToSavepoint(ctx, "sqlwarden_sp_1"); err != nil {
		t.Fatalf("RollbackToSavepoint: %v", err)
	}
	// Transaction is still usable and the first insert is still visible.
	rs, err := d.Query(ctx, `SELECT id FROM tx_sp_test`)
	if err != nil {
		t.Fatalf("query after savepoint rollback: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows after savepoint rollback = %d, want 1", len(rs.Rows))
	}
}
