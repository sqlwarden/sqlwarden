// Package executiontest contains runtime contract tests shared by local and
// remote execution adapters.
package executiontest

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
	"github.com/sqlwarden/internal/execution"
)

// Factory constructs a fresh runtime and its observable session directory.
// Implementations register cleanup through t before returning.
type Factory func(t testing.TB) (execution.SessionRuntime, execution.SessionDirectory)

// RunRuntimeContract verifies the stable behavior every execution adapter must
// preserve across in-process and RPC transports.
func RunRuntimeContract(t *testing.T, factory Factory) {
	t.Helper()
	runtime, directory := factory(t)
	ctx := context.Background()
	scope := execution.Scope{TenantID: "11", AccountID: "22", WorkspaceID: "33", ConnectionID: "44"}

	opened, err := runtime.Open(ctx, execution.OpenRequest{
		Scope: scope,
		Target: execution.Target{
			Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "runtime.db"),
			Limits: execution.Limits{MaxRows: 100, MaxBytes: 1 << 20},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Handle == "" || opened.Reused {
		t.Fatalf("Open() = %+v", opened)
	}
	openedAgain, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope, Target: execution.Target{Driver: "sqlite"}})
	if err != nil {
		t.Fatal(err)
	}
	if openedAgain.Handle != opened.Handle || !openedAgain.Reused {
		t.Fatalf("second Open() = %+v, want reused %q", openedAgain, opened.Handle)
	}

	if _, err := runtime.Execute(ctx, execution.ExecuteRequest{Handle: opened.Handle, SQL: "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SetTransactionMode(ctx, execution.SessionRequest{Handle: opened.Handle}, execution.TransactionModeManual); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(ctx, execution.ExecuteRequest{Handle: opened.Handle, SQL: "INSERT INTO widgets(name) VALUES ('pending')"}); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.TransactionStatus(ctx, execution.SessionRequest{Handle: opened.Handle})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Open || status.PendingStatements != 1 {
		t.Fatalf("TransactionStatus() = %+v", status)
	}
	if _, err := runtime.Rollback(ctx, execution.SessionRequest{Handle: opened.Handle}); err != nil {
		t.Fatal(err)
	}
	status, err = runtime.TransactionStatus(ctx, execution.SessionRequest{Handle: opened.Handle})
	if err != nil {
		t.Fatal(err)
	}
	if status.Open || status.PendingStatements != 0 {
		t.Fatalf("status after rollback = %+v", status)
	}
	if _, err := runtime.Execute(ctx, execution.ExecuteRequest{Handle: opened.Handle, SQL: "INSERT INTO widgets(name) VALUES ('committed')"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Commit(ctx, execution.SessionRequest{Handle: opened.Handle}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SetTransactionMode(ctx, execution.SessionRequest{Handle: opened.Handle}, execution.TransactionModeAuto); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(ctx, execution.ExecuteRequest{Handle: opened.Handle, SQL: "INSERT INTO widgets(name) VALUES ('second')"}); err != nil {
		t.Fatal(err)
	}

	query, err := runtime.Query(ctx, execution.QueryRequest{
		Handle: opened.Handle, SQL: "SELECT id, name FROM widgets ORDER BY id", UseCursor: true, PageSize: 1,
		Limits: execution.Limits{MaxRows: 1, MaxBytes: 1 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if query.Result == nil || query.Exhausted || query.Cursor == "" || len(query.Result.Rows) != 1 {
		t.Fatalf("Query() = %+v, want first page and an opaque cursor handle", query)
	}
	fetched, err := runtime.Fetch(ctx, execution.FetchRequest{
		Handle: opened.Handle, Cursor: query.Cursor, PageSize: 1,
		Limits: execution.Limits{MaxRows: 1, MaxBytes: 1 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Result == nil || fetched.Exhausted || len(fetched.Result.Rows) != 1 {
		t.Fatalf("Fetch() = %+v, want second page", fetched)
	}
	fetched, err = runtime.Fetch(ctx, execution.FetchRequest{
		Handle: opened.Handle, Cursor: query.Cursor, PageSize: 1,
		Limits: execution.Limits{MaxRows: 1, MaxBytes: 1 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Result == nil || !fetched.Exhausted || len(fetched.Result.Rows) != 0 {
		t.Fatalf("final Fetch() = %+v, want exhausted empty page", fetched)
	}
	if err := runtime.Close(ctx, execution.CloseRequest{Handle: opened.Handle, Cursor: query.Cursor}); err != nil {
		t.Fatalf("idempotent cursor Close() error = %v", err)
	}

	info, found, err := runtime.Session(ctx, opened.Handle)
	if err != nil || !found || info.Scope != scope {
		t.Fatalf("Session() = (%+v, %v, %v)", info, found, err)
	}
	record, found, err := directory.Get(ctx, opened.Handle)
	if err != nil || !found || record.Scope != scope || record.OwnerRuntimeID == "" || record.RoutingAddress == "" {
		t.Fatalf("directory Get() = (%+v, %v, %v)", record, found, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = runtime.Execute(cancelled, execution.ExecuteRequest{Handle: opened.Handle, SQL: "INSERT INTO widgets(name) VALUES ('uncertain')"})
	var failure *execution.Failure
	if !errors.As(err, &failure) || failure.Code != execution.FailureExecutionOutcomeUnknown || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Execute() error = %v, want execution_outcome_unknown wrapping context cancellation", err)
	}

	if err := runtime.Cancel(ctx, execution.SessionRequest{Handle: opened.Handle}); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Query(ctx, execution.QueryRequest{Handle: opened.Handle, SQL: "SELECT 1"})
	if !errors.As(err, &failure) || failure.Code != execution.FailureSessionLost {
		t.Fatalf("Query() after Cancel error = %v, want session_lost", err)
	}
	_, err = runtime.Commit(ctx, execution.SessionRequest{Handle: opened.Handle})
	if !errors.As(err, &failure) || failure.Code != execution.FailureTransactionLost || !errors.Is(err, execution.ErrTransactionLost) {
		t.Fatalf("Commit() after Cancel error = %v, want transaction_lost", err)
	}
	if _, found, err := directory.Get(ctx, opened.Handle); err != nil || found {
		t.Fatalf("directory retained cancelled handle: found=%v err=%v", found, err)
	}
}
