package connection

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/pkg/result"
)

// mockTxDriver extends mockDriver with full transaction.SavepointController support.
type mockTxDriver struct {
	mockDriver
	inTx                      bool
	beginCtx                  context.Context
	commits                   int
	rollbacks                 int
	savepoints                []string
	failNextExec              bool
	failNextCommit            bool
	failNextRollback          bool
	failNextSavepointRollback bool
}

func (d *mockTxDriver) BeginTx(ctx context.Context) error {
	if d.inTx {
		return errors.New("already in transaction")
	}
	d.inTx = true
	d.beginCtx = ctx
	return nil
}

// txDead mimics database/sql: a *sql.Tx is tied to the context passed to
// BeginTx and is rolled back in the background once that context is done,
// even though nothing has told the driver the transaction closed.
func (d *mockTxDriver) txDead() bool {
	return d.beginCtx != nil && d.beginCtx.Err() != nil
}

// Commit/Rollback unset inTx even on failure, matching every real driver
// (postgres/mysql/sqlite): a *sql.Tx is unusable after Commit/Rollback
// regardless of whether the call errored, so the driver can't keep treating
// it as open even when the failure came from a dead connection.
func (d *mockTxDriver) Commit(ctx context.Context) error {
	if !d.inTx {
		return transaction.ErrNoOpenTransaction
	}
	d.inTx = false
	if d.failNextCommit {
		d.failNextCommit = false
		return errors.New("commit: connection reset by peer")
	}
	d.commits++
	return nil
}
func (d *mockTxDriver) Rollback(ctx context.Context) error {
	if !d.inTx {
		return transaction.ErrNoOpenTransaction
	}
	d.inTx = false
	if d.failNextRollback {
		d.failNextRollback = false
		return errors.New("rollback: connection reset by peer")
	}
	d.rollbacks++
	return nil
}
func (d *mockTxDriver) InTransaction() bool { return d.inTx }
func (d *mockTxDriver) Savepoint(ctx context.Context, name string) error {
	if d.txDead() {
		return errors.New("sql: transaction has already been committed or rolled back")
	}
	d.savepoints = append(d.savepoints, name)
	return nil
}
func (d *mockTxDriver) RollbackToSavepoint(ctx context.Context, name string) error {
	if d.failNextSavepointRollback {
		d.failNextSavepointRollback = false
		return errors.New("savepoint rollback: connection reset by peer")
	}
	return nil
}

func (d *mockTxDriver) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	if d.txDead() {
		return nil, errors.New("sql: transaction has already been committed or rolled back")
	}
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

