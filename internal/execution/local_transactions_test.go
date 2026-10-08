package execution

import (
	"context"
	"errors"
	"testing"
)

func TestTransactionsManualRoundTrip(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx := context.Background()

	st, err := r.SetMode(ctx, baseScope, info.ID, TxModeManual)
	if err != nil || st.Mode != TxModeManual || st.Open || st.Statements == nil {
		t.Fatalf("set manual: %+v %v", st, err)
	}
	if _, err := r.Execute(ctx, baseScope, ExecuteRequest{SessionID: info.ID, SQL: "insert into t values (4, 'd')"}); err != nil {
		t.Fatal(err)
	}
	st, err = r.Status(ctx, baseScope, info.ID)
	if err != nil || !st.Open || st.PendingStatements != 1 {
		t.Fatalf("status: %+v %v", st, err)
	}
	st, err = r.Rollback(ctx, baseScope, info.ID)
	if err != nil || st.Open || st.PendingStatements != 0 || st.Statements == nil {
		t.Fatalf("rollback: %+v %v", st, err)
	}
	res, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t"})
	if err != nil || len(res.Result.Rows) != 3 {
		t.Fatalf("rows after rollback: %+v %v", res, err)
	}
}

func TestTransactionsCommit(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx := context.Background()
	if _, err := r.SetMode(ctx, baseScope, info.ID, TxModeManual); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Execute(ctx, baseScope, ExecuteRequest{SessionID: info.ID, SQL: "insert into t values (4, 'd')"}); err != nil {
		t.Fatal(err)
	}
	st, err := r.Commit(ctx, baseScope, info.ID)
	if err != nil || st.Open {
		t.Fatalf("commit: %+v %v", st, err)
	}
	res, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select id from t"})
	if err != nil || len(res.Result.Rows) != 4 {
		t.Fatalf("rows after commit: %+v %v", res, err)
	}
}

func TestTransactionsAutoWhileOpenFails(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx := context.Background()
	if _, err := r.SetMode(ctx, baseScope, info.ID, TxModeManual); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Execute(ctx, baseScope, ExecuteRequest{SessionID: info.ID, SQL: "insert into t values (4, 'd')"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetMode(ctx, baseScope, info.ID, TxModeAuto); !errors.Is(err, ErrTransactionOpen) {
		t.Fatalf("want ErrTransactionOpen, got %v", err)
	}
}

func TestTransactionsInvalidMode(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	if _, err := r.SetMode(context.Background(), baseScope, info.ID, "bogus"); !errors.Is(err, ErrInvalidTxMode) {
		t.Fatalf("want ErrInvalidTxMode, got %v", err)
	}
}

func TestTransactionsNothingOpen(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx := context.Background()
	if _, err := r.SetMode(ctx, baseScope, info.ID, TxModeManual); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, baseScope, info.ID); !errors.Is(err, ErrNoOpenTransaction) {
		t.Fatalf("commit: want ErrNoOpenTransaction, got %v", err)
	}
	if _, err := r.Rollback(ctx, baseScope, info.ID); !errors.Is(err, ErrNoOpenTransaction) {
		t.Fatalf("rollback: want ErrNoOpenTransaction, got %v", err)
	}
}

func TestTransactionsAfterClose(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx := context.Background()
	if err := r.Close(ctx, baseScope, info.ID); err != nil {
		t.Fatal(err)
	}
	_, err := r.Commit(ctx, baseScope, info.ID)
	requireFailure(t, err, FailureTransactionLost)
	_, err = r.Rollback(ctx, baseScope, info.ID)
	requireFailure(t, err, FailureTransactionLost)
	_, err = r.Status(ctx, baseScope, info.ID)
	requireFailure(t, err, FailureSessionNotFound)
}

func TestTransactionsForeignScope(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := seededSession(t, r)
	ctx := context.Background()
	other := scopeOf("o1", "w1", "a2", "c1")
	_, err := r.Status(ctx, other, info.ID)
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.SetMode(ctx, other, info.ID, TxModeManual)
	requireFailure(t, err, FailureSessionNotFound)
	for name, call := range map[string]func(Scope, SessionID) (TxStatus, error){
		"commit":   func(s Scope, id SessionID) (TxStatus, error) { return r.Commit(ctx, s, id) },
		"rollback": func(s Scope, id SessionID) (TxStatus, error) { return r.Rollback(ctx, s, id) },
	} {
		_, foreign := call(other, info.ID)
		_, unknown := call(baseScope, "unknown")
		requireFailure(t, foreign, FailureTransactionLost)
		requireFailure(t, unknown, FailureTransactionLost)
		if foreign.Error() != unknown.Error() {
			t.Fatalf("%s: foreign %q differs from unknown %q", name, foreign, unknown)
		}
	}
}
