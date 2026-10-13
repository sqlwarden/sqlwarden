package exportjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	_ "github.com/sqlwarden/internal/engine/engines/mysql"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/settings"
)

const (
	testAccount   int64 = 7
	testOrg       int64 = 1
	testWorkspace int64 = 10
	testConn      int64 = 30
	foreignWs     int64 = 11
	foreignConn   int64 = 31
)

type fakeDirectory struct{}

func (fakeDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == testOrg, nil
}

func (fakeDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return orgID == testOrg && accountID == testAccount, nil
}

func (fakeDirectory) IsEffectiveWorkspaceMember(context.Context, int64, int64, int64) (bool, error) {
	return true, nil
}

func (fakeDirectory) GetWorkspace(_ context.Context, id int64) (access.Workspace, bool, error) {
	org := testOrg
	return access.Workspace{ID: id, OrgID: &org, OwnerType: "org", OwnerID: org}, id == testWorkspace || id == foreignWs, nil
}

func (fakeDirectory) GetConnection(_ context.Context, id int64) (access.Connection, bool, error) {
	switch id {
	case testConn:
		return access.Connection{ID: id, WorkspaceID: testWorkspace, Driver: "sqlite"}, true, nil
	case foreignConn:
		return access.Connection{ID: id, WorkspaceID: foreignWs, Driver: "sqlite"}, true, nil
	case 32:
		return access.Connection{ID: id, WorkspaceID: testWorkspace, Driver: "driver-with-no-engine"}, true, nil
	}
	return access.Connection{}, false, nil
}

type fakePolicy struct {
	mu     sync.Mutex
	grants map[string]bool
}

func (p *fakePolicy) grant(connectionID int64, permission string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.grants == nil {
		p.grants = map[string]bool{}
	}
	p.grants[fmt.Sprint(connectionID, permission)] = true
}

func (p *fakePolicy) revokeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.grants = map[string]bool{}
}

func (p *fakePolicy) Can(_ context.Context, accountID, _ int64, _, resourceType string, resourceID int64, permission string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return accountID == testAccount && resourceType == "connection" && p.grants[fmt.Sprint(resourceID, permission)]
}

func (*fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

type fakeQueue struct {
	inputs []jobs.EnqueueInput
}

func (q *fakeQueue) Enqueue(_ context.Context, input jobs.EnqueueInput) (jobs.Record, error) {
	q.inputs = append(q.inputs, input)
	return jobs.Record{ID: "job-1", Type: input.Type}, nil
}

type fakeRuntime struct {
	mu      sync.Mutex
	gets    []execution.Scope
	opens   []execution.OpenRequest
	closes  []execution.SessionID
	streams []execution.StreamRequest
	getErr  error
	openErr error
	streamF func(ctx context.Context, w io.Writer) (exports.StreamResult, error)
}

func (r *fakeRuntime) Open(_ context.Context, req execution.OpenRequest) (execution.SessionInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opens = append(r.opens, req)
	if r.openErr != nil {
		return execution.SessionInfo{}, r.openErr
	}
	return execution.SessionInfo{ID: "ephemeral", Scope: req.Scope}, nil
}

func (r *fakeRuntime) Get(_ context.Context, scope execution.Scope, id execution.SessionID) (execution.SessionInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gets = append(r.gets, scope)
	if r.getErr != nil {
		return execution.SessionInfo{}, r.getErr
	}
	return execution.SessionInfo{ID: id, Scope: scope}, nil
}

func (r *fakeRuntime) Close(_ context.Context, _ execution.Scope, id execution.SessionID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closes = append(r.closes, id)
	return nil
}

func (r *fakeRuntime) Stream(ctx context.Context, _ execution.Scope, _ execution.SessionID, req execution.StreamRequest, w io.Writer) (exports.StreamResult, error) {
	r.mu.Lock()
	r.streams = append(r.streams, req)
	fn := r.streamF
	r.mu.Unlock()
	if fn != nil {
		return fn(ctx, w)
	}
	n, err := w.Write([]byte("id\n1\n"))
	return exports.StreamResult{Rows: 1, Bytes: int64(n)}, err
}

func (r *fakeRuntime) untouched() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.gets)+len(r.opens)+len(r.closes)+len(r.streams) == 0
}

