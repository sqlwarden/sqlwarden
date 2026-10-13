package query

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/safety"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/pkg/result"
)

const (
	dqlUser  int64 = 40
	dmlUser  int64 = 41
	ddlUser  int64 = 42
	ownerSID       = execution.SessionID("owner-session")
)

// fakePolicy grants connection permissions per account. userA holds the
// broad conn:execute; userB holds policy:read and policy:modify on the
// workspace.
type fakePolicy struct{}

func (fakePolicy) Can(_ context.Context, accountID, orgID int64, _, resourceType string, resourceID int64, permission string) bool {
	if orgID != testOrg {
		return false
	}
	switch resourceType {
	case "connection":
		if resourceID != testConn {
			return false
		}
		switch accountID {
		case userA:
			return permission == access.PermConnExecute
		case dqlUser:
			return permission == access.PermConnDQL
		case dmlUser:
			return permission == access.PermConnDML
		case ddlUser:
			return permission == access.PermConnDDL
		}
	case "workspace":
		return resourceID == testWorkspace && accountID == userB &&
			(permission == access.PermPolicyRead || permission == access.PermPolicyModify)
	}
	return false
}

func (fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

// fakeRuntime knows one live session per account, keyed by the account id.
// It embeds the interface so unexercised methods panic if reached.
type fakeRuntime struct {
	Runtime
	calls    int
	sessions []execution.SessionInfo
	query    *execution.QueryRequest
	execute  *execution.ExecuteRequest
	fetch    *execution.FetchRequest
	closed   []execution.SessionID
	mode     execution.TxMode
	queryErr error
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{sessions: []execution.SessionInfo{
		{ID: ownerSID, Scope: scope(principal(userA), connRef()), Driver: "sqlite"},
		{ID: "b-session", Scope: scope(principal(userB), connRef()), Driver: "sqlite"},
		{ID: "elsewhere", Scope: execution.Scope{OrgID: "1", WorkspaceID: "11", AccountID: "7", ConnectionID: "31"}, Driver: "sqlite"},
	}}
}

func (f *fakeRuntime) lookup(sc execution.Scope, id execution.SessionID) (execution.SessionInfo, error) {
	for _, s := range f.sessions {
		if s.ID == id && s.Scope == sc {
			return s, nil
		}
	}
	return execution.SessionInfo{}, &execution.Failure{Code: execution.FailureSessionNotFound, Err: execution.ErrSessionNotFound}
}

func (f *fakeRuntime) Open(_ context.Context, req execution.OpenRequest) (execution.SessionInfo, error) {
	f.calls++
	return execution.SessionInfo{ID: "opened", Scope: req.Scope}, nil
}

func (f *fakeRuntime) Get(_ context.Context, sc execution.Scope, id execution.SessionID) (execution.SessionInfo, error) {
	f.calls++
	return f.lookup(sc, id)
}

func (f *fakeRuntime) List(_ context.Context, filter execution.SessionFilter) ([]execution.SessionInfo, error) {
	f.calls++
	var out []execution.SessionInfo
	for _, s := range f.sessions {
		if (filter.AccountID == "" || s.Scope.AccountID == filter.AccountID) &&
			(filter.WorkspaceID == "" || s.Scope.WorkspaceID == filter.WorkspaceID) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRuntime) Close(_ context.Context, sc execution.Scope, id execution.SessionID) error {
	f.calls++
	if _, err := f.lookup(sc, id); err == nil {
		f.closed = append(f.closed, id)
	}
	return nil
}

func (f *fakeRuntime) Query(_ context.Context, _ execution.Scope, req execution.QueryRequest) (execution.QueryResult, error) {
	f.calls++
	f.query = &req
	if f.queryErr != nil {
		return execution.QueryResult{}, f.queryErr
	}
	return execution.QueryResult{Result: &result.ResultSet{}, CursorID: "c1"}, nil
}

func (f *fakeRuntime) Fetch(_ context.Context, _ execution.Scope, req execution.FetchRequest) (execution.QueryResult, error) {
	f.calls++
	f.fetch = &req
	return execution.QueryResult{Result: &result.ResultSet{}}, nil
}

func (f *fakeRuntime) CloseCursor(context.Context, execution.Scope, execution.SessionID, execution.CursorID) error {
	f.calls++
	return nil
}

func (f *fakeRuntime) Execute(_ context.Context, _ execution.Scope, req execution.ExecuteRequest) (execution.ExecuteResult, error) {
	f.calls++
	f.execute = &req
	affected := int64(3)
	return execution.ExecuteResult{Result: &result.ResultSet{RowsAffected: &affected}}, nil
}

func (f *fakeRuntime) Status(context.Context, execution.Scope, execution.SessionID) (execution.TxStatus, error) {
	f.calls++
	return execution.TxStatus{Mode: execution.TxModeAuto}, nil
}

func (f *fakeRuntime) SetMode(_ context.Context, _ execution.Scope, _ execution.SessionID, mode execution.TxMode) (execution.TxStatus, error) {
	f.calls++
	f.mode = mode
	return execution.TxStatus{Mode: mode}, nil
}

func (f *fakeRuntime) Commit(context.Context, execution.Scope, execution.SessionID) (execution.TxStatus, error) {
	f.calls++
	return execution.TxStatus{}, nil
}

func (f *fakeRuntime) Rollback(context.Context, execution.Scope, execution.SessionID) (execution.TxStatus, error) {
	f.calls++
	return execution.TxStatus{}, nil
}

// fakeAnalyzers classifies by statement prefix and reports a missing WHERE
// for any statement containing "ALL ROWS".
type fakeAnalyzers struct {
	classifyCalls int
	safetyCalls   int
	explainer     explain.Explainer
}

func (f *fakeAnalyzers) Classifier(string) classifier.Classifier { return fakeClassifier{f} }
func (f *fakeAnalyzers) SafetyChecker(string) safety.Checker     { return fakeChecker{f} }
func (f *fakeAnalyzers) Explainer(string) (explain.Explainer, bool) {
	return f.explainer, f.explainer != nil
}

type fakeClassifier struct{ a *fakeAnalyzers }

func (c fakeClassifier) Classify(_ context.Context, req classifier.Request) (classifier.Result, error) {
	c.a.classifyCalls++
	kind := classifier.KindUnknown
	switch strings.Fields(req.SQL)[0] {
	case "SELECT":
		kind = classifier.KindDQL
	case "UPDATE", "DELETE":
		kind = classifier.KindDML
	case "CREATE":
		kind = classifier.KindDDL
	}
	return classifier.Result{Kind: kind, Source: "fake"}, nil
}

type fakeChecker struct{ a *fakeAnalyzers }

func (c fakeChecker) Check(_ context.Context, req safety.Request) (safety.Result, error) {
	c.a.safetyCalls++
	if strings.Contains(req.SQL, "ALL ROWS") {
		return safety.Result{Unsafe: true, Statements: []safety.UnsafeStatement{{Kind: safety.KindUnsafeMissingWhere, EndOffset: len(req.SQL)}}}, nil
	}
	return safety.Result{}, nil
}

type fakeExplainer struct{ analyze bool }

func (e fakeExplainer) ExplainSpec() explain.Spec { return explain.Spec{SupportsAnalyze: e.analyze} }

func (e fakeExplainer) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
	switch {
	case strings.Contains(sql, ";"):
		return explain.Plan{}, explain.ErrMultipleStatements
	case strings.HasPrefix(sql, "EXPLAIN"):
		return explain.Plan{}, explain.ErrAlreadyExplained
	}
	return explain.Plan{Statement: "EXPLAIN " + string(mode) + " " + sql}, nil
}

type recordingAudit struct {
	events []audit.Event
	err    error
}

func (a *recordingAudit) Write(_ context.Context, e audit.Event) error {
	a.events = append(a.events, e)
	return a.err
}

type runtimeHarness struct {
	svc       *Service
	runtime   *fakeRuntime
	analyzers *fakeAnalyzers
	settings  *fakeSettings
	audit     *recordingAudit
}

func newRuntimeHarness() runtimeHarness {
	h := runtimeHarness{
		runtime:   newFakeRuntime(),
		analyzers: &fakeAnalyzers{explainer: fakeExplainer{analyze: true}},
		settings:  &fakeSettings{policy: Policy{MaxRows: 100, MaxBytes: 1 << 20, CursorPageSize: 50}},
		audit:     &recordingAudit{},
	}
	h.svc = New(Deps{
		Settings:  h.settings,
		Requirer:  access.NewRequirer(fakeDirectory{}, fakePolicy{}),
		Runtime:   h.runtime,
		Analyzers: h.analyzers,
		Audit:     h.audit,
	})
	return h
}

// addSession gives the account its own live session on the test connection.
func (h runtimeHarness) addSession(account int64) execution.SessionID {
	id := execution.SessionID("session-" + strconv.FormatInt(account, 10))
	h.runtime.sessions = append(h.runtime.sessions, execution.SessionInfo{ID: id, Scope: scope(principal(account), connRef())})
	return id
}

func TestExecuteClassificationPermissionTable(t *testing.T) {
	statements := map[classifier.Kind]string{
		classifier.KindDQL:     "SELECT 1",
		classifier.KindDML:     "UPDATE t SET a = 1 WHERE id = 1",
		classifier.KindDDL:     "CREATE TABLE t (id INTEGER)",
		classifier.KindUnknown: "VACUUM",
	}
	allowed := map[int64]map[classifier.Kind]bool{
		userA:   {classifier.KindDQL: true, classifier.KindDML: true, classifier.KindDDL: true, classifier.KindUnknown: true},
		dqlUser: {classifier.KindDQL: true},
		dmlUser: {classifier.KindDML: true},
		ddlUser: {classifier.KindDDL: true},
	}
	for account, kinds := range allowed {
		for kind, sql := range statements {
			t.Run(strconv.FormatInt(account, 10)+"/"+string(kind), func(t *testing.T) {
				h := newRuntimeHarness()
				sid := h.addSession(account)
				_, err := h.svc.Execute(context.Background(), principal(account), connRef(), ExecuteInput{SessionID: sid, SQL: sql})
				ran := h.runtime.query != nil || h.runtime.execute != nil
				if kinds[kind] {
					if err != nil || !ran {
						t.Fatalf("err = %v, ran = %v, want the statement to run", err, ran)
					}
					if (kind == classifier.KindDQL) != (h.runtime.query != nil) {
						t.Fatalf("DQL must run through Query and other classes through Execute")
					}
					return
				}
				if !errors.Is(err, ErrForbidden) || ran {
					t.Fatalf("err = %v, ran = %v, want ErrForbidden without running", err, ran)
				}
			})
		}
	}
}

func TestRuntimeOperationsDenyBeforeRuntime(t *testing.T) {
	foreignConn := access.ConnRef{OrgID: testOrg, WorkspaceID: 11, ConnectionID: testConn}
	tests := []struct {
		name string
		p    access.Principal
		ref  access.ConnRef
		want error
		// gated reports whether the operation requires a runtime permission;
		// operations whose authorization the runtime owns only resolve the
		// connection.
		gatedOnly bool
	}{
		{name: "non-member", p: principal(outsider), ref: connRef(), want: ErrForbidden},
		{name: "unknown connection", p: principal(userA), ref: access.ConnRef{OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: 404}, want: ErrNotFound},
		{name: "connection in another workspace", p: principal(userA), ref: foreignConn, want: ErrNotFound},
		{name: "org member without connection permission", p: principal(orgOnlyUser), ref: connRef(), want: ErrForbidden, gatedOnly: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newRuntimeHarness()
			ctx := context.Background()
			gated := map[string]error{}
			_, gated["connect"] = h.svc.Connect(ctx, tt.p, tt.ref)
			_, gated["execute"] = h.svc.Execute(ctx, tt.p, tt.ref, ExecuteInput{SessionID: ownerSID, SQL: "SELECT 1"})
			_, gated["start cursor"] = h.svc.StartCursor(ctx, tt.p, tt.ref, CursorInput{SessionID: ownerSID, SQL: "SELECT 1"})
			_, gated["fetch cursor"] = h.svc.FetchCursor(ctx, tt.p, tt.ref, FetchInput{SessionID: ownerSID, CursorID: "c1"})
			_, gated["status"] = h.svc.TransactionStatus(ctx, tt.p, tt.ref, ownerSID)
			_, gated["mode"] = h.svc.SetTransactionMode(ctx, tt.p, tt.ref, ownerSID, "manual")
			_, gated["commit"] = h.svc.Commit(ctx, tt.p, tt.ref, ownerSID)
			_, gated["rollback"] = h.svc.Rollback(ctx, tt.p, tt.ref, ownerSID)
			for op, err := range gated {
				accesstest.AssertDenies(t, err, tt.want, op)
			}
			if h.runtime.calls != 0 || h.settings.calls != 0 || h.analyzers.classifyCalls != 0 {
				t.Fatalf("runtime calls = %d, settings calls = %d, classify calls = %d, want none on denial",
					h.runtime.calls, h.settings.calls, h.analyzers.classifyCalls)
			}

			resolved := map[string]error{}
			resolved["disconnect"] = h.svc.Disconnect(ctx, tt.p, tt.ref, ownerSID)
			resolved["close cursor"] = h.svc.CloseCursor(ctx, tt.p, tt.ref, ownerSID, "c1")
			for op, err := range resolved {
				if tt.gatedOnly {
					if err != nil {
						t.Errorf("%s error = %v, want the runtime to decide", op, err)
					}
					continue
				}
				accesstest.AssertDenies(t, err, tt.want, op)
			}
			if !tt.gatedOnly && h.runtime.calls != 0 {
				t.Fatalf("runtime calls = %d, want none on denial", h.runtime.calls)
			}
			if len(h.runtime.closed) != 0 {
				t.Fatalf("closed %v, want no session closed", h.runtime.closed)
			}
		})
	}
}

