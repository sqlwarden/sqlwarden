package executiontest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
)

// Factory constructs a fresh runtime. Implementations register cleanup
// through t before returning.
type Factory func(t testing.TB) execution.Runtime

// Fixture describes what the runtime under test has been seeded with.
type Fixture struct {
	// Scope resolves to a connection to a fresh, empty, writable database
	// that supports cursors, manual transactions, and metadata listing.
	// ConnectionID and AccountID are free-form: the suite derives isolated
	// databases by varying ConnectionID and further principals by varying
	// AccountID, so every derived connection must resolve to its own empty
	// database.
	//
	// The suite issues SQLite-compatible SQL (INTEGER PRIMARY KEY, TEXT),
	// so the seeded database must accept it.
	Scope execution.Scope
}

func (fx Fixture) isolated(suffix string) execution.Scope {
	s := fx.Scope
	s.ConnectionID += "-" + suffix
	return s
}

func intruderOf(s execution.Scope) execution.Scope {
	s.AccountID += "-intruder"
	return s
}

// RunRuntimeContract runs the runtime contract suite. Each subtest uses its
// own connection, and so its own database and session, and may run only
// against a runtime that no other test shares.
func RunRuntimeContract(t *testing.T, factory Factory, fx Fixture) {
	rt := factory(t)

	t.Run("open reuses the session", func(t *testing.T) {
		scope := fx.isolated("open")
		scope.AccountID += "-open"
		testOpenReuse(t, rt, scope)
	})
	t.Run("manual transactions", func(t *testing.T) { testTransactions(t, rt, fx.isolated("tx")) })
	t.Run("cursor paging", func(t *testing.T) { testCursorPaging(t, rt, fx.isolated("cursor")) })
	t.Run("metadata survives a json round trip", func(t *testing.T) { testMetadata(t, rt, fx.isolated("meta")) })
	t.Run("foreign scope is indistinguishable from unknown id", func(t *testing.T) {
		owner := fx.isolated("owner")
		testScopeOpacity(t, rt, owner, intruderOf(owner))
	})
	t.Run("cancelled commit is outcome unknown", func(t *testing.T) { testCancelledCommit(t, rt, fx.isolated("commit")) })
	t.Run("cancelled execute is outcome unknown", func(t *testing.T) { testCancelledExecute(t, rt, fx.isolated("exec")) })
	t.Run("cancel ends the session", func(t *testing.T) { testCancel(t, rt, fx.isolated("cancel")) })
}

func open(t *testing.T, rt execution.Runtime, scope execution.Scope) execution.SessionInfo {
	t.Helper()
	info, err := rt.Open(context.Background(), execution.OpenRequest{Scope: scope})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return info
}

func exec(t *testing.T, rt execution.Runtime, scope execution.Scope, id execution.SessionID, sql string) {
	t.Helper()
	if _, err := rt.Execute(context.Background(), scope, execution.ExecuteRequest{SessionID: id, SQL: sql}); err != nil {
		t.Fatalf("Execute(%q): %v", sql, err)
	}
}

func requireCode(t *testing.T, err error, want execution.FailureCode) *execution.Failure {
	t.Helper()
	var f *execution.Failure
	if !errors.As(err, &f) || f.Code != want {
		t.Fatalf("error = %v, want failure %q", err, want)
	}
	return f
}

func requireSameFailure(t *testing.T, what string, a, b error, want execution.FailureCode) {
	t.Helper()
	fa := requireCode(t, a, want)
	fb := requireCode(t, b, want)
	if fa.Retryable != fb.Retryable || a.Error() != b.Error() {
		t.Fatalf("%s differ: %q (retryable=%v) vs %q (retryable=%v)", what, a, fa.Retryable, b, fb.Retryable)
	}
}

func rowCount(t *testing.T, rt execution.Runtime, scope execution.Scope, id execution.SessionID) int {
	t.Helper()
	res, err := rt.Query(context.Background(), scope, execution.QueryRequest{SessionID: id, SQL: "SELECT id FROM widgets"})
	if err != nil || res.Result == nil {
		t.Fatalf("Query: %+v, %v", res, err)
	}
	return len(res.Result.Rows)
}