type fakeSettings struct{}

func (fakeSettings) ForWorkspace(context.Context, access.Workspace) (Policy, error) {
	return Policy{SyncMaxBytes: 100, BackgroundMaxBytes: 200}, nil
}

type fakeFiles struct {
	mu      sync.Mutex
	nextID  int64
	entries []database.WorkspaceFile
	content map[int64][]byte
	writeF  func(r io.Reader) error
}

func (f *fakeFiles) ForWorkspace(context.Context, access.Workspace) (FileWriter, error) {
	return f, nil
}

func (f *fakeFiles) List(context.Context, access.Principal, files.Scope, *int64) ([]database.WorkspaceFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]database.WorkspaceFile(nil), f.entries...), nil
}

func (f *fakeFiles) Create(_ context.Context, _ access.Principal, _ files.Scope, in files.CreateInput) (database.WorkspaceFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, entry := range f.entries {
		if entry.Name == in.Name && ((entry.ParentID == nil) == (in.ParentID == nil)) {
			return database.WorkspaceFile{}, errors.New("UNIQUE constraint failed: workspace_files.name")
		}
	}
	f.nextID++
	file := database.WorkspaceFile{ID: f.nextID, Name: in.Name, ObjectType: in.ObjectType, ParentID: in.ParentID}
	if file.ObjectType == "" {
		file.ObjectType = database.FileObjectTypeFile
	}
	f.entries = append(f.entries, file)
	return file, nil
}

func (f *fakeFiles) WriteContent(_ context.Context, _ access.Principal, _ files.Scope, id int64, _ string, content io.Reader) (database.WorkspaceFileContent, error) {
	if f.writeF != nil {
		return database.WorkspaceFileContent{}, f.writeF(content)
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.content == nil {
		f.content = map[int64][]byte{}
	}
	f.content[id] = data
	return database.WorkspaceFileContent{}, nil
}

type auditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
	err    error
}

func (a *auditRecorder) Write(_ context.Context, event audit.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
	return a.err
}