func TestExecuteValidationAndSession(t *testing.T) {
	h := newRuntimeHarness()
	ctx := context.Background()
	zero := 0
	_, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, Explain: "verbose", PageSize: &zero})
	var validation *ValidationError
	if !errors.As(err, &validation) || len(validation.Fields) != 3 {
		t.Fatalf("err = %v, want sql, explain and page_size field errors", err)
	}
	if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SQL: "SELECT 1"}); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
	if h.runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want none before a valid request", h.runtime.calls)
	}
	_, err = h.svc.Execute(ctx, principal(dqlUser), connRef(), ExecuteInput{SessionID: ownerSID, SQL: "SELECT 1"})
	var failure *execution.Failure
	if !errors.As(err, &failure) || failure.Code != execution.FailureSessionNotFound {
		t.Fatalf("err = %v, want the runtime's session_not_found for another account's session", err)
	}
	if h.runtime.query != nil {
		t.Fatal("a foreign session must not run the statement")
	}
}

func TestExecuteExplainGating(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		explainer explain.Explainer
		sql       string
		mode      string
		want      error
	}{
		{name: "no explainer", sql: "SELECT 1", mode: "plain", want: ErrExplainUnsupported},
		{name: "analyze unsupported", explainer: fakeExplainer{}, sql: "SELECT 1", mode: "analyze", want: ErrExplainAnalyzeUnsupported},
		{name: "multiple statements", explainer: fakeExplainer{analyze: true}, sql: "SELECT 1; SELECT 2", mode: "plain", want: ErrExplainMultiple},
		{name: "already explained", explainer: fakeExplainer{analyze: true}, sql: "EXPLAIN SELECT 1", mode: "plain", want: ErrExplainAlready},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newRuntimeHarness()
			h.analyzers.explainer = tt.explainer
			_, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: tt.sql, Explain: tt.mode})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if h.runtime.query != nil || h.runtime.execute != nil {
				t.Fatal("a refused EXPLAIN must not run")
			}
		})
	}

	t.Run("plan runs as an execution and still needs the class permission", func(t *testing.T) {
		h := newRuntimeHarness()
		sid := h.addSession(dqlUser)
		out, err := h.svc.Execute(ctx, principal(dqlUser), connRef(), ExecuteInput{SessionID: sid, SQL: "SELECT 1", Explain: "analyze"})
		if err != nil {
			t.Fatal(err)
		}
		if h.runtime.execute == nil || h.runtime.execute.Explain == nil || h.runtime.execute.Explain.Statement != "EXPLAIN analyze SELECT 1" {
			t.Fatalf("execute = %+v, want the explain plan", h.runtime.execute)
		}
		if out.Result.RowsAffected != nil {
			t.Fatal("rows affected must be cleared for non-DML")
		}
		_, err = h.svc.Execute(ctx, principal(dqlUser), connRef(), ExecuteInput{SessionID: sid, SQL: "UPDATE t SET a = 1", Explain: "plain"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want planning DML without conn:dml to be forbidden", err)
		}
	})
}

