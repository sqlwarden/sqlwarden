package execution

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/internal/exports"
)

func seededSession(t *testing.T, r *LocalRuntime) SessionInfo {
	t.Helper()
	info := mustOpen(t, r, baseScope)
	for _, stmt := range []string{
		"create table t (id integer, name text)",
		"insert into t values (1, 'a'), (2, 'b'), (3, 'c')",
	} {
		if _, err := r.Execute(context.Background(), baseScope, ExecuteRequest{SessionID: info.ID, SQL: stmt}); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	return info
}

func requireFailure(t *testing.T, err error, code FailureCode) {
	t.Helper()
	var f *Failure
	if !errors.As(err, &f) || f.Code != code {
		t.Fatalf("want failure %s, got %v", code, err)
	}
}

func TestQueriesBufferedQuery(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)

	res, err := r.Query(context.Background(), baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t order by id"})
	if err != nil {
		t.Fatal(err)
	}
	if res.CursorID != "" || !res.Exhausted || len(res.Result.Rows) != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Transaction.Mode != TxModeAuto || res.Transaction.Open {
		t.Fatalf("unexpected transaction status: %+v", res.Transaction)
	}
}

func TestQueriesQueryForeignScope(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	_, err := r.Query(context.Background(), scopeOf("o1", "w1", "a2", "c1"), QueryRequest{SessionID: info.ID, SQL: "select 1"})
	requireNotFound(t, err)
	_, err = r.Execute(context.Background(), scopeOf("o1", "w1", "a2", "c1"), ExecuteRequest{SessionID: info.ID, SQL: "select 1"})
	requireNotFound(t, err)
}

func TestCursorPaging(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)

	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t order by id", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.CursorID == "" || first.Exhausted || len(first.Result.Rows) != 1 {
		t.Fatalf("first page: %+v", first)
	}

	second, err := r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if second.Exhausted || len(second.Result.Rows) != 1 || second.CursorID != first.CursorID {
		t.Fatalf("second page: %+v", second)
	}

	third, err := r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !third.Exhausted && len(third.Result.Rows) == 1 {
		third, err = r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !third.Exhausted || len(third.Result.Rows) != 0 {
		t.Fatalf("final page: %+v", third)
	}

	_, err = r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID})
	requireFailure(t, err, FailureCursorLost)
}

func TestCursorSurvivesCancelledCreatingContext(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)

	ctx, cancel := context.WithCancel(context.Background())
	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t order by id", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	next, err := r.Fetch(context.Background(), baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID, PageSize: 1})
	if err != nil || len(next.Result.Rows) != 1 {
		t.Fatalf("fetch after creating context cancelled: %+v, %v", next, err)
	}
}

func TestCursorLostWhenSessionClosed(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(ctx, baseScope, info.ID); err != nil {
		t.Fatal(err)
	}
	_, err = r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID})
	requireFailure(t, err, FailureCursorLost)
}

func TestCursorForeignScope(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	foreign := scopeOf("o1", "w1", "a2", "c1")
	_, err = r.Fetch(ctx, foreign, FetchRequest{SessionID: info.ID, CursorID: first.CursorID})
	requireNotFound(t, err)

	if err := r.CloseCursor(ctx, foreign, info.ID, first.CursorID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID}); err != nil {
		t.Fatalf("foreign CloseCursor must not close the cursor: %v", err)
	}
}

func TestCursorCloseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := r.CloseCursor(ctx, baseScope, info.ID, first.CursorID); err != nil {
			t.Fatalf("close %d: %v", i, err)
		}
	}
	if err := r.CloseCursor(ctx, baseScope, info.ID, "never-existed"); err != nil {
		t.Fatal(err)
	}
	_, err = r.Fetch(ctx, baseScope, FetchRequest{SessionID: info.ID, CursorID: first.CursorID})
	requireFailure(t, err, FailureCursorLost)
}

