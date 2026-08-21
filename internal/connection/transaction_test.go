package connection

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/pkg/result"
)

// mockTxDriver extends mockDriver with full transaction.SavepointController support.
type mockTxDriver struct {
	mockDriver
	inTx         bool
	commits      int
	rollbacks    int
	savepoints   []string
	failNextExec bool
}

func (d *mockTxDriver) BeginTx(ctx context.Context) error {
	if d.inTx {
		return errors.New("already in transaction")
	}
	d.inTx = true
	return nil
}
func (d *mockTxDriver) Commit(ctx context.Context) error {
	if !d.inTx {
		return transaction.ErrNoOpenTransaction
	}
	d.inTx = false
	d.commits++
	return nil
}
func (d *mockTxDriver) Rollback(ctx context.Context) error {
	if !d.inTx {
		return transaction.ErrNoOpenTransaction
	}
	d.inTx = false
	d.rollbacks++
	return nil
}
func (d *mockTxDriver) InTransaction() bool { return d.inTx }
func (d *mockTxDriver) Savepoint(ctx context.Context, name string) error {
	d.savepoints = append(d.savepoints, name)
	return nil
}
func (d *mockTxDriver) RollbackToSavepoint(ctx context.Context, name string) error { return nil }

func (d *mockTxDriver) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	if d.failNextExec {
		d.failNextExec = false
		return nil, errors.New("statement failed")
	}
	return d.mockDriver.Execute(ctx, sql, args...)
}

var _ transaction.SavepointController = (*mockTxDriver)(nil)
var _ engine.Driver = (*mockTxDriver)(nil)

func newTxSession(t *testing.T) (*Session, *mockTxDriver) {
	t.Helper()
	d := &mockTxDriver{}
	return &Session{ID: "s1", AccountID: "a1", ConnectionID: "c1", Conn: d}, d
}

func TestSetTransactionMode_ManualThenAuto(t *testing.T) {
	s, _ := newTxSession(t)
	ctx := context.Background()
	if s.TransactionStatus().Mode != TxModeAuto {
		t.Fatalf("default mode = %v, want auto", s.TransactionStatus().Mode)
	}
	if err := s.SetTransactionMode(ctx, TxModeManual); err != nil {
		t.Fatalf("SetTransactionMode(manual): %v", err)
	}
	if s.TransactionStatus().Mode != TxModeManual {
		t.Fatal("mode did not switch to manual")
	}
	if err := s.SetTransactionMode(ctx, TxModeAuto); err != nil {
		t.Fatalf("SetTransactionMode(auto): %v", err)
	}
}

func TestSetTransactionMode_AutoBlockedWhileOpen(t *testing.T) {
	s, _ := newTxSession(t)
	ctx := context.Background()
	if err := s.SetTransactionMode(ctx, TxModeManual); err != nil {
		t.Fatalf("SetTransactionMode(manual): %v", err)
	}
	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !s.TransactionStatus().Open {
		t.Fatal("expected transaction to be lazily opened by Execute")
	}
	if err := s.SetTransactionMode(ctx, TxModeAuto); !errors.Is(err, ErrTransactionOpen) {
		t.Fatalf("SetTransactionMode(auto) while open = %v, want ErrTransactionOpen", err)
	}
}

func TestManualMode_LazyBeginAndPendingCount(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)

	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !driver.inTx {
		t.Fatal("driver.BeginTx was not called on first manual-mode statement")
	}
	status := s.TransactionStatus()
	if status.PendingStatements != 1 {
		t.Fatalf("PendingStatements = %d, want 1", status.PendingStatements)
	}

	if _, err := s.Query(ctx, "SELECT 1"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if s.TransactionStatus().PendingStatements != 2 {
		t.Fatalf("PendingStatements after SELECT = %d, want 2", s.TransactionStatus().PendingStatements)
	}
}

func TestManualMode_StatementFailureRollsBackToSavepoint(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	driver.failNextExec = true
	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (bad)"); err == nil {
		t.Fatal("expected statement error")
	}
	if len(driver.savepoints) == 0 {
		t.Fatal("expected at least one savepoint to have been taken")
	}
	// Transaction stays open for the user to inspect/commit/rollback.
	if !s.TransactionStatus().Open {
		t.Fatal("transaction should remain open after a savepoint-recovered failure")
	}
	if driver.rollbacks != 0 {
		t.Fatalf("rollbacks = %d, want 0 (only the savepoint should roll back)", driver.rollbacks)
	}
}

func TestCommitTransaction_ResetsState(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	_, _ = s.Execute(ctx, "INSERT INTO t VALUES (1)")

	if err := s.CommitTransaction(ctx); err != nil {
		t.Fatalf("CommitTransaction: %v", err)
	}
	if driver.commits != 1 {
		t.Fatalf("commits = %d, want 1", driver.commits)
	}
	status := s.TransactionStatus()
	if status.Open || status.PendingStatements != 0 {
		t.Fatalf("status after commit = %+v, want closed with 0 pending", status)
	}
}

func TestRollbackTransaction_RequiresOpenTransaction(t *testing.T) {
	s, _ := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	if err := s.RollbackTransaction(ctx); !errors.Is(err, transaction.ErrNoOpenTransaction) {
		t.Fatalf("RollbackTransaction with nothing open = %v, want ErrNoOpenTransaction", err)
	}
}

func TestSessionClose_RollsBackOpenTransaction(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	_, _ = s.Execute(ctx, "INSERT INTO t VALUES (1)")

	s.close()

	if driver.rollbacks != 1 {
		t.Fatalf("rollbacks on close = %d, want 1", driver.rollbacks)
	}
	if !driver.closed {
		t.Fatal("driver was not closed")
	}
}