func TestExecuteUnsafeConfirmationGate(t *testing.T) {
	ctx := context.Background()
	const unsafeSQL = "DELETE FROM t ALL ROWS"

	h := newRuntimeHarness()
	_, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: unsafeSQL})
	var unsafe *UnsafeError
	if !errors.As(err, &unsafe) || len(unsafe.Statements) != 1 || !errors.Is(err, ErrUnsafeConfirmation) {
		t.Fatalf("err = %v, want UnsafeError with the offending statement", err)
	}
	if h.runtime.execute != nil {
		t.Fatal("an unconfirmed unsafe statement must not run")
	}

	h = newRuntimeHarness()
	out, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: unsafeSQL, ConfirmUnsafe: true})
	if err != nil || h.runtime.execute == nil || h.analyzers.safetyCalls != 0 {
		t.Fatalf("err = %v, execute = %v, safety calls = %d, want a confirmed statement to run unchecked", err, h.runtime.execute, h.analyzers.safetyCalls)
	}
	if out.Result.RowsAffected == nil {
		t.Fatal("rows affected must be kept for DML")
	}

	h = newRuntimeHarness()
	if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: unsafeSQL, Explain: "plain"}); err != nil {
		t.Fatalf("plain EXPLAIN err = %v, want the gate skipped", err)
	}
	if h.analyzers.safetyCalls != 0 {
		t.Fatal("plain EXPLAIN must not consult the safety checker")
	}

	h = newRuntimeHarness()
	if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: unsafeSQL, Explain: "analyze"}); !errors.As(err, &unsafe) {
		t.Fatalf("EXPLAIN ANALYZE err = %v, want the gate applied", err)
	}

	h = newRuntimeHarness()
	if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: "CREATE TABLE ALL ROWS"}); err != nil || h.analyzers.safetyCalls != 0 {
		t.Fatalf("err = %v, safety calls = %d, want DDL ungated", err, h.analyzers.safetyCalls)
	}
}

