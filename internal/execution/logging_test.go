package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/observability"
)

type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *logCapture) records(t *testing.T) []map[string]any {
	t.Helper()
	var records []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(c.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

// find returns the single record with msg whose attributes include every
// key/value in match.
func (c *logCapture) find(t *testing.T, msg string, match map[string]any) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, record := range c.records(t) {
		if record["msg"] != msg {
			continue
		}
		ok := true
		for key, want := range match {
			if record[key] != want {
				ok = false
				break
			}
		}
		if ok {
			found = append(found, record)
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d %q records matching %v, want 1; logs:\n%s", len(found), msg, match, c.String())
	}
	return found[0]
}

func (c *logCapture) assertAbsent(t *testing.T, values ...string) {
	t.Helper()
	output := c.String()
	for _, value := range values {
		if value != "" && strings.Contains(output, value) {
			t.Fatalf("logs contain forbidden value %q:\n%s", value, output)
		}
	}
}

func newCapturedLogger() (*slog.Logger, *logCapture) {
	capture := &logCapture{}
	return slog.New(slog.NewJSONHandler(capture, &slog.HandlerOptions{Level: slog.LevelDebug})), capture
}

type loggedConnector struct {
	server    *httptest.Server
	authority *execution.GrantAuthority
	local     execution.SessionRuntime
	logs      *logCapture
}

func newLoggedConnector(t *testing.T) loggedConnector {
	t.Helper()
	logger, logs := newCapturedLogger()
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	local := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), sqliteCredentials(t, "4"), nil, time.Minute, execution.WithLogger(logger))
	authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	serverRuntime, err := execution.NewRuntimeServer(local, authority, execution.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(serverRuntime.Handler())
	t.Cleanup(func() {
		server.Close()
		cursors.Close()
		sessions.Close()
	})
	return loggedConnector{server: server, authority: authority, local: local, logs: logs}
}

func (c loggedConnector) worker(t *testing.T, opts ...execution.Option) *execution.WorkerRuntime {
	t.Helper()
	worker, err := execution.NewWorkerRuntime(
		execution.NewStaticSessionDirectory(strings.TrimPrefix(c.server.URL, "http://")),
		c.authority,
		execution.InsecureTransportCredentials{Client: c.server.Client()},
		opts...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func (c loggedConnector) post(t *testing.T, body string) {
	t.Helper()
	response, err := c.server.Client().Post(c.server.URL+"/internal/execution/v2/call", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

var testScope = execution.Scope{TenantID: "1", AccountID: "2", WorkspaceID: "3", ConnectionID: "4"}

func TestRuntimeServerLogsSuccessfulOutcomesAtDebugWithRequestID(t *testing.T) {
	connector := newLoggedConnector(t)
	worker := connector.worker(t)
	ctx := observability.WithRequestID(context.Background(), "req-success-1")

	opened, err := worker.Open(ctx, execution.OpenRequest{Scope: testScope})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Query(ctx, execution.QueryRequest{
		Handle: opened.Handle, SQL: "SELECT 'sql-secret-marker' AS v WHERE 1 = ?", Args: []any{1},
	}); err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{"open", "query"} {
		record := connector.logs.find(t, "execution rpc completed", map[string]any{"rpc_method": method})
		if record["level"] != "DEBUG" || record["outcome"] != "success" || record["request_id"] != "req-success-1" {
			t.Fatalf("%s outcome record = %v", method, record)
		}
		if _, ok := record["duration_ms"]; !ok {
			t.Fatalf("%s outcome record missing duration_ms: %v", method, record)
		}
	}
	opens := connector.logs.find(t, "execution session opened", nil)
	if opens["level"] != "INFO" || opens["request_id"] != "req-success-1" || opens["driver"] != "sqlite" ||
		opens["connection_id"] != "4" || opens["reused"] != false || opens["ephemeral"] != false {
		t.Fatalf("session opened record = %v", opens)
	}
	connector.logs.assertAbsent(t, "sql-secret-marker", string(opened.Handle), "signature", ".db")
}

func TestRuntimeServerLogsRejectedRequestsAtWarn(t *testing.T) {
	connector := newLoggedConnector(t)
	opened, err := connector.local.Open(context.Background(), execution.OpenRequest{Scope: testScope})
	if err != nil {
		t.Fatal(err)
	}

	connector.post(t, `{not json`)
	connector.assertWarnOutcome(t, "unknown", "invalid_request")

	connector.post(t, `{"method":"drop-everything-marker","payload":{}}`)
	connector.assertWarnOutcome(t, "unknown", "unknown_method")

	connector.post(t, `{"method":"query","payload":{"sql":"SELECT 'payload-secret-marker'","unexpected":true}}`)
	connector.assertWarnOutcome(t, "query", "invalid_request")

	foreign, err := connector.authority.Issue(execution.Scope{TenantID: "1", AccountID: "99", WorkspaceID: "3", ConnectionID: "4"}, []string{"query"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"session_handle": opened.Handle, "grant": foreign, "sql": "SELECT 'grant-secret-marker'"})
	if err != nil {
		t.Fatal(err)
	}
	connector.post(t, `{"method":"execute","payload":`+string(payload)+`}`)
	connector.assertWarnOutcome(t, "execute", "unauthorized")

	connector.logs.assertAbsent(t, "drop-everything-marker", "payload-secret-marker", "grant-secret-marker", foreign.Signature, string(opened.Handle))
}

func (c loggedConnector) assertWarnOutcome(t *testing.T, method, outcome string) {
	t.Helper()
	record := c.logs.find(t, "execution rpc completed", map[string]any{"rpc_method": method, "outcome": outcome})
	if record["level"] != "WARN" {
		t.Fatalf("%s/%s level = %v, want WARN", method, outcome, record["level"])
	}
}

func TestRuntimeServerLogsApplicationFailuresWithoutDriverErrors(t *testing.T) {
	connector := newLoggedConnector(t)
	worker := connector.worker(t)
	ctx := context.Background()
	opened, err := worker.Open(ctx, execution.OpenRequest{Scope: testScope})
	if err != nil {
		t.Fatal(err)
	}
	_, err = worker.Query(ctx, execution.QueryRequest{Handle: opened.Handle, SQL: "SELECT * FROM missing_table_secret_marker"})
	if err == nil {
		t.Fatal("Query() error = nil, want driver failure")
	}

	record := connector.logs.find(t, "execution rpc completed", map[string]any{"rpc_method": "query"})
	if record["outcome"] != "application_error" || record["level"] != "DEBUG" {
		t.Fatalf("query failure record = %v", record)
	}
	connector.logs.assertAbsent(t, "missing_table_secret_marker", "no such table")
}

func TestWorkerRuntimeDoesNotDuplicateRemoteApplicationFailures(t *testing.T) {
	connector := newLoggedConnector(t)
	logger, workerLogs := newCapturedLogger()
	worker := connector.worker(t, execution.WithLogger(logger))
	ctx := context.Background()
	opened, err := worker.Open(ctx, execution.OpenRequest{Scope: testScope})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Query(ctx, execution.QueryRequest{Handle: opened.Handle, SQL: "SELECT * FROM missing_table"}); err == nil {
		t.Fatal("Query() error = nil, want remote failure")
	}
	var sink bytes.Buffer
	if _, err := worker.Stream(ctx, execution.SessionRequest{Handle: opened.Handle}, &sink, exports.StreamOptions{
		Format: "parquet", SQL: "SELECT 1", MaxBytes: 1 << 20,
	}); !errors.Is(err, exports.ErrUnsupportedFormat) {
		t.Fatalf("Stream() error = %v, want unsupported format", err)
	}
	if output := workerLogs.String(); output != "" {
		t.Fatalf("worker logged remote application failures:\n%s", output)
	}
}

func TestWorkerRuntimeForwardsRequestIDAndLogsTransportFailures(t *testing.T) {
	var mu sync.Mutex
	var forwarded []string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		forwarded = append(forwarded, request.URL.Path+" "+request.Header.Get(observability.RequestIDHeader))
		mu.Unlock()
		return nil, errors.New("dial tcp 10.9.8.7:6021: raw-transport-secret")
	})}
	authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	logger, logs := newCapturedLogger()
	worker, err := execution.NewWorkerRuntime(
		execution.NewStaticSessionDirectory("http://connector-address-marker:6021"),
		authority,
		execution.InsecureTransportCredentials{Client: client},
		execution.WithLogger(logger),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithRequestID(context.Background(), "req-forward-1")
	request := execution.SessionRequest{Handle: "opaque-session-marker", Grant: execution.Grant{Scope: testScope}}

	if _, err := worker.Execute(ctx, execution.ExecuteRequest{Handle: request.Handle, Grant: request.Grant, SQL: "UPDATE t SET v = 'sql-secret'"}); err == nil {
		t.Fatal("Execute() error = nil, want transport failure")
	}
	if _, err := worker.Stream(ctx, request, io.Discard, exports.StreamOptions{Format: exports.FormatCSV, SQL: "SELECT 1"}); err == nil {
		t.Fatal("Stream() error = nil, want transport failure")
	}

	want := []string{"/internal/execution/v2/call req-forward-1", "/internal/execution/v2/stream req-forward-1"}
	if strings.Join(forwarded, ",") != strings.Join(want, ",") {
		t.Fatalf("forwarded requests = %v, want %v", forwarded, want)
	}
	for _, method := range []string{"execute", "stream"} {
		record := logs.find(t, "execution transport failed", map[string]any{"rpc_method": method})
		if record["level"] != "WARN" || record["failure_category"] != "unreachable" || record["request_id"] != "req-forward-1" {
			t.Fatalf("%s transport record = %v", method, record)
		}
	}
	logs.assertAbsent(t, "connector-address-marker", "10.9.8.7", "raw-transport-secret", "sql-secret", "opaque-session-marker")
}

func TestRuntimeServerPropagatesOnlyValidRequestIDs(t *testing.T) {
	connector := newLoggedConnector(t)
	request, err := http.NewRequest(http.MethodPost, connector.server.URL+"/internal/execution/v2/call", strings.NewReader(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(observability.RequestIDHeader, "bad id\" level=ERROR")
	response, err := connector.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	record := connector.logs.find(t, "execution rpc completed", map[string]any{"outcome": "invalid_request"})
	if _, ok := record["request_id"]; ok {
		t.Fatalf("invalid request ID was logged: %v", record)
	}
}

type failingCredentialProvider struct{ err error }

func (p failingCredentialProvider) Resolve(context.Context, string) (execution.Credentials, error) {
	return execution.Credentials{}, p.err
}

type failingStreamWriter struct{}

func (failingStreamWriter) Write([]byte) (int, error) {
	return 0, errors.New("browser-write-secret")
}

func TestWorkerRuntimeLogsClientStreamWriteFailuresAtDebug(t *testing.T) {
	connector := newLoggedConnector(t)
	logger, workerLogs := newCapturedLogger()
	worker := connector.worker(t, execution.WithLogger(logger))
	opened, err := worker.Open(t.Context(), execution.OpenRequest{Scope: testScope})
	if err != nil {
		t.Fatal(err)
	}
	_, err = worker.Stream(t.Context(), execution.SessionRequest{Handle: opened.Handle}, failingStreamWriter{}, exports.StreamOptions{
		Format: exports.FormatCSV, SQL: "SELECT 'stream-sql-secret'", MaxBytes: 1 << 20,
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want client write failure")
	}
	record := workerLogs.find(t, "execution transport failed", map[string]any{"rpc_method": "stream"})
	if record["level"] != "DEBUG" || record["failure_category"] != "client_write" {
		t.Fatalf("stream client-write record = %v", record)
	}
	workerLogs.assertAbsent(t, "browser-write-secret", "stream-sql-secret", string(opened.Handle))
}

func TestLocalRuntimeLogsSafeOpenFailures(t *testing.T) {
	const dsnMarker = "postgres://admin:dsn-secret-marker@db.internal/app"
	tests := []struct {
		name      string
		provider  execution.CredentialProvider
		validator execution.TargetValidator
		stage     string
		category  string
		level     string
		driver    any
	}{
		{
			name:     "credentials not found",
			provider: staticCredentialProvider{},
			stage:    "credentials", category: "credentials_not_found", level: "WARN", driver: nil,
		},
		{
			name:     "credential decryption",
			provider: failingCredentialProvider{err: errors.Join(execution.ErrCredentialDecryption, errors.New("cipher-secret-marker"))},
			stage:    "credentials", category: "credential_decryption", level: "ERROR", driver: nil,
		},
		{
			name:     "target policy rejection",
			provider: staticCredentialProvider{"4": {Driver: "sqlite", DSN: dsnMarker}},
			validator: targetValidatorFunc(func(context.Context, string, string) error {
				return errors.Join(execution.ErrTargetRejected, errors.New(dsnMarker))
			}),
			stage: "target_policy", category: "target_rejected", level: "WARN", driver: "sqlite",
		},
		{
			name:     "unsupported driver",
			provider: staticCredentialProvider{"4": {Driver: "no-such-driver", DSN: dsnMarker}},
			stage:    "driver", category: "unsupported_driver", level: "ERROR", driver: "no-such-driver",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs := newCapturedLogger()
			sessions := connection.New(time.Minute)
			cursors := connection.NewQueryCursorManager(time.Minute)
			t.Cleanup(func() {
				cursors.Close()
				sessions.Close()
			})
			runtime := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), tt.provider, tt.validator, time.Minute, execution.WithLogger(logger))
			ctx := observability.WithRequestID(context.Background(), "req-open-1")
			if _, err := runtime.Open(ctx, execution.OpenRequest{Scope: testScope, Ephemeral: true}); err == nil {
				t.Fatal("Open() error = nil, want failure")
			}

			record := logs.find(t, "execution session open failed", nil)
			if record["stage"] != tt.stage || record["failure_category"] != tt.category || record["level"] != tt.level ||
				record["driver"] != tt.driver || record["connection_id"] != "4" || record["org_id"] != "1" ||
				record["ephemeral"] != true || record["request_id"] != "req-open-1" {
				t.Fatalf("open failure record = %v", record)
			}
			logs.assertAbsent(t, "dsn-secret-marker", "cipher-secret-marker", "db.internal")
		})
	}
}

func TestLocalRuntimeLogsReusedSessionsAtDebug(t *testing.T) {
	logger, logs := newCapturedLogger()
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	t.Cleanup(func() {
		cursors.Close()
		sessions.Close()
	})
	runtime := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), sqliteCredentials(t, "4"), nil, time.Minute, execution.WithLogger(logger))
	ctx := context.Background()
	if _, err := runtime.Open(ctx, execution.OpenRequest{Scope: testScope}); err != nil {
		t.Fatal(err)
	}
	reused, err := runtime.Open(ctx, execution.OpenRequest{Scope: testScope})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Query(ctx, execution.QueryRequest{Handle: reused.Handle, SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}

	record := logs.find(t, "execution session opened", map[string]any{"reused": true})
	if record["level"] != "DEBUG" {
		t.Fatalf("reused session record = %v, want DEBUG", record)
	}
	for _, record := range logs.records(t) {
		if record["level"] == "INFO" && record["msg"] != "execution session opened" {
			t.Fatalf("unexpected info log for query operation: %v", record)
		}
	}
	logs.assertAbsent(t, string(reused.Handle))
}