type fixture struct {
	svc     *Service
	policy  *fakePolicy
	queue   *fakeQueue
	runtime *fakeRuntime
	files   *fakeFiles
	audit   *auditRecorder
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		policy:  &fakePolicy{},
		queue:   &fakeQueue{},
		runtime: &fakeRuntime{},
		files:   &fakeFiles{},
		audit:   &auditRecorder{},
	}
	f.policy.grant(testConn, access.PermConnDQL)
	f.svc = New(Deps{
		Requirer: access.NewRequirer(fakeDirectory{}, f.policy),
		Queue:    f.queue,
		Runtime:  f.runtime,
		Settings: fakeSettings{},
		Files:    f.files,
		Audit:    f.audit,
		Now:      func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
	return f
}

func principal(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

func ref(workspaceID, connectionID int64) access.ConnRef {
	return access.ConnRef{OrgID: testOrg, WorkspaceID: workspaceID, ConnectionID: connectionID}
}

func readInput(sql string) RequestInput {
	return RequestInput{SQL: sql, Format: "csv", Filename: "report"}
}

func TestRequestEnqueuesUserJobAndAudits(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	job, err := f.svc.Request(context.Background(), principal(testAccount), ref(testWorkspace, testConn), readInput("  SELECT 1  "))
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "job-1" || len(f.queue.inputs) != 1 {
		t.Fatalf("job = %+v, enqueued = %d", job, len(f.queue.inputs))
	}
	in := f.queue.inputs[0]
	if in.Type != jobs.TypeExportQueryCSV || in.Visibility != jobs.VisibilityUser || in.MaxAttempts != 1 {
		t.Fatalf("enqueue = %+v", in)
	}
	if *in.OrgID != testOrg || *in.WorkspaceID != testWorkspace || *in.OwnerAccountID != testAccount {
		t.Fatalf("scope = %+v", in)
	}
	want := JobInput{AccountID: testAccount, OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: testConn, SQL: "SELECT 1", Format: "csv", Filename: "report"}
	if in.Input != want {
		t.Fatalf("input = %+v, want %+v", in.Input, want)
	}
	if len(f.audit.events) != 1 || f.audit.events[0].Action != "export.requested" || f.audit.events[0].ResourceID != "30" {
		t.Fatalf("audit = %+v", f.audit.events)
	}
	if _, leaked := f.audit.events[0].Metadata["sql"]; leaked {
		t.Fatal("audit metadata carries SQL")
	}
}

func TestRequestAuditFailureDoesNotFailExport(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.audit.err = errors.New("audit down")

	if _, err := f.svc.Request(context.Background(), principal(testAccount), ref(testWorkspace, testConn), readInput("SELECT 1")); err != nil {
		t.Fatal(err)
	}
}

func TestRequestAllowsExecutePermission(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.policy.revokeAll()
	f.policy.grant(testConn, access.PermConnExecute)

	if _, err := f.svc.Request(context.Background(), principal(testAccount), ref(testWorkspace, testConn), readInput("SELECT 1")); err != nil {
		t.Fatal(err)
	}
}

func TestRequestAndDownloadDenials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    access.Principal
		ref  access.ConnRef
		prep func(f *fixture)
		want error
	}{
		{name: "no permission", p: principal(testAccount), ref: ref(testWorkspace, testConn), prep: func(f *fixture) { f.policy.revokeAll() }, want: access.ErrForbidden},
		{name: "non member", p: principal(99), ref: ref(testWorkspace, testConn), want: access.ErrForbidden},
		{name: "non account subject", p: access.Principal{Subject: access.SubjectRef{Kind: access.SubjectTeam, ID: testAccount}}, ref: ref(testWorkspace, testConn), want: access.ErrForbidden},
		{name: "connection in another workspace", p: principal(testAccount), ref: ref(testWorkspace, foreignConn), want: access.ErrNotFound},
		{name: "workspace does not own connection", p: principal(testAccount), ref: ref(foreignWs, testConn), want: access.ErrNotFound},
		{name: "unknown connection", p: principal(testAccount), ref: ref(testWorkspace, 999), want: access.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.prep != nil {
				tt.prep(f)
			}
			started := false
			var out bytes.Buffer

			_, err := f.svc.Request(context.Background(), tt.p, tt.ref, readInput("SELECT 1"))
			accesstest.AssertDenies(t, err, tt.want, "Request")
			_, err = f.svc.Download(context.Background(), tt.p, tt.ref, "session", readInput("SELECT 1"), &out, func(string) { started = true })
			accesstest.AssertDenies(t, err, tt.want, "Download")
			if started || out.Len() != 0 {
				t.Fatal("download started for a denied caller")
			}
			if len(f.queue.inputs) != 0 || !f.runtime.untouched() || len(f.audit.events) != 0 {
				t.Fatalf("denied call touched collaborators: queue=%d runtime untouched=%v audit=%d", len(f.queue.inputs), f.runtime.untouched(), len(f.audit.events))
			}
		})
	}
}

func TestRequestRejectsInvalidSQL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "mutation", sql: "DELETE FROM widgets", want: "Only read queries can be exported."},
		{name: "multi statement", sql: "SELECT 1; SELECT 2", want: "Only a single query can be exported. Multi-query export isn't supported yet."},
		{name: "empty", sql: "   ", want: "SQL is required."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			var validation *ValidationError

			_, err := f.svc.Request(context.Background(), principal(testAccount), ref(testWorkspace, testConn), readInput(tt.sql))
			if !errors.As(err, &validation) || validation.Fields["sql"] != tt.want {
				t.Fatalf("err = %v, want sql validation %q", err, tt.want)
			}
			_, err = f.svc.Download(context.Background(), principal(testAccount), ref(testWorkspace, testConn), "session", readInput(tt.sql), io.Discard, nil)
			if !errors.As(err, &validation) || validation.Fields["sql"] != tt.want {
				t.Fatalf("download err = %v, want sql validation %q", err, tt.want)
			}
			if len(f.queue.inputs) != 0 || !f.runtime.untouched() {
				t.Fatal("invalid export touched the queue or runtime")
			}
		})
	}
}