func TestCursorPageSizeClamp(t *testing.T) {
	policy := Policy{MaxRows: 100, CursorPageSize: 50}
	ptr := func(n int) *int { return &n }
	tests := []struct {
		requested *int
		want      int
	}{
		{nil, 50},
		{ptr(10), 10},
		{ptr(100), 100},
		{ptr(500), 100},
	}
	for _, tt := range tests {
		if got := cursorPageSize(tt.requested, policy); got != tt.want {
			t.Errorf("cursorPageSize(%v) = %d, want %d", tt.requested, got, tt.want)
		}
	}
	if got := cursorPageSize(nil, Policy{MaxRows: 20, CursorPageSize: 50}); got != 20 {
		t.Errorf("default page size = %d, want it clamped to max rows", got)
	}

	ctx := context.Background()
	h := newRuntimeHarness()
	page, err := h.svc.StartCursor(ctx, principal(userA), connRef(), CursorInput{SessionID: ownerSID, SQL: "SELECT 1", PageSize: ptr(500)})
	if err != nil || page.PageSize != 100 || h.runtime.query.PageSize != 100 || !h.runtime.query.RequireCursor {
		t.Fatalf("err = %v, page = %+v, query = %+v, want a required cursor clamped to 100", err, page, h.runtime.query)
	}
	page, err = h.svc.FetchCursor(ctx, principal(userA), connRef(), FetchInput{CursorID: "c9", PageSize: ptr(500)})
	if err != nil || page.PageSize != 100 || h.runtime.fetch.PageSize != 100 || page.Result.CursorID != "c9" {
		t.Fatalf("err = %v, page = %+v, want the fetch clamped to 100 and the cursor id echoed", err, page)
	}
	if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	if !h.runtime.query.UseCursor || h.runtime.query.PageSize != 50 {
		t.Fatalf("query = %+v, want DQL to default to a cursor of the configured page size", h.runtime.query)
	}
}