func TestCursorPageSizeCappedByRowLimit(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	res, err := r.Query(context.Background(), baseScope, QueryRequest{
		SessionID: info.ID, SQL: "select id from t", UseCursor: true, PageSize: 50, Limits: Limits{MaxRows: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Result.Rows) != 2 || res.Result.PageSize != 2 {
		t.Fatalf("page not capped: rows=%d page_size=%d", len(res.Result.Rows), res.Result.PageSize)
	}
}

func TestQueriesExecuteCancelledContextIsOutcomeUnknown(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Execute(ctx, baseScope, ExecuteRequest{SessionID: info.ID, SQL: "insert into t values (4, 'd')"})
	requireFailure(t, err, FailureExecutionOutcomeUnknown)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("error must wrap both sentinels: %v", err)
	}
	if _, err := r.Get(context.Background(), baseScope, info.ID); err == nil {
		t.Fatal("cancelled execution must discard the session")
	}
}

func TestQueriesQueryCancelledContextPassesThrough(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	var f *Failure
	if errors.As(err, &f) {
		t.Fatalf("read cancellation must not carry a failure code: %v", err)
	}
}

func TestQueriesQueryTruncatesAtRowLimit(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	res, err := r.Query(context.Background(), baseScope, QueryRequest{
		SessionID: info.ID, SQL: "select id from t order by id", Limits: Limits{MaxRows: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Result.Truncated || res.Result.TruncationReason != cursor.TruncationReasonMaxRows || len(res.Result.Rows) != 2 {
		t.Fatalf("expected row-limit truncation: %+v", res.Result)
	}
}

func TestQueriesExecuteRejectsStructuredDDL(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)
	_, err := r.Execute(context.Background(), baseScope, ExecuteRequest{SessionID: info.ID, DDL: &ddl.Request{}})
	if !errors.Is(err, ErrDDLRequiresApply) {
		t.Fatalf("err = %v", err)
	}
	_, err = r.Execute(context.Background(), baseScope, ExecuteRequest{SessionID: "unknown", DDL: &ddl.Request{}})
	if !errors.Is(err, ErrDDLRequiresApply) {
		t.Fatalf("DDL rejection must precede session lookup: %v", err)
	}
}

func TestQueriesCancelRemovesSession(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	if err := r.Cancel(ctx, baseScope, info.ID); err != nil {
		t.Fatal(err)
	}
	_, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select 1"})
	requireNotFound(t, err)
	if err := r.Cancel(ctx, baseScope, info.ID); err != nil {
		t.Fatalf("second cancel: %v", err)
	}
}

func TestStreamWritesCSV(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)

	var buf bytes.Buffer
	var progressed int64
	res, err := r.Stream(ctx, baseScope, info.ID, StreamRequest{
		SQL:        "select id, name from t order by id",
		Format:     exports.FormatCSV,
		OnProgress: func(rows, _ int64) { progressed = rows },
	}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 || strings.TrimSpace(lines[0]) != "id,name" {
		t.Fatalf("unexpected csv:\n%s", buf.String())
	}
	if res.Rows != 3 || res.Bytes != int64(buf.Len()) || progressed != 3 {
		t.Fatalf("result %+v progressed %d len %d", res, progressed, buf.Len())
	}
}

func TestStreamByteLimit(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	var buf bytes.Buffer
	_, err := r.Stream(context.Background(), baseScope, info.ID, StreamRequest{
		SQL: "select id, name from t", Format: exports.FormatCSV, Limits: Limits{MaxBytes: 5},
	}, &buf)
	requireFailure(t, err, FailureLimitExceeded)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamForeignScope(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	_, err := r.Stream(context.Background(), scopeOf("o1", "w1", "a2", "c1"), info.ID, StreamRequest{SQL: "select 1"}, &bytes.Buffer{})
	requireNotFound(t, err)
}

func TestMapError(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name     string
		err      error
		op       operation
		wantCode FailureCode
		wantIs   []error
		noFail   bool
		target   bool
	}{
		{name: "nil", err: nil, op: opQuery},
		{name: "cancel on write", err: context.Canceled, op: opExecute, wantCode: FailureExecutionOutcomeUnknown, wantIs: []error{context.Canceled, ErrOutcomeUnknown}},
		{name: "deadline on commit", err: context.DeadlineExceeded, op: opCommit, wantCode: FailureExecutionOutcomeUnknown, wantIs: []error{context.DeadlineExceeded, ErrOutcomeUnknown}},
		{name: "cancel on ddl", err: context.Canceled, op: opDDL, wantCode: FailureExecutionOutcomeUnknown, wantIs: []error{context.Canceled}},
		{name: "cancel on read", err: context.Canceled, op: opQuery, noFail: true, wantIs: []error{context.Canceled}},
		{name: "cancel on fetch", err: context.Canceled, op: opFetch, noFail: true, wantIs: []error{context.Canceled}},
		{name: "session not found", err: ErrSessionNotFound, op: opQuery, wantCode: FailureSessionNotFound, wantIs: []error{ErrSessionNotFound}},
		{name: "session lost", err: ErrSessionLost, op: opQuery, wantCode: FailureSessionLost, wantIs: []error{ErrSessionLost}},
		{name: "cursor lost", err: ErrCursorLost, op: opFetch, wantCode: FailureCursorLost, wantIs: []error{ErrCursorLost}},
		{name: "driver cursor closed", err: cursor.ErrCursorClosed, op: opFetch, wantCode: FailureCursorLost, wantIs: []error{ErrCursorLost, cursor.ErrCursorClosed}},
		{name: "transaction lost", err: ErrTransactionLost, op: opExecute, wantCode: FailureTransactionLost, wantIs: []error{ErrTransactionLost}},
		{name: "outcome unknown", err: ErrOutcomeUnknown, op: opExecute, wantCode: FailureExecutionOutcomeUnknown, wantIs: []error{ErrOutcomeUnknown}},
		{name: "limit", err: ErrLimitExceeded, op: opQuery, wantCode: FailureLimitExceeded, wantIs: []error{ErrLimitExceeded}},
		{name: "export byte limit", err: exports.ErrByteLimitExceeded, op: opStream, wantCode: FailureLimitExceeded, wantIs: []error{ErrLimitExceeded, exports.ErrByteLimitExceeded}},
		{name: "cursors unsupported", err: connection.ErrQueryCursorsUnsupported, op: opQuery, noFail: true, wantIs: []error{ErrCursorsUnsupported, connection.ErrQueryCursorsUnsupported}},
		{name: "export cursors unsupported", err: exports.ErrCursorUnsupported, op: opStream, noFail: true, wantIs: []error{ErrCursorsUnsupported}},
		{name: "transaction open", err: connection.ErrTransactionOpen, op: opTransaction, noFail: true, wantIs: []error{ErrTransactionOpen, connection.ErrTransactionOpen}},
		{name: "no open transaction", err: transaction.ErrNoOpenTransaction, op: opCommit, noFail: true, wantIs: []error{ErrNoOpenTransaction}},
		{name: "ddl unsupported", err: ddl.ErrUnsupported, op: opDDL, noFail: true, wantIs: []error{ErrDDLUnsupported}},
		{name: "wrapped sentinel", err: errors.Join(boom, ErrSessionLost), op: opMetadata, wantCode: FailureSessionLost, wantIs: []error{boom, ErrSessionLost}},
		{name: "target error on execute", err: boom, op: opExecute, noFail: true, target: true, wantIs: []error{boom}},
		{name: "target error on query", err: boom, op: opQuery, noFail: true, target: true, wantIs: []error{boom}},
		{name: "target error on commit", err: boom, op: opCommit, noFail: true, target: true, wantIs: []error{boom}},
		{name: "existing target error", err: &TargetError{Err: boom}, op: opQuery, noFail: true, target: true, wantIs: []error{boom}},
		{name: "cancel error is internal", err: boom, op: opCancel, noFail: true, wantIs: []error{boom}},
		{name: "runtime sentinel is not target", err: ErrSchemaUnsupported, op: opMetadata, noFail: true, wantIs: []error{ErrSchemaUnsupported}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapError(tc.err, tc.op)
			if tc.err == nil {
				if got != nil {
					t.Fatalf("got %v", got)
				}
				return
			}
			var f *Failure
			hasFailure := errors.As(got, &f)
			if tc.noFail {
				if hasFailure {
					t.Fatalf("unexpected failure code %s", f.Code)
				}
			} else if !hasFailure || f.Code != tc.wantCode {
				t.Fatalf("want code %s, got %v", tc.wantCode, got)
			}
			for _, want := range tc.wantIs {
				if !errors.Is(got, want) {
					t.Fatalf("errors.Is(%v, %v) = false", got, want)
				}
			}
			var target *TargetError
			if errors.As(got, &target) != tc.target {
				t.Fatalf("TargetError = %v, want %v (got %v)", !tc.target, tc.target, got)
			}
		})
	}

	existing := &Failure{Code: FailureCursorLost, Err: boom}
	if got := mapError(existing, opExecute); got != error(existing) {
		t.Fatalf("an existing failure must pass through, got %v", got)
	}
}

func TestQueriesRequireCursorDoesNotFallBack(t *testing.T) {
	r, p := newTestRuntime(t, allowAll())
	p.creds = credentials.Credentials{Driver: "capturecfg", DSN: "x"}
	info := mustOpen(t, r, baseScope)
	_, err := r.Query(context.Background(), baseScope, QueryRequest{SessionID: info.ID, SQL: "select 1", UseCursor: true, RequireCursor: true})
	if !errors.Is(err, ErrCursorsUnsupported) {
		t.Fatalf("err = %v, want ErrCursorsUnsupported", err)
	}
	if _, err := r.Get(context.Background(), baseScope, info.ID); err != nil {
		t.Fatalf("unsupported cursor must keep the session: %v", err)
	}
}

func TestCursorAddressedWithoutSessionID(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t order by id", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}

	page, err := r.Fetch(ctx, baseScope, FetchRequest{CursorID: first.CursorID, PageSize: 1})
	if err != nil || len(page.Result.Rows) != 1 {
		t.Fatalf("owner fetch without session id: %+v %v", page, err)
	}

	foreign := scopeOf("o1", "w1", "a2", "c1")
	_, err = r.Fetch(ctx, foreign, FetchRequest{CursorID: first.CursorID})
	requireFailure(t, err, FailureCursorLost)
	_, err = r.Fetch(ctx, baseScope, FetchRequest{CursorID: "no-such-cursor"})
	requireFailure(t, err, FailureCursorLost)

	if err := r.CloseCursor(ctx, foreign, "", first.CursorID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Fetch(ctx, baseScope, FetchRequest{CursorID: first.CursorID, PageSize: 1}); err != nil {
		t.Fatalf("foreign close must not close the cursor: %v", err)
	}
	if err := r.CloseCursor(ctx, baseScope, "", first.CursorID); err != nil {
		t.Fatal(err)
	}
	_, err = r.Fetch(ctx, baseScope, FetchRequest{CursorID: first.CursorID})
	requireFailure(t, err, FailureCursorLost)
}

func TestCursorForeignAndUnknownAreIndistinguishable(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	first, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t", UseCursor: true, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}

	otherScope := scopeOf("o1", "w1", "a2", "c1")
	other, err := r.Open(ctx, OpenRequest{Scope: otherScope})
	if err != nil {
		t.Fatal(err)
	}
	for name, sid := range map[string]SessionID{"own session": other.ID, "owner's session id": info.ID, "unknown session": "nope"} {
		_, foreignErr := r.Fetch(ctx, otherScope, FetchRequest{SessionID: sid, CursorID: first.CursorID})
		_, unknownErr := r.Fetch(ctx, otherScope, FetchRequest{SessionID: sid, CursorID: "no-such-cursor"})
		var ff, uf *Failure
		if !errors.As(foreignErr, &ff) || !errors.As(unknownErr, &uf) || ff.Code != uf.Code || foreignErr.Error() != unknownErr.Error() {
			t.Fatalf("%s: foreign=%v unknown=%v", name, foreignErr, unknownErr)
		}
	}
}