func TestRequestValidatesFormatAndFilename(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var validation *ValidationError

	_, err := f.svc.Request(context.Background(), principal(testAccount), ref(testWorkspace, testConn), RequestInput{SQL: "SELECT 1", Format: "xlsx"})
	if !errors.As(err, &validation) || validation.Fields["format"] != "Format must be csv." {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestWithoutRegisteredClassifier(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.policy.grant(32, access.PermConnDQL)

	_, err := f.svc.Request(context.Background(), principal(testAccount), ref(testWorkspace, 32), readInput("SELECT 1"))
	if !errors.Is(err, ErrClassifierUnavailable) {
		t.Fatalf("err = %v, want ErrClassifierUnavailable", err)
	}
}

func TestDownloadStreamsAfterStart(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var out bytes.Buffer
	var filename string
	startedBeforeBytes := false
	f.runtime.streamF = func(_ context.Context, w io.Writer) (exports.StreamResult, error) {
		startedBeforeBytes = filename != ""
		n, err := w.Write([]byte("id\n1\n"))
		return exports.StreamResult{Rows: 1, Bytes: int64(n)}, err
	}

	result, err := f.svc.Download(context.Background(), principal(testAccount), ref(testWorkspace, testConn), "session", readInput("SELECT 1"), &out, func(name string) { filename = name })
	if err != nil {
		t.Fatal(err)
	}
	if filename != "report.csv" || !startedBeforeBytes || out.String() != "id\n1\n" || result.Rows != 1 {
		t.Fatalf("filename=%q startedBeforeBytes=%v out=%q result=%+v", filename, startedBeforeBytes, out.String(), result)
	}
	if got := f.runtime.streams[0].Limits.MaxBytes; got != 100 {
		t.Fatalf("max bytes = %d, want sync limit 100", got)
	}
	if f.runtime.gets[0].AccountID != "7" || f.runtime.gets[0].ConnectionID != "30" {
		t.Fatalf("session scope = %+v", f.runtime.gets[0])
	}
	if len(f.audit.events) != 1 || f.audit.events[0].Metadata["via"] != "download" {
		t.Fatalf("audit = %+v", f.audit.events)
	}
}

func TestDownloadSessionFailuresStartNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	started := false
	onStart := func(string) { started = true }

	_, err := f.svc.Download(context.Background(), principal(testAccount), ref(testWorkspace, testConn), "", readInput("SELECT 1"), io.Discard, onStart)
	if !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
	foreign := &execution.Failure{Code: execution.FailureSessionNotFound}
	f.runtime.getErr = foreign
	_, err = f.svc.Download(context.Background(), principal(testAccount), ref(testWorkspace, testConn), "other-users-session", readInput("SELECT 1"), io.Discard, onStart)
	if !errors.Is(err, foreign) {
		t.Fatalf("err = %v, want the runtime failure", err)
	}
	if started || len(f.runtime.streams) != 0 || len(f.audit.events) != 0 {
		t.Fatal("download started without an owned session")
	}
}

func TestDownloadByteLimitIsReturnedAfterStart(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.runtime.streamF = func(context.Context, io.Writer) (exports.StreamResult, error) {
		return exports.StreamResult{}, exports.ErrByteLimitExceeded
	}
	started := false

	_, err := f.svc.Download(context.Background(), principal(testAccount), ref(testWorkspace, testConn), "session", readInput("SELECT 1"), io.Discard, func(string) { started = true })
	if !errors.Is(err, exports.ErrByteLimitExceeded) || !started {
		t.Fatalf("err = %v, started = %v", err, started)
	}
}

func runtimeFor(input JobInput) jobs.Runtime {
	data, _ := json.Marshal(input)
	return jobs.Runtime{Job: jobs.Record{ID: "job-1", Type: jobs.TypeExportQueryCSV, InputJSON: string(data)}, Events: noEvents{}}
}

type noEvents struct{}

func (noEvents) Info(context.Context, string, string, any)  {}
func (noEvents) Warn(context.Context, string, string, any)  {}
func (noEvents) Error(context.Context, string, string, any) {}

func validJob() JobInput {
	return JobInput{AccountID: testAccount, OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: testConn, SQL: "SELECT 1", Format: "csv", Filename: "report"}
}

func requireCoded(t *testing.T, err error, code string, retryable bool) {
	t.Helper()
	var coded jobs.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("err = %v, want jobs.CodedError %q", err, code)
	}
	if coded.Code != code || coded.Retryable != retryable {
		t.Fatalf("coded = %+v, want %q retryable=%v", coded, code, retryable)
	}
}