func TestStartCursorPermission(t *testing.T) {
	ctx := context.Background()
	h := newRuntimeHarness()
	if _, err := h.svc.StartCursor(ctx, principal(userA), connRef(), CursorInput{SessionID: ownerSID, SQL: "VACUUM"}); err != nil {
		t.Fatal(err)
	}
	if h.analyzers.classifyCalls != 0 {
		t.Fatal("broad execute must not classify")
	}
	for _, tt := range []struct {
		account int64
		sql     string
	}{
		{dqlUser, "UPDATE t SET a = 1"},
		{dqlUser, "VACUUM"},
		{dmlUser, "SELECT 1"},
	} {
		h := newRuntimeHarness()
		if _, err := h.svc.StartCursor(ctx, principal(tt.account), connRef(), CursorInput{SessionID: "any", SQL: tt.sql}); !errors.Is(err, ErrForbidden) || h.runtime.calls != 0 {
			t.Errorf("account %d %q: err = %v, runtime calls = %d, want ErrForbidden before the runtime", tt.account, tt.sql, err, h.runtime.calls)
		}
	}
	h = newRuntimeHarness()
	if _, err := h.svc.StartCursor(ctx, principal(dqlUser), connRef(), CursorInput{SQL: "SELECT 1"}); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
	h = newRuntimeHarness()
	h.runtime.queryErr = ErrCursorsUnsupported
	page, err := h.svc.StartCursor(ctx, principal(userA), connRef(), CursorInput{SessionID: ownerSID, SQL: "SELECT 1"})
	if !errors.Is(err, execution.ErrCursorsUnsupported) || page.PageSize != 50 {
		t.Fatalf("err = %v, page = %+v, want the runtime error with the effective page size", err, page)
	}
	var runtimeErr *RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("err = %v, want it marked as a runtime failure", err)
	}
}