func testOpenReuse(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	first, err := rt.Open(ctx, execution.OpenRequest{
		Scope:  scope,
		Limits: execution.Limits{MaxRows: 100, MaxBytes: 1 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Reused || first.Scope != scope || first.Driver == "" {
		t.Fatalf("Open() = %+v", first)
	}
	second := open(t, rt, scope)
	if second.ID != first.ID || !second.Reused {
		t.Fatalf("second Open() = %+v, want reuse of %q", second, first.ID)
	}
	got, err := rt.Get(ctx, scope, first.ID)
	if err != nil || got.ID != first.ID || got.Scope != scope {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	listed, err := rt.List(ctx, execution.SessionFilter{AccountID: scope.AccountID, WorkspaceID: scope.WorkspaceID})
	if err != nil || len(listed) != 1 || listed[0].ID != first.ID {
		t.Fatalf("List() = %+v, %v", listed, err)
	}

	ephemeral, err := rt.Open(ctx, execution.OpenRequest{Scope: scope, Ephemeral: true})
	if err != nil || ephemeral.ID == first.ID || ephemeral.Reused {
		t.Fatalf("ephemeral Open() = %+v, %v", ephemeral, err)
	}

	if err := rt.Close(ctx, scope, first.ID); err != nil {
		t.Fatal(err)
	}
	_, err = rt.Get(ctx, scope, first.ID)
	requireCode(t, err, execution.FailureSessionNotFound)
	if err := rt.Close(ctx, scope, first.ID); err != nil {
		t.Fatalf("closing a closed session must be a no-op: %v", err)
	}
	if err := rt.Close(ctx, scope, ephemeral.ID); err != nil {
		t.Fatal(err)
	}
}

func testTransactions(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	info := open(t, rt, scope)
	id := info.ID
	exec(t, rt, scope, id, "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)")

	closed, err := rt.Status(ctx, scope, id)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Mode != execution.TxModeAuto || closed.Open || closed.PendingStatements != 0 || closed.Statements == nil || len(closed.Statements) != 0 {
		t.Fatalf("initial Status() = %+v, want auto mode with a non-nil empty statement list", closed)
	}

	manual, err := rt.SetMode(ctx, scope, id, execution.TxModeManual)
	if err != nil || manual.Mode != execution.TxModeManual || manual.Open || manual.Statements == nil {
		t.Fatalf("SetMode(manual) = %+v, %v", manual, err)
	}

	pending := "INSERT INTO widgets(name) VALUES ('pending')"
	exec(t, rt, scope, id, pending)
	open1, err := rt.Status(ctx, scope, id)
	if err != nil {
		t.Fatal(err)
	}
	if !open1.Open || open1.PendingStatements != 1 || len(open1.Statements) != 1 || open1.Statements[0] != pending {
		t.Fatalf("open Status() = %+v, want one pending statement %q", open1, pending)
	}

	rolled, err := rt.Rollback(ctx, scope, id)
	if err != nil || rolled.Open || rolled.PendingStatements != 0 || rolled.Statements == nil || len(rolled.Statements) != 0 {
		t.Fatalf("Rollback() = %+v, %v", rolled, err)
	}
	if n := rowCount(t, rt, scope, id); n != 0 {
		t.Fatalf("rows after rollback = %d, want 0", n)
	}

	exec(t, rt, scope, id, "INSERT INTO widgets(name) VALUES ('committed')")
	committed, err := rt.Commit(ctx, scope, id)
	if err != nil || committed.Open || committed.Statements == nil {
		t.Fatalf("Commit() = %+v, %v", committed, err)
	}
	if n := rowCount(t, rt, scope, id); n != 1 {
		t.Fatalf("rows after commit = %d, want 1", n)
	}
	if _, err := rt.Rollback(ctx, scope, id); err != nil {
		t.Fatal(err)
	}

	_, err = rt.SetMode(ctx, scope, id, execution.TxMode("bogus"))
	if !errors.Is(err, execution.ErrInvalidTxMode) {
		t.Fatalf("SetMode(bogus) error = %v, want ErrInvalidTxMode", err)
	}
	auto, err := rt.SetMode(ctx, scope, id, execution.TxModeAuto)
	if err != nil || auto.Mode != execution.TxModeAuto {
		t.Fatalf("SetMode(auto) = %+v, %v", auto, err)
	}
	exec(t, rt, scope, id, "INSERT INTO widgets(name) VALUES ('auto')")
	status, err := rt.Status(ctx, scope, id)
	if err != nil || status.Open || status.PendingStatements != 0 {
		t.Fatalf("Status() after an auto-mode statement = %+v, %v, want nothing pending", status, err)
	}
	if n := rowCount(t, rt, scope, id); n != 2 {
		t.Fatalf("rows after auto-mode insert = %d, want 2", n)
	}
}

func testCursorPaging(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	id := open(t, rt, scope).ID
	exec(t, rt, scope, id, "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)")
	exec(t, rt, scope, id, "INSERT INTO widgets(name) VALUES ('a')")
	exec(t, rt, scope, id, "INSERT INTO widgets(name) VALUES ('b')")

	page := execution.Limits{MaxRows: 1, MaxBytes: 1 << 20}
	query, err := rt.Query(ctx, scope, execution.QueryRequest{
		SessionID: id, SQL: "SELECT id, name FROM widgets ORDER BY id", UseCursor: true, PageSize: 1, Limits: page,
	})
	if err != nil {
		t.Fatal(err)
	}
	if query.Result == nil || query.Exhausted || query.CursorID == "" || len(query.Result.Rows) != 1 {
		t.Fatalf("Query() = %+v, want first page and an opaque cursor", query)
	}
	fetch := func() execution.QueryResult {
		t.Helper()
		out, err := rt.Fetch(ctx, scope, execution.FetchRequest{SessionID: id, CursorID: query.CursorID, PageSize: 1, Limits: page})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	second := fetch()
	if second.Result == nil || second.Exhausted || len(second.Result.Rows) != 1 {
		t.Fatalf("Fetch() = %+v, want second page", second)
	}
	last := fetch()
	if last.Result == nil || !last.Exhausted || len(last.Result.Rows) != 0 {
		t.Fatalf("final Fetch() = %+v, want exhausted empty page", last)
	}

	if err := rt.CloseCursor(ctx, scope, id, query.CursorID); err != nil {
		t.Fatalf("CloseCursor() of an exhausted cursor: %v", err)
	}
	if err := rt.CloseCursor(ctx, scope, id, query.CursorID); err != nil {
		t.Fatalf("repeated CloseCursor(): %v", err)
	}
	_, err = rt.Fetch(ctx, scope, execution.FetchRequest{SessionID: id, CursorID: query.CursorID, PageSize: 1, Limits: page})
	requireCode(t, err, execution.FailureCursorLost)
}

func testMetadata(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	id := open(t, rt, scope).ID

	caps, err := rt.Capabilities(ctx, scope, id)
	if err != nil || caps.Schema == nil {
		t.Fatalf("Capabilities() = %+v, %v", caps, err)
	}
	roots, err := rt.LoadChildren(ctx, scope, id, execution.ChildrenRequest{
		FolderKind: "databases",
		Parents:    []metadata.ScopePath{""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots[""]) == 0 {
		t.Fatalf("LoadChildren() = %+v, want at least one root database", roots)
	}
	for _, c := range roots[""] {
		if c.Name == "" {
			t.Fatalf("child without a name: %+v", c)
		}
	}

	encoded, err := json.Marshal(roots)
	if err != nil {
		t.Fatalf("LoadChildren() result is not serialisable: %v", err)
	}
	var decoded map[metadata.ScopePath][]metadata.Child
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded[""]) != len(roots[""]) {
		t.Fatalf("round trip changed children: %+v -> %+v", roots, decoded)
	}
	for i, c := range roots[""] {
		if decoded[""][i].Name != c.Name {
			t.Fatalf("round trip changed child %d: %q -> %q", i, c.Name, decoded[""][i].Name)
		}
	}
}

func testScopeOpacity(t *testing.T, rt execution.Runtime, owner, intruder execution.Scope) {
	ctx := context.Background()
	id := open(t, rt, owner).ID
	exec(t, rt, owner, id, "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)")
	exec(t, rt, owner, id, "INSERT INTO widgets(name) VALUES ('a')")
	exec(t, rt, owner, id, "INSERT INTO widgets(name) VALUES ('b')")

	page := execution.Limits{MaxRows: 1, MaxBytes: 1 << 20}
	query, err := rt.Query(ctx, owner, execution.QueryRequest{
		SessionID: id, SQL: "SELECT id FROM widgets ORDER BY id", UseCursor: true, PageSize: 1, Limits: page,
	})
	if err != nil || query.CursorID == "" {
		t.Fatalf("Query() = %+v, %v", query, err)
	}
	const unknown = execution.SessionID("no-such-session")

	_, err = rt.Get(ctx, intruder, id)
	_, errUnknown := rt.Get(ctx, intruder, unknown)
	requireSameFailure(t, "Get", err, errUnknown, execution.FailureSessionNotFound)

	_, err = rt.Status(ctx, intruder, id)
	_, errUnknown = rt.Status(ctx, intruder, unknown)
	requireSameFailure(t, "Status", err, errUnknown, execution.FailureSessionNotFound)

	_, err = rt.SetMode(ctx, intruder, id, execution.TxModeManual)
	_, errUnknown = rt.SetMode(ctx, intruder, unknown, execution.TxModeManual)
	requireSameFailure(t, "SetMode", err, errUnknown, execution.FailureSessionNotFound)

	_, err = rt.Query(ctx, intruder, execution.QueryRequest{SessionID: id, SQL: "SELECT 1"})
	_, errUnknown = rt.Query(ctx, intruder, execution.QueryRequest{SessionID: unknown, SQL: "SELECT 1"})
	requireSameFailure(t, "Query", err, errUnknown, execution.FailureSessionNotFound)

	_, err = rt.Execute(ctx, intruder, execution.ExecuteRequest{SessionID: id, SQL: "DELETE FROM widgets"})
	_, errUnknown = rt.Execute(ctx, intruder, execution.ExecuteRequest{SessionID: unknown, SQL: "DELETE FROM widgets"})
	requireSameFailure(t, "Execute", err, errUnknown, execution.FailureSessionNotFound)

	_, err = rt.Commit(ctx, intruder, id)
	_, errUnknown = rt.Commit(ctx, intruder, unknown)
	requireSameFailure(t, "Commit", err, errUnknown, execution.FailureTransactionLost)
	if !errors.Is(err, execution.ErrTransactionLost) {
		t.Fatalf("foreign Commit() = %v, want ErrTransactionLost", err)
	}

	_, err = rt.Rollback(ctx, intruder, id)
	_, errUnknown = rt.Rollback(ctx, intruder, unknown)
	requireSameFailure(t, "Rollback", err, errUnknown, execution.FailureTransactionLost)

	fetchReq := func(session execution.SessionID, cursor execution.CursorID) execution.FetchRequest {
		return execution.FetchRequest{SessionID: session, CursorID: cursor, PageSize: 1, Limits: page}
	}
	_, err = rt.Fetch(ctx, intruder, fetchReq(id, query.CursorID))
	_, errUnknown = rt.Fetch(ctx, intruder, fetchReq(unknown, "no-such-cursor"))
	requireSameFailure(t, "Fetch with foreign session and cursor", err, errUnknown, execution.FailureSessionNotFound)

	intruderID := open(t, rt, intruder).ID
	_, err = rt.Fetch(ctx, intruder, fetchReq(intruderID, query.CursorID))
	_, errUnknown = rt.Fetch(ctx, intruder, fetchReq(intruderID, "no-such-cursor"))
	requireSameFailure(t, "Fetch with foreign cursor on own session", err, errUnknown, execution.FailureCursorLost)

	if err := rt.CloseCursor(ctx, intruder, id, query.CursorID); err != nil {
		t.Fatalf("foreign CloseCursor() = %v, want silent no-op", err)
	}
	if err := rt.Close(ctx, intruder, id); err != nil {
		t.Fatalf("foreign Close() = %v, want silent no-op", err)
	}

	next, err := rt.Fetch(ctx, owner, fetchReq(id, query.CursorID))
	if err != nil || next.Result == nil || len(next.Result.Rows) != 1 {
		t.Fatalf("owner Fetch() after foreign attempts = %+v, %v", next, err)
	}
	if n := rowCount(t, rt, owner, id); n != 2 {
		t.Fatalf("owner rows = %d, want 2", n)
	}
}

func testCancelledCommit(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	id := open(t, rt, scope).ID
	exec(t, rt, scope, id, "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)")
	if _, err := rt.SetMode(ctx, scope, id, execution.TxModeManual); err != nil {
		t.Fatal(err)
	}
	exec(t, rt, scope, id, "INSERT INTO widgets(name) VALUES ('uncertain')")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := rt.Commit(cancelled, scope, id)
	requireCode(t, err, execution.FailureExecutionOutcomeUnknown)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, execution.ErrOutcomeUnknown) {
		t.Fatalf("cancelled Commit() error = %v, want outcome unknown wrapping context cancellation", err)
	}

	status, err := rt.Status(ctx, scope, id)
	if err != nil || !status.Open || status.PendingStatements != 1 {
		t.Fatalf("Status() after cancelled Commit = %+v, %v, want the transaction still open with one pending statement", status, err)
	}
	if _, err := rt.Rollback(ctx, scope, id); err != nil {
		t.Fatalf("Rollback() after cancelled Commit: %v", err)
	}
	if n := rowCount(t, rt, scope, id); n != 0 {
		t.Fatalf("rows after rollback of a cancelled Commit = %d, want 0", n)
	}
}

func testCancelledExecute(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	id := open(t, rt, scope).ID
	exec(t, rt, scope, id, "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := rt.Execute(cancelled, scope, execution.ExecuteRequest{SessionID: id, SQL: "INSERT INTO widgets(name) VALUES ('uncertain')"})
	requireCode(t, err, execution.FailureExecutionOutcomeUnknown)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, execution.ErrOutcomeUnknown) {
		t.Fatalf("cancelled Execute() error = %v, want outcome unknown wrapping context cancellation", err)
	}
}

func testCancel(t *testing.T, rt execution.Runtime, scope execution.Scope) {
	ctx := context.Background()
	id := open(t, rt, scope).ID
	if _, err := rt.SetMode(ctx, scope, id, execution.TxModeManual); err != nil {
		t.Fatal(err)
	}

	if err := rt.Cancel(ctx, scope, id); err != nil {
		t.Fatal(err)
	}
	_, err := rt.Query(ctx, scope, execution.QueryRequest{SessionID: id, SQL: "SELECT 1"})
	requireCode(t, err, execution.FailureSessionNotFound)
	_, err = rt.Status(ctx, scope, id)
	requireCode(t, err, execution.FailureSessionNotFound)
	_, err = rt.Commit(ctx, scope, id)
	requireCode(t, err, execution.FailureTransactionLost)
	if !errors.Is(err, execution.ErrTransactionLost) {
		t.Fatalf("Commit() after Cancel = %v, want ErrTransactionLost", err)
	}
	if err := rt.Cancel(ctx, scope, id); err != nil {
		t.Fatalf("repeated Cancel(): %v", err)
	}
}