func TestRunJobWritesExportFile(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	out, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
	if err != nil {
		t.Fatal(err)
	}
	result, ok := out.(JobOutput)
	if !ok || result.Filename != "report.csv" || result.RowCount != 1 || result.Format != "csv" {
		t.Fatalf("output = %#v", out)
	}
	if string(f.files.content[result.FileID]) != "id\n1\n" {
		t.Fatalf("content = %q", f.files.content[result.FileID])
	}
	if f.files.entries[0].Name != "Exports" || f.files.entries[0].ObjectType != database.FileObjectTypeFolder {
		t.Fatalf("entries = %+v", f.files.entries)
	}
	if len(f.runtime.opens) != 1 || !f.runtime.opens[0].Ephemeral || len(f.runtime.closes) != 1 {
		t.Fatalf("opens=%+v closes=%v", f.runtime.opens, f.runtime.closes)
	}
	if got := f.runtime.streams[0].Limits.MaxBytes; got != 200 {
		t.Fatalf("max bytes = %d, want background limit 200", got)
	}
}

func TestRunJobUsesUniqueFilenames(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	first, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
	if err != nil {
		t.Fatal(err)
	}
	if first.(JobOutput).Filename != "report.csv" || second.(JobOutput).Filename != "report-2.csv" {
		t.Fatalf("filenames = %q, %q", first.(JobOutput).Filename, second.(JobOutput).Filename)
	}
}

func TestRunJobDenials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(in *JobInput, f *fixture)
		code string
	}{
		{name: "access revoked between queue and run", edit: func(_ *JobInput, f *fixture) { f.policy.revokeAll() }, code: "export_not_permitted"},
		{name: "org missing", edit: func(in *JobInput, _ *fixture) { in.OrgID = 404 }, code: "export_not_permitted"},
		{name: "owner left org", edit: func(in *JobInput, _ *fixture) { in.AccountID = 99 }, code: "export_not_permitted"},
		{name: "workspace missing", edit: func(in *JobInput, _ *fixture) { in.WorkspaceID = 404 }, code: "connection_not_found"},
		{name: "connection missing", edit: func(in *JobInput, _ *fixture) { in.ConnectionID = 404 }, code: "connection_not_found"},
		{name: "connection in another workspace", edit: func(in *JobInput, _ *fixture) { in.ConnectionID = foreignConn }, code: "connection_not_found"},
		{name: "mutation", edit: func(in *JobInput, _ *fixture) { in.SQL = "DELETE FROM widgets" }, code: "export_not_read_query"},
		{name: "multi statement", edit: func(in *JobInput, _ *fixture) { in.SQL = "SELECT 1; SELECT 2" }, code: "export_multi_statement"},
		{name: "classifier unavailable", edit: func(in *JobInput, f *fixture) { in.ConnectionID = 32; f.policy.grant(32, access.PermConnDQL) }, code: "export_classifier_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			input := validJob()
			tt.edit(&input, f)

			out, err := f.svc.RunJob(context.Background(), runtimeFor(input))
			if out != nil {
				t.Fatalf("output = %#v", out)
			}
			requireCoded(t, err, tt.code, false)
			if !f.runtime.untouched() || len(f.files.entries) != 0 {
				t.Fatal("denied job opened a session or created files")
			}
		})
	}
}