// userB is a workspace member with no connection permission, the state of a
// principal whose grants were revoked while a transaction or cursor was open.
func TestRevokedPrincipalCanOnlyRollBack(t *testing.T) {
	ctx := context.Background()
	h := newRuntimeHarness()
	if _, err := h.svc.Rollback(ctx, principal(userB), connRef(), ownerSID); err != nil {
		t.Fatalf("rollback err = %v, want nil", err)
	}
	if h.runtime.calls != 1 {
		t.Fatalf("runtime calls = %d, want the rollback to reach the runtime", h.runtime.calls)
	}
	if _, err := h.svc.Rollback(ctx, principal(userB), connRef(), ""); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
	_, err := h.svc.Rollback(ctx, principal(orgOnlyUser), connRef(), ownerSID)
	accesstest.AssertForbidden(t, err, "rollback by non-member")

	h = newRuntimeHarness()
	_, err = h.svc.Commit(ctx, principal(userB), connRef(), ownerSID)
	accesstest.AssertForbidden(t, err, "commit")
	_, err = h.svc.FetchCursor(ctx, principal(userB), connRef(), FetchInput{SessionID: ownerSID, CursorID: "c1"})
	accesstest.AssertForbidden(t, err, "fetch")
	if h.runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want none", h.runtime.calls)
	}
}