// TestManualMode_BeginTxOutlivesTriggeringRequest reproduces a production bug:
// a manual-mode transaction is begun during one HTTP request and used again
// during a later, separate request. If BeginTx is called with that first
// request's context, database/sql ties the *sql.Tx's lifetime to it and rolls
// it back in the background the moment the request's context is canceled
// (net/http cancels r.Context() once the handler returns) — but the driver's
// own bookkeeping (currentTx/InTransaction) has no way to observe that
// out-of-band rollback, so the next statement wrongly reuses the dead
// transaction instead of beginning a new one.
func TestManualMode_BeginTxOutlivesTriggeringRequest(t *testing.T) {
	s, _ := newTxSession(t)
	_ = s.SetTransactionMode(context.Background(), TxModeManual)

	requestCtx, cancel := context.WithCancel(context.Background())
	if _, err := s.Execute(requestCtx, "UPDATE customer SET name = 'x'"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	cancel() // the HTTP request that began the transaction has completed

	nextRequestCtx := context.Background()
	if _, err := s.Query(nextRequestCtx, "SELECT 1"); err != nil {
		t.Fatalf("Query in a later request on the same still-open transaction: %v", err)
	}
}

func TestManualMode_TransactionStatusListsStatementText(t *testing.T) {
	s, _ := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)

	if _, err := s.Execute(ctx, "UPDATE customer SET name = 'x'"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := s.Query(ctx, "SELECT 1"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	status := s.TransactionStatus()
	want := []string{"UPDATE customer SET name = 'x'", "SELECT 1"}
	if !slices.Equal(status.Statements, want) {
		t.Fatalf("Statements = %v, want %v", status.Statements, want)
	}

	if err := s.CommitTransaction(ctx); err != nil {
		t.Fatalf("CommitTransaction: %v", err)
	}
	if statements := s.TransactionStatus().Statements; len(statements) != 0 {
		t.Fatalf("Statements after commit = %v, want empty", statements)
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

// TestManualMode_SavepointRollbackFailureFullyClosesTransaction covers a
// harsher variant of the connection-death bug: the connection dies *during*
// per-statement savepoint recovery, not at Commit/Rollback time. Without
// discarding the transaction here, the session would report Open=true
// indefinitely — a plain Rollback/Commit later would fail the same way a
// savepoint operation just did, and the user would have no path out.
func TestManualMode_SavepointRollbackFailureFullyClosesTransaction(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	driver.failNextExec = true
	driver.failNextSavepointRollback = true
	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (bad)"); err == nil {
		t.Fatal("expected statement error")
	}

	if driver.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1 (full rollback after savepoint recovery failed)", driver.rollbacks)
	}
	status := s.TransactionStatus()
	if status.Open {
		t.Fatal("transaction must not report open after savepoint recovery itself failed")
	}
	if status.PendingStatements != 0 || len(status.Statements) != 0 {
		t.Fatalf("status after failed savepoint recovery = %+v, want 0 pending statements", status)
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

// TestCommitTransaction_ConnectionDeath reproduces the production report: the
// underlying connection dies while a manual transaction is open, and the user
// clicks Commit. The driver-level Commit unsets inTx even though it errored
// (matching real drivers), so the session must not keep reporting the
// transaction as open/pending afterward — otherwise the UI is stuck showing
// Commit/Rollback for a transaction the database already discarded, with no
// way to escape.
func TestCommitTransaction_ConnectionDeath(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	_, _ = s.Execute(ctx, "UPDATE customer SET name = 'x'")

	driver.failNextCommit = true
	if err := s.CommitTransaction(ctx); err == nil {
		t.Fatal("expected CommitTransaction to surface the driver error")
	}

	status := s.TransactionStatus()
	if status.Open {
		t.Fatal("transaction must not report open after the driver discarded it, even on commit error")
	}
	if status.PendingStatements != 0 || len(status.Statements) != 0 {
		t.Fatalf("status after failed commit = %+v, want 0 pending statements", status)
	}
}

// TestRollbackTransaction_ConnectionDeath is the rollback counterpart: a dead
// connection fails the rollback call itself, and the session state must still
// resolve to closed rather than leaving the user stuck.
func TestRollbackTransaction_ConnectionDeath(t *testing.T) {
	s, driver := newTxSession(t)
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)
	_, _ = s.Execute(ctx, "DELETE FROM customer WHERE id = 1")

	driver.failNextRollback = true
	if err := s.RollbackTransaction(ctx); err == nil {
		t.Fatal("expected RollbackTransaction to surface the driver error")
	}

	status := s.TransactionStatus()
	if status.Open {
		t.Fatal("transaction must not report open after the driver discarded it, even on rollback error")
	}
	if status.PendingStatements != 0 || len(status.Statements) != 0 {
		t.Fatalf("status after failed rollback = %+v, want 0 pending statements", status)
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

// mockTxCursorDriver is a savepoint-capable driver that also supports query
// cursors, exercising the cursor path's transaction participation.
type mockTxCursorDriver struct {
	mockTxDriver
	cursors        []*mockQueryCursor
	failNextCursor bool
}

func (d *mockTxCursorDriver) StartQuery(context.Context, cursor.QueryRequest) (cursor.QueryCursor, error) {
	if d.failNextCursor {
		d.failNextCursor = false
		return nil, errors.New("select failed")
	}
	c := &mockQueryCursor{}
	d.cursors = append(d.cursors, c)
	return c, nil
}

var _ cursor.QueryCursorDriver = (*mockTxCursorDriver)(nil)

func newTxCursorSession() (*Session, *mockTxCursorDriver) {
	d := &mockTxCursorDriver{}
	return &Session{ID: "s1", AccountID: "a1", ConnectionID: "c1", Conn: d}, d
}

func TestManualMode_StartQueryCursorLazilyBeginsTransaction(t *testing.T) {
	s, driver := newTxCursorSession()
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)

	if _, err := s.StartQueryCursor(ctx, "SELECT 1"); err != nil {
		t.Fatalf("StartQueryCursor: %v", err)
	}
	if !driver.inTx {
		t.Fatal("cursor-backed SELECT did not lazily begin a transaction in manual mode")
	}
	status := s.TransactionStatus()
	if !status.Open || status.PendingStatements != 1 {
		t.Fatalf("status = %+v, want open with 1 pending statement", status)
	}
	if len(driver.savepoints) != 1 {
		t.Fatalf("savepoints = %d, want 1", len(driver.savepoints))
	}
}

func TestManualMode_FailingCursorRollsBackToSavepointOnly(t *testing.T) {
	s, driver := newTxCursorSession()
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)

	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	driver.failNextCursor = true
	if _, err := s.StartQueryCursor(ctx, "SELECT bad"); err == nil {
		t.Fatal("expected cursor open to fail")
	}
	if !s.TransactionStatus().Open {
		t.Fatal("transaction should stay open after a savepoint-recovered cursor failure")
	}
	if driver.rollbacks != 0 {
		t.Fatalf("rollbacks = %d, want 0 (only the savepoint should roll back)", driver.rollbacks)
	}
	if len(driver.savepoints) != 2 {
		t.Fatalf("savepoints = %d, want 2 (one per statement)", len(driver.savepoints))
	}
}

func TestManualMode_NewStatementClosesOpenCursors(t *testing.T) {
	s, driver := newTxCursorSession()
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)

	if _, err := s.StartQueryCursor(ctx, "SELECT 1"); err != nil {
		t.Fatalf("StartQueryCursor: %v", err)
	}
	if _, err := s.Execute(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !driver.cursors[0].closed {
		t.Fatal("expected the open cursor to be closed before the next manual-mode statement")
	}
	if len(s.cursors) != 0 {
		t.Fatalf("tracked cursors = %d, want 0", len(s.cursors))
	}
}

// TestManager_ReapIdle_RollsBackOpenManualTransaction covers the case where a
// user opens a manual transaction and then walks away (or the tab is closed
// without a clean disconnect): the idle reaper must still roll back and tear
// down the session rather than leaving an orphaned open transaction holding
// locks on the target database indefinitely.
func TestManager_ReapIdle_RollsBackOpenManualTransaction(t *testing.T) {
	m := New(50 * time.Millisecond)
	defer m.Close()

	driver := &mockTxDriver{}
	open := func() (engine.Driver, error) { return driver, nil }

	sess, _, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	_ = sess.SetTransactionMode(context.Background(), TxModeManual)
	if _, err := sess.Execute(context.Background(), "UPDATE customer SET name = 'x'"); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	m.reapIdle()

	if driver.rollbacks != 1 {
		t.Fatalf("rollbacks after idle reap = %d, want 1", driver.rollbacks)
	}
	if !driver.closed {
		t.Fatal("expected driver to be closed after idle reap")
	}
	if _, ok := m.Get(sess.ID); ok {
		t.Fatal("expected session to be gone after idle reap")
	}
}

// TestConcurrentCursorAndCommit exercises the cursor-open path against
// commit/rollback so `go test -race` catches unsynchronized access to the
// driver's transaction state if the session lock regresses.
func TestConcurrentCursorAndCommit(t *testing.T) {
	s, _ := newTxCursorSession()
	ctx := context.Background()
	_ = s.SetTransactionMode(ctx, TxModeManual)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = s.StartQueryCursor(ctx, "SELECT 1")
		}()
		go func() {
			defer wg.Done()
			_ = s.CommitTransaction(ctx)
		}()
	}
	wg.Wait()
}