func TestRunJobInvalidInput(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	_, err := f.svc.RunJob(context.Background(), jobs.Runtime{Job: jobs.Record{InputJSON: "{"}, Events: noEvents{}})
	requireCoded(t, err, "invalid_export_input", false)
	in := validJob()
	in.Format = "xlsx"
	_, err = f.svc.RunJob(context.Background(), runtimeFor(in))
	requireCoded(t, err, "unsupported_export_format", false)
}

func TestRunJobOpenAndStreamErrors(t *testing.T) {
	t.Parallel()
	t.Run("blocked target", func(t *testing.T) {
		f := newFixture(t)
		f.runtime.openErr = settings.ErrSQLiteFileTargetDisabled
		_, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
		requireCoded(t, err, "export_target_blocked", false)
		if len(f.runtime.closes) != 0 {
			t.Fatal("closed a session that never opened")
		}
	})
	t.Run("size limit", func(t *testing.T) {
		f := newFixture(t)
		f.runtime.streamF = func(context.Context, io.Writer) (exports.StreamResult, error) {
			return exports.StreamResult{}, exports.ErrByteLimitExceeded
		}
		_, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
		requireCoded(t, err, "export_too_large", false)
		if len(f.runtime.closes) != 1 {
			t.Fatal("session was not closed")
		}
	})
	t.Run("unsupported driver", func(t *testing.T) {
		f := newFixture(t)
		f.runtime.streamF = func(context.Context, io.Writer) (exports.StreamResult, error) {
			return exports.StreamResult{}, exports.ErrCursorUnsupported
		}
		_, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
		requireCoded(t, err, "export_unsupported_driver", false)
	})
	t.Run("target failure", func(t *testing.T) {
		f := newFixture(t)
		f.runtime.streamF = func(context.Context, io.Writer) (exports.StreamResult, error) {
			return exports.StreamResult{}, errors.New("boom")
		}
		_, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
		requireCoded(t, err, "export_failed", true)
	})
	t.Run("write failure wins over abandoned stream", func(t *testing.T) {
		f := newFixture(t)
		writeErr := errors.New("disk full")
		f.files.writeF = func(r io.Reader) error {
			_, _ = r.Read(make([]byte, 1))
			return writeErr
		}
		f.runtime.streamF = func(_ context.Context, w io.Writer) (exports.StreamResult, error) {
			chunk := make([]byte, 4096)
			for {
				if _, err := w.Write(chunk); err != nil {
					return exports.StreamResult{}, err
				}
			}
		}
		_, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
		if !errors.Is(err, writeErr) {
			t.Fatalf("err = %v, want the write failure", err)
		}
		if len(f.runtime.closes) != 1 {
			t.Fatal("session was not closed")
		}
	})
}

func TestOpenErrorClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		wantCode  string
		wantRetry bool
		wantRaw   error
	}{
		{name: "cancelled during connect", err: &execution.ConnectError{Err: context.Canceled}, wantRaw: context.Canceled},
		{name: "deadline during connect", err: &execution.TunnelError{Err: context.DeadlineExceeded}, wantRaw: context.DeadlineExceeded},
		{name: "connect failure", err: &execution.ConnectError{Err: errors.New("refused")}, wantCode: "export_connect_failed", wantRetry: true},
		{name: "tunnel failure", err: &execution.TunnelError{Err: errors.New("refused")}, wantCode: "export_connect_failed", wantRetry: true},
		{name: "policy denial", err: settings.ErrSQLiteFileTargetDisabled, wantCode: "export_target_blocked"},
		{name: "missing connection", err: fmt.Errorf("execution: %w", credentials.ErrNotFound), wantCode: "connection_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openError(tt.err)
			if tt.wantRaw != nil {
				if !errors.Is(got, tt.wantRaw) {
					t.Fatalf("got %v, want %v", got, tt.wantRaw)
				}
				var coded jobs.CodedError
				if errors.As(got, &coded) {
					t.Fatalf("cancellation was wrapped as job error %q", coded.Code)
				}
				return
			}
			requireCoded(t, got, tt.wantCode, tt.wantRetry)
		})
	}
}