func TestSetTransactionModeValidation(t *testing.T) {
	ctx := context.Background()
	h := newRuntimeHarness()
	_, err := h.svc.SetTransactionMode(ctx, principal(userA), connRef(), ownerSID, "sometimes")
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["mode"] != "Mode must be auto or manual." {
		t.Fatalf("err = %v, want a mode field error", err)
	}
	if h.runtime.mode != "" {
		t.Fatal("an invalid mode must not reach the runtime")
	}

	_, err = h.svc.SetTransactionMode(ctx, principal(dqlUser), connRef(), ownerSID, "sometimes")
	var failure *execution.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v, want the session to be confirmed before the mode is validated", err)
	}

	status, err := h.svc.SetTransactionMode(ctx, principal(userA), connRef(), ownerSID, "manual")
	if err != nil || status.Mode != execution.TxModeManual {
		t.Fatalf("status = %+v, err = %v, want manual", status, err)
	}
	if _, err := h.svc.Commit(ctx, principal(userA), connRef(), ""); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
}

func TestConnectAndDisconnect(t *testing.T) {
	ctx := context.Background()
	h := newRuntimeHarness()
	info, err := h.svc.Connect(ctx, principal(dqlUser), connRef())
	if err != nil || info.Scope != scope(principal(dqlUser), connRef()) {
		t.Fatalf("info = %+v, err = %v, want a session in the caller's scope", info, err)
	}
	if err := h.svc.Disconnect(ctx, principal(dqlUser), connRef(), ""); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
	if err := h.svc.Disconnect(ctx, principal(orgOnlyUser), connRef(), ownerSID); err != nil {
		t.Fatal(err)
	}
	if len(h.runtime.closed) != 0 {
		t.Fatal("disconnecting another account's session must leave it alive")
	}
	if err := h.svc.Disconnect(ctx, principal(userA), connRef(), ownerSID); err != nil || len(h.runtime.closed) != 1 {
		t.Fatalf("err = %v, closed = %v, want the owner's session closed", err, h.runtime.closed)
	}
}

func TestListWorkspaceSessions(t *testing.T) {
	ctx := context.Background()
	h := newRuntimeHarness()
	own, err := h.svc.ListWorkspaceSessions(ctx, principal(userA), testOrg, testWorkspace)
	if err != nil || len(own) != 1 || own[0].ID != ownerSID {
		t.Fatalf("sessions = %+v, err = %v, want only the caller's session", own, err)
	}
	all, err := h.svc.ListWorkspaceSessions(ctx, principal(userB), testOrg, testWorkspace)
	if err != nil || len(all) != 2 {
		t.Fatalf("sessions = %+v, err = %v, want every session in the workspace", all, err)
	}
	h = newRuntimeHarness()
	for _, account := range []int64{outsider, orgOnlyUser} {
		_, err := h.svc.ListWorkspaceSessions(ctx, principal(account), testOrg, testWorkspace)
		accesstest.AssertForbidden(t, err, "list sessions by "+strconv.FormatInt(account, 10))
	}
	_, err = h.svc.ListWorkspaceSessions(ctx, principal(userA), testOrg, 404)
	accesstest.AssertNotFound(t, err, "list sessions in unknown workspace")
	if h.runtime.calls != 0 {
		t.Fatalf("runtime calls = %d, want none on denial", h.runtime.calls)
	}
}

