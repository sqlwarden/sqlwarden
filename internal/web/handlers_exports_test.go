package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/filestore"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/settings"
)

func TestConnectionExportsUnavailableWithoutRegisteredClassifier(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "export-unavailable"), "Export Unavailable", "Export Unavailable Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Unavailable WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	// Every registered engine ships a classifier, so exercise the unavailable
	// path with a connection whose driver name resolves to no engine.
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "driver-with-no-engine", "ExportUnavailableConn", "open")
	baseURL := orgConnectionURL(org.Slug, ws.ID, envID, fmt.Sprintf("%d", conn.ID))

	for _, path := range []string{"/exports", "/exports/download"} {
		res := send(t, newAuthRequest(t, http.MethodPost, baseURL+path,
			map[string]any{"sql": "SELECT 1 AS id", "filename": "report"}, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
	}

	count, err := app.db.NewSelect().Model((*jobs.Record)(nil)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, 0)
}

func TestHandleExportJobFailsBeforeTargetAccessWithoutRegisteredClassifier(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account, _, org := seedOrgOwner(t, app, uniqueEmail(t, "export-worker-unavailable"), "Export Worker Unavailable", "Export Worker Unavailable Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Worker Unavailable WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "driver-with-no-engine", "Export Worker Unavailable Conn", "open")

	inputJSON, err := json.Marshal(exportJobInput{
		AccountID:    account.ID,
		OrgID:        org.ID,
		WorkspaceID:  ws.ID,
		ConnectionID: conn.ID,
		SQL:          "SELECT 7 AS id",
		Format:       "csv",
		Filename:     "worker-export",
	})
	if err != nil {
		t.Fatal(err)
	}

	output, err := app.handleExportJob(context.Background(), jobs.Runtime{
		Job:    jobs.Record{ID: database.NewID(), Type: jobs.TypeExportQueryCSV, InputJSON: string(inputJSON)},
		Events: recordingEventWriter{},
	})
	if output != nil {
		t.Fatalf("output = %#v, want nil", output)
	}
	var coded jobs.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("error = %v, want jobs.CodedError", err)
	}
	assert.Equal(t, coded.Code, "export_classifier_unavailable")
	assert.Equal(t, coded.Retryable, false)
}

func TestRegisteredClassifierIsRequiredForExportValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	ok := app.validateExportSQL(recorder, req, database.Connection{Driver: "driver-with-no-engine"}, "SELECT 1")
	assert.Equal(t, ok, false)
	assert.Equal(t, recorder.Code, http.StatusNotImplemented)
}

func TestOmniClassifierExportValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tests := []struct {
		name       string
		driver     string
		sql        string
		wantOK     bool
		wantStatus int
	}{
		{name: "postgres read", driver: "postgres", sql: "SELECT 1", wantOK: true},
		{name: "mysql read", driver: "mysql", sql: "SHOW TABLES", wantOK: true},
		{name: "postgres read CTE", driver: "postgres", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value", wantOK: true},
		{name: "mysql read CTE", driver: "mysql", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value", wantOK: true},
		{name: "postgres mutation", driver: "postgres", sql: "DELETE FROM widgets", wantStatus: http.StatusUnprocessableEntity},
		{name: "mysql locking read", driver: "mysql", sql: "SELECT * FROM widgets FOR UPDATE", wantStatus: http.StatusUnprocessableEntity},
		{name: "postgres multi query", driver: "postgres", sql: "SELECT 1; SELECT 2", wantStatus: http.StatusUnprocessableEntity},
		{name: "mysql invalid", driver: "mysql", sql: "SELECT FROM", wantStatus: http.StatusUnprocessableEntity},
		{name: "sqlite read", driver: "sqlite", sql: "SELECT 1", wantOK: true},
		{name: "sqlite read CTE", driver: "sqlite", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value", wantOK: true},
		{name: "sqlite mutation", driver: "sqlite", sql: "DELETE FROM widgets", wantStatus: http.StatusUnprocessableEntity},
		{name: "sqlite multi query", driver: "sqlite", sql: "SELECT 1; SELECT 2", wantStatus: http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			got := app.validateExportSQL(recorder, req, database.Connection{Driver: tt.driver}, tt.sql)
			if got != tt.wantOK {
				t.Fatalf("validateExportSQL() = %v, want %v", got, tt.wantOK)
			}
			if !tt.wantOK && recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

type recordingEventWriter struct{}

func (recordingEventWriter) Info(context.Context, string, string, any)  {}
func (recordingEventWriter) Warn(context.Context, string, string, any)  {}
func (recordingEventWriter) Error(context.Context, string, string, any) {}

type lifecycleRuntime struct {
	execution.Runtime
	mu        sync.Mutex
	opens     []execution.OpenRequest
	closes    []execution.SessionID
	closeErrs []error
	streamFn  func(ctx context.Context, w io.Writer) error
	probes    []probeCall
}

type probeCall struct {
	scope execution.Scope
	creds credentials.Credentials
}

func (l *lifecycleRuntime) Open(ctx context.Context, req execution.OpenRequest) (execution.SessionInfo, error) {
	l.mu.Lock()
	l.opens = append(l.opens, req)
	l.mu.Unlock()
	return l.Runtime.Open(ctx, req)
}

func (l *lifecycleRuntime) Close(ctx context.Context, scope execution.Scope, id execution.SessionID) error {
	l.mu.Lock()
	l.closes = append(l.closes, id)
	l.closeErrs = append(l.closeErrs, ctx.Err())
	l.mu.Unlock()
	return l.Runtime.Close(ctx, scope, id)
}

func (l *lifecycleRuntime) Stream(ctx context.Context, scope execution.Scope, id execution.SessionID, req execution.StreamRequest, w io.Writer) (exports.StreamResult, error) {
	if l.streamFn != nil {
		return exports.StreamResult{}, l.streamFn(ctx, w)
	}
	return l.Runtime.Stream(ctx, scope, id, req, w)
}

func (l *lifecycleRuntime) Probe(ctx context.Context, scope execution.Scope, creds credentials.Credentials, limits execution.Limits, fn func(metadata.SchemaInspector) error) error {
	l.mu.Lock()
	l.probes = append(l.probes, probeCall{scope: scope, creds: creds})
	l.mu.Unlock()
	return l.Runtime.Probe(ctx, scope, creds, limits, fn)
}

func installLifecycleRuntime(app *application) *lifecycleRuntime {
	rt := &lifecycleRuntime{Runtime: app.runtime}
	app.runtime = rt
	return rt
}

func seedExportJobInput(t *testing.T, app *application, sql string) (exportJobInput, jobs.Runtime) {
	t.Helper()
	account, _, org := seedOrgOwner(t, app, uniqueEmail(t, "export-lifecycle"), "Export Lifecycle", "Export Lifecycle Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Lifecycle WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	encryptedDSN, err := app.keyring.Encrypt(filepath.Join(t.TempDir(), "export.db"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := app.db.InsertConnection(context.Background(), ws.ID, &envID, "Export Lifecycle Conn", "sqlite", encryptedDSN, "open")
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer.InvalidateAncestry("connection", conn.ID)
	input := exportJobInput{
		AccountID: account.ID, OrgID: org.ID, WorkspaceID: ws.ID, ConnectionID: conn.ID,
		SQL: sql, Format: "csv", Filename: "lifecycle",
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return input, jobs.Runtime{
		Job:    jobs.Record{ID: database.NewID(), Type: jobs.TypeExportQueryCSV, InputJSON: string(inputJSON)},
		Events: recordingEventWriter{},
	}
}

func requireSingleClosedEphemeralSession(t *testing.T, rt *lifecycleRuntime, input exportJobInput) {
	t.Helper()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	assert.Equal(t, len(rt.opens), 1)
	assert.Equal(t, rt.opens[0].Ephemeral, true)
	assert.Equal(t, rt.opens[0].Scope, execution.Scope{
		OrgID:        strconv.FormatInt(input.OrgID, 10),
		WorkspaceID:  strconv.FormatInt(input.WorkspaceID, 10),
		AccountID:    strconv.FormatInt(input.AccountID, 10),
		ConnectionID: strconv.FormatInt(input.ConnectionID, 10),
	})
	assert.Equal(t, rt.opens[0].Limits, execution.Limits{})
	assert.Equal(t, len(rt.closes), 1)
	if rt.closeErrs[0] != nil {
		t.Fatalf("session was closed with a cancelled context: %v", rt.closeErrs[0])
	}
}

func TestHandleExportJobClosesEphemeralSessionOnSuccess(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rt := installLifecycleRuntime(app)
	input, job := seedExportJobInput(t, app, "SELECT 1 AS id")

	output, err := app.handleExportJob(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := output.(exportJobOutput)
	if !ok {
		t.Fatalf("output = %#v, want exportJobOutput", output)
	}
	assert.Equal(t, result.RowCount, int64(1))
	requireSingleClosedEphemeralSession(t, rt, input)
}

func TestHandleExportJobClosesEphemeralSessionOnStreamError(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rt := installLifecycleRuntime(app)
	input, job := seedExportJobInput(t, app, "SELECT * FROM table_that_does_not_exist")

	output, err := app.handleExportJob(context.Background(), job)
	if err == nil || output != nil {
		t.Fatalf("output = %#v, err = %v, want failure", output, err)
	}
	requireSingleClosedEphemeralSession(t, rt, input)
}

func TestHandleExportJobClosesEphemeralSessionOnCancel(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rt := installLifecycleRuntime(app)
	input, job := seedExportJobInput(t, app, "SELECT 1 AS id")

	ctx, cancel := context.WithCancel(context.Background())
	rt.streamFn = func(ctx context.Context, _ io.Writer) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}
	_, err := app.handleExportJob(ctx, job)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	requireSingleClosedEphemeralSession(t, rt, input)
}

func TestHandleExportJobBlockedTargetOpensNoSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rt := installLifecycleRuntime(app)
	_, job := seedExportJobInput(t, app, "SELECT 1 AS id")
	updateInstanceSettingsForTest(t, app, func(s *database.InstanceSettings) {
		s.SQLiteLocalTargetsEnabled = false
	})

	_, err := app.handleExportJob(context.Background(), job)
	var coded jobs.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("error = %v, want jobs.CodedError", err)
	}
	assert.Equal(t, coded.Code, "export_target_blocked")
	assert.Equal(t, coded.Retryable, false)
	assert.Equal(t, len(rt.closes), 0)
}

func TestTestConnectionProbesWithCallerCredentials(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rt := installLifecycleRuntime(app)

	_, tok, slug := registerAndLogin(t, app, "conn-test-probe@example.com", "Conn Test Probe", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Probe WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID, _ := strconv.ParseInt(fmt.Sprintf("%v", wsRes.BodyFields["id"]), 10, 64)
	envID := defaultEnvironmentID(t, app, wsID)

	res := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(slug, wsID, envID)+"/test",
		map[string]any{"driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["ok"], true)

	assert.Equal(t, len(rt.probes), 1)
	assert.Equal(t, rt.probes[0].creds.Driver, "sqlite")
	assert.Equal(t, rt.probes[0].creds.DSN, ":memory:")
	assert.Equal(t, rt.probes[0].scope.ConnectionID, "")
	assert.Equal(t, rt.probes[0].scope.WorkspaceID, strconv.FormatInt(wsID, 10))
	assert.Equal(t, len(rt.opens), 0)
}

type failingPutStores struct{ FileStores }

func (f failingPutStores) Store(ctx context.Context, backendID string) (filestore.Store, error) {
	store, err := f.FileStores.Store(ctx, backendID)
	if err != nil {
		return nil, err
	}
	return failingPutStore{Store: store}, nil
}

type failingPutStore struct{ filestore.Store }

var errTestDiskFull = errors.New("disk full")

func (failingPutStore) Put(_ context.Context, _ string, content io.Reader) (filestore.StoredObject, error) {
	_, _ = content.Read(make([]byte, 1))
	return filestore.StoredObject{}, errTestDiskFull
}

func TestHandleExportJobAbandonedPipeStopsStreamAndClosesSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rt := installLifecycleRuntime(app)
	app.fileStores = failingPutStores{FileStores: app.fileStores}
	input, job := seedExportJobInput(t, app, "SELECT 1 AS id")

	streamDone := make(chan struct{})
	rt.streamFn = func(_ context.Context, w io.Writer) error {
		defer close(streamDone)
		chunk := make([]byte, 4096)
		for {
			if _, err := w.Write(chunk); err != nil {
				return err
			}
		}
	}
	_, err := app.handleExportJob(context.Background(), job)
	if !errors.Is(err, errTestDiskFull) {
		t.Fatalf("err = %v, want the write failure", err)
	}
	select {
	case <-streamDone:
	default:
		t.Fatal("stream goroutine was still running when the job returned")
	}
	requireSingleClosedEphemeralSession(t, rt, input)
}

func TestExportOpenErrorClassification(t *testing.T) {
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
			got := exportOpenError(tt.err)
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
			var coded jobs.CodedError
			if !errors.As(got, &coded) {
				t.Fatalf("got %v, want jobs.CodedError", got)
			}
			assert.Equal(t, coded.Code, tt.wantCode)
			assert.Equal(t, coded.Retryable, tt.wantRetry)
		})
	}
}

func TestTestConnectionFailureDoesNotEchoCallerSecrets(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "conn-test-secret@example.com", "Conn Test Secret", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Secret WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID, _ := strconv.ParseInt(fmt.Sprintf("%v", wsRes.BodyFields["id"]), 10, 64)
	envID := defaultEnvironmentID(t, app, wsID)

	const secret = "s3cr3t-Passw0rd-xyz"
	for name, dsn := range map[string]string{
		"postgres keyword": "host=localhost port=19999 user=test password=" + secret + " dbname=test sslmode=disable connect_timeout=1",
		"postgres url":     "postgres://test:" + secret + "@localhost:19999/test?sslmode=disable&connect_timeout=1",
		"mysql":            "test:" + secret + "@tcp(localhost:19999)/test?timeout=1s",
	} {
		driver := "postgres"
		if name == "mysql" {
			driver = "mysql"
		}
		res := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(slug, wsID, envID)+"/test",
			map[string]any{"driver": driver, "dsn": dsn}, tok), app.routes())
		if strings.Contains(string(res.BodyBytes), secret) {
			t.Fatalf("%s: response echoes the caller password: %s", name, res.BodyBytes)
		}
	}
}