func TestClassifierLookupByDriver(t *testing.T) {
	t.Parallel()
	svc := New(Deps{})
	tests := []struct {
		driver string
		sql    string
		want   error
	}{
		{driver: "postgres", sql: "SELECT 1"},
		{driver: "mysql", sql: "SHOW TABLES"},
		{driver: "postgres", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value"},
		{driver: "mysql", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value"},
		{driver: "postgres", sql: "DELETE FROM widgets", want: errNotReadQuery},
		{driver: "mysql", sql: "SELECT * FROM widgets FOR UPDATE", want: errNotReadQuery},
		{driver: "postgres", sql: "SELECT 1; SELECT 2", want: errMultiStatement},
		{driver: "sqlite", sql: "SELECT 1"},
		{driver: "sqlite", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value"},
		{driver: "sqlite", sql: "DELETE FROM widgets", want: errNotReadQuery},
		{driver: "sqlite", sql: "SELECT 1; SELECT 2", want: errMultiStatement},
		{driver: "driver-with-no-engine", sql: "SELECT 1", want: ErrClassifierUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.driver+" "+tt.sql, func(t *testing.T) {
			if got := svc.classify(context.Background(), tt.driver, tt.sql); !errors.Is(got, tt.want) {
				t.Fatalf("classify = %v, want %v", got, tt.want)
			}
		})
	}
}

func debugLogs(f *fixture) *bytes.Buffer {
	var buf bytes.Buffer
	f.svc.deps.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &buf
}

func TestDownloadByteLimitAbortIsLogged(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	logs := debugLogs(f)
	f.runtime.streamF = func(context.Context, io.Writer) (exports.StreamResult, error) {
		return exports.StreamResult{Rows: 3, Bytes: 100}, exports.ErrByteLimitExceeded
	}

	_, err := f.svc.Download(context.Background(), principal(testAccount), ref(testWorkspace, testConn), "session", readInput("SELECT secret_column FROM t"), io.Discard, nil)
	if !errors.Is(err, exports.ErrByteLimitExceeded) {
		t.Fatalf("err = %v", err)
	}
	out := logs.String()
	if !strings.Contains(out, "export download aborted at byte limit") || !strings.Contains(out, "level=DEBUG") {
		t.Fatalf("logs = %q, want a debug entry for the byte-limit abort", out)
	}
	if strings.Contains(out, "secret_column") {
		t.Fatalf("logs = %q, must not include SQL text", out)
	}
}

func TestRunJobLogsEarlyPipeClose(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	logs := debugLogs(f)
	f.files.writeF = func(r io.Reader) error {
		_, _ = r.Read(make([]byte, 1))
		return errors.New("disk full")
	}
	f.runtime.streamF = func(_ context.Context, w io.Writer) (exports.StreamResult, error) {
		chunk := make([]byte, 4096)
		for {
			if _, err := w.Write(chunk); err != nil {
				return exports.StreamResult{}, err
			}
		}
	}

	if _, err := f.svc.RunJob(context.Background(), runtimeFor(validJob())); err == nil {
		t.Fatal("want the write failure")
	}
	if out := logs.String(); !strings.Contains(out, "export stream stopped after the file write ended early") {
		t.Fatalf("logs = %q, want a debug entry for the early pipe close", out)
	}
}

func TestRunJobLogsByteLimitAbort(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	logs := debugLogs(f)
	f.runtime.streamF = func(context.Context, io.Writer) (exports.StreamResult, error) {
		return exports.StreamResult{}, exports.ErrByteLimitExceeded
	}

	_, err := f.svc.RunJob(context.Background(), runtimeFor(validJob()))
	requireCoded(t, err, "export_too_large", false)
	if out := logs.String(); !strings.Contains(out, "export job aborted at byte limit") {
		t.Fatalf("logs = %q, want a debug entry for the byte-limit abort", out)
	}
}