func TestRevokeSessionOwnership(t *testing.T) {
	ctx := context.Background()

	h := newRuntimeHarness()
	if _, err := h.svc.RevokeSession(ctx, principal(userA), testOrg, testWorkspace, "b-session"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden revoking another account's session", err)
	}
	if len(h.runtime.closed) != 0 {
		t.Fatal("a refused revoke must not close the session")
	}

	revoked, err := h.svc.RevokeSession(ctx, principal(userA), testOrg, testWorkspace, string(ownerSID))
	if err != nil || revoked == nil || revoked.ID != ownerSID || len(h.runtime.closed) != 1 {
		t.Fatalf("revoked = %+v, err = %v, want the owner to revoke their own session", revoked, err)
	}

	h = newRuntimeHarness()
	if revoked, err := h.svc.RevokeSession(ctx, principal(userB), testOrg, testWorkspace, string(ownerSID)); err != nil || revoked == nil {
		t.Fatalf("revoked = %+v, err = %v, want policy:modify to revoke another account's session", revoked, err)
	}

	h = newRuntimeHarness()
	for _, id := range []string{"unknown", "elsewhere"} {
		revoked, err := h.svc.RevokeSession(ctx, principal(userA), testOrg, testWorkspace, id)
		if err != nil || revoked != nil {
			t.Errorf("%s: revoked = %+v, err = %v, want no session and no error", id, revoked, err)
		}
	}

	h = newRuntimeHarness()
	if _, err := h.svc.RevokeSession(ctx, principal(outsider), testOrg, testWorkspace, string(ownerSID)); !errors.Is(err, ErrForbidden) || h.runtime.calls != 0 {
		t.Fatalf("err = %v, runtime calls = %d, want ErrForbidden before the runtime", err, h.runtime.calls)
	}

	h = newRuntimeHarness()
	if _, err := h.svc.RevokeSession(ctx, principal(orgOnlyUser), testOrg, testWorkspace, "unknown"); !errors.Is(err, ErrForbidden) || h.runtime.calls != 0 {
		t.Fatalf("err = %v, runtime calls = %d, want a non-member refused before the runtime lists sessions", err, h.runtime.calls)
	}
}

func TestExecuteAuditsNonReadStatements(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		sql     string
		explain string
		want    bool
	}{
		{name: "dql", sql: "SELECT 1"},
		{name: "dml", sql: "UPDATE t SET secret = 'x' WHERE id = 1", want: true},
		{name: "ddl", sql: "CREATE TABLE t (id INTEGER)", want: true},
		{name: "unknown", sql: "VACUUM", want: true},
		{name: "plain explain of dml", sql: "UPDATE t SET a = 1 WHERE id = 1", explain: "plain"},
		{name: "explain analyze of dml", sql: "UPDATE t SET a = 1 WHERE id = 1", explain: "analyze", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newRuntimeHarness()
			if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: tt.sql, Explain: tt.explain}); err != nil {
				t.Fatal(err)
			}
			if !tt.want {
				if len(h.audit.events) != 0 {
					t.Fatalf("events = %+v, want none", h.audit.events)
				}
				return
			}
			if len(h.audit.events) != 1 {
				t.Fatalf("events = %d, want 1", len(h.audit.events))
			}
			e := h.audit.events[0]
			if e.Action != actionQueryExecuted || e.Resource != "connection" || e.ResourceID != strconv.FormatInt(testConn, 10) ||
				e.Outcome != audit.OutcomeSuccess || e.OrgID == nil || *e.OrgID != testOrg {
				t.Fatalf("event = %+v", e)
			}
			for key, value := range e.Metadata {
				if key != "workspace_id" && key != "driver" && key != "kind" {
					t.Errorf("unexpected metadata key %q", key)
				}
				if strings.Contains(value, "secret") || strings.Contains(value, "UPDATE") {
					t.Errorf("metadata %q carries SQL: %q", key, value)
				}
			}
		})
	}

	h := newRuntimeHarness()
	h.audit.err = errors.New("audit store down")
	if _, err := h.svc.Execute(ctx, principal(userA), connRef(), ExecuteInput{SessionID: ownerSID, SQL: "UPDATE t SET a = 1 WHERE id = 1"}); err != nil {
		t.Fatalf("err = %v, want the audit failure not to fail a statement that already ran", err)
	}
}
