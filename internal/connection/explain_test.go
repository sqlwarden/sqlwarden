package connection

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/pkg/result"
)

// explainMockDriver records call order and can fail the Statement query, to
// verify RunPinned/ExecuteExplainPlan's pinning and teardown guarantees.
type explainMockDriver struct {
	mockTxDriver
	calls     []string
	failQuery bool
}

func (d *explainMockDriver) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	d.calls = append(d.calls, "exec:"+sql)
	return d.mockTxDriver.Execute(ctx, sql, args...)
}

func (d *explainMockDriver) Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	d.calls = append(d.calls, "query:"+sql)
	if d.failQuery {
		return nil, errors.New("statement failed")
	}
	return &result.ResultSet{}, nil
}

func newExplainSession() (*Session, *explainMockDriver) {
	d := &explainMockDriver{}
	return &Session{ID: "s1", AccountID: "a1", ConnectionID: "c1", Conn: d}, d
}

// TestExecuteExplainPlan_PinsToOneTransaction reproduces the bug this exists
// to fix: without pinning, Setup/Statement/Teardown could land on different
// pooled connections, so a session-scoped toggle Setup sets (e.g. SQL
// Server's SET SHOWPLAN_XML ON) would have no effect on Statement. Pinning
// via a driver transaction guarantees they share a connection.
func TestExecuteExplainPlan_PinsToOneTransaction(t *testing.T) {
	s, driver := newExplainSession()
	plan := explain.Plan{
		Setup:     []string{"SET SHOWPLAN_XML ON"},
		Statement: "SELECT 1",
		Teardown:  []string{"SET SHOWPLAN_XML OFF"},
	}

	rs, err := s.ExecuteExplainPlan(context.Background(), plan, cursor.ScanOptions{})
	if err != nil {
		t.Fatalf("ExecuteExplainPlan: %v", err)
	}
	if rs == nil {
		t.Fatal("expected a result set")
	}
	want := []string{"exec:SET SHOWPLAN_XML ON", "query:SELECT 1", "exec:SET SHOWPLAN_XML OFF"}
	if len(driver.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", driver.calls, want)
	}
	for i, c := range want {
		if driver.calls[i] != c {
			t.Fatalf("calls = %v, want %v", driver.calls, want)
		}
	}
	if driver.commits != 0 || driver.rollbacks != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want 0 commits and 1 rollback (explain never persists)", driver.commits, driver.rollbacks)
	}
}

// TestExecuteExplainPlan_TeardownRunsOnStatementFailure ensures a
// session-scoped toggle set by Setup is never left on for later statements
// on the same connection, even when Statement itself fails.
func TestExecuteExplainPlan_TeardownRunsOnStatementFailure(t *testing.T) {
	s, driver := newExplainSession()
	driver.failQuery = true
	plan := explain.Plan{
		Setup:     []string{"SET SHOWPLAN_XML ON"},
		Statement: "SELECT bad",
		Teardown:  []string{"SET SHOWPLAN_XML OFF"},
	}

	if _, err := s.ExecuteExplainPlan(context.Background(), plan, cursor.ScanOptions{}); err == nil {
		t.Fatal("expected the statement failure to surface")
	}
	want := []string{"exec:SET SHOWPLAN_XML ON", "query:SELECT bad", "exec:SET SHOWPLAN_XML OFF"}
	if len(driver.calls) != len(want) {
		t.Fatalf("calls = %v, want %v (teardown must still run)", driver.calls, want)
	}
	for i, c := range want {
		if driver.calls[i] != c {
			t.Fatalf("calls = %v, want %v", driver.calls, want)
		}
	}
}

// TestExecuteExplainPlan_ReusesOpenManualTransaction covers a user who
// already has a manual transaction open: RunPinned must not begin (or roll
// back) a second, wrapper transaction — that would silently discard the
// user's real transaction. Statements already share one pinned connection
// via the open transaction, so ExecuteExplainPlan should just run directly
// against it.
func TestExecuteExplainPlan_ReusesOpenManualTransaction(t *testing.T) {
	s, driver := newExplainSession()
	_ = s.SetTransactionMode(context.Background(), TxModeManual)
	if _, err := s.Execute(context.Background(), "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	plan := explain.Plan{
		Setup:     []string{"SET SHOWPLAN_XML ON"},
		Statement: "SELECT 1",
		Teardown:  []string{"SET SHOWPLAN_XML OFF"},
	}
	if _, err := s.ExecuteExplainPlan(context.Background(), plan, cursor.ScanOptions{}); err != nil {
		t.Fatalf("ExecuteExplainPlan: %v", err)
	}

	if driver.rollbacks != 0 {
		t.Fatalf("rollbacks = %d, want 0 (must not touch the user's own open transaction)", driver.rollbacks)
	}
	if !driver.InTransaction() {
		t.Fatal("the user's manual transaction must still be open after ExecuteExplainPlan")
	}
}
