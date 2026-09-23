package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/execution/executiontest"
	"github.com/sqlwarden/internal/exports"
)

const testGrantKey = "test-execution-grant-key-32bytes!!"

func TestWorkerRuntimeContract(t *testing.T) {
	executiontest.RunRuntimeContract(t, func(t testing.TB) (execution.SessionRuntime, execution.SessionDirectory) {
		sessions := connection.New(time.Minute)
		cursors := connection.NewQueryCursorManager(time.Minute)
		localDirectory := execution.NewMemorySessionDirectory()
		local := execution.NewLocalRuntime(sessions, cursors, localDirectory, sqliteCredentials(t, "44"), nil, time.Minute)
		authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		serverRuntime, err := execution.NewRuntimeServer(local, authority)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(serverRuntime.Handler())
		t.Cleanup(func() {
			server.Close()
			cursors.Close()
			sessions.Close()
		})

		worker, err := execution.NewWorkerRuntime(
			execution.NewStaticSessionDirectory(strings.TrimPrefix(server.URL, "http://")),
			authority,
			execution.InsecureTransportCredentials{Client: server.Client()},
		)
		if err != nil {
			t.Fatal(err)
		}
		return worker, localDirectory
	})
}

func TestWorkerRuntimeDoesNotReplayAmbiguousWrite(t *testing.T) {
	var calls atomic.Int32
	var requestURL string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		requestURL = request.URL.String()
		return nil, errors.New("connection dropped after write")
	})}
	authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := execution.NewWorkerRuntime(
		execution.NewStaticSessionDirectory("connector.internal:6021"),
		authority,
		execution.InsecureTransportCredentials{Client: client},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = worker.Execute(t.Context(), execution.ExecuteRequest{
		Handle: "opaque-session",
		Grant:  execution.Grant{Scope: execution.Scope{ConnectionID: "42"}},
		SQL:    "UPDATE widgets SET name = 'changed'",
	})
	var failure *execution.Failure
	if !errors.As(err, &failure) || failure.Code != execution.FailureExecutionOutcomeUnknown {
		t.Fatalf("Execute() error = %v, want execution_outcome_unknown", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls = %d, want exactly 1", got)
	}
	if requestURL != "http://connector.internal:6021/internal/execution/v2/call" {
		t.Fatalf("request URL = %q, want connector RPC URL", requestURL)
	}
}

func TestWorkerRuntimeRejectsIncompatibleDirectoryProtocol(t *testing.T) {
	directory := execution.NewMemorySessionDirectory()
	handle := execution.SessionHandle("old-runtime-session")
	scope := execution.Scope{ConnectionID: "42"}
	now := time.Now()
	if err := directory.Put(t.Context(), execution.DirectoryRecord{
		Handle: handle, Scope: scope, OwnerRuntimeID: "old-runtime", RoutingAddress: "connector.internal:6021",
		CreatedAt: now, LastSeenAt: now, LeaseExpiresAt: now.Add(time.Minute), ProtocolVersion: execution.ProtocolVersion - 1,
	}); err != nil {
		t.Fatal(err)
	}
	authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := execution.NewWorkerRuntime(directory, authority, execution.InsecureTransportCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = worker.Query(t.Context(), execution.QueryRequest{Handle: handle, Grant: execution.Grant{Scope: scope}, SQL: "SELECT 1"})
	if !errors.Is(err, execution.ErrProtocolMismatch) {
		t.Fatalf("Query() error = %v, want protocol mismatch", err)
	}
}

func TestWorkerRuntimeDoesNotRequireAPIReplicaAffinity(t *testing.T) {
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	local := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), sqliteCredentials(t, "4"), nil, time.Minute)
	authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	serverRuntime, err := execution.NewRuntimeServer(local, authority)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(serverRuntime.Handler())
	t.Cleanup(func() {
		server.Close()
		cursors.Close()
		sessions.Close()
	})
	directory := execution.NewStaticSessionDirectory(strings.TrimPrefix(server.URL, "http://"))
	credentials := execution.InsecureTransportCredentials{Client: server.Client()}
	first, err := execution.NewWorkerRuntime(directory, authority, credentials)
	if err != nil {
		t.Fatal(err)
	}
	second, err := execution.NewWorkerRuntime(directory, authority, credentials)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	opened, err := first.Open(ctx, execution.OpenRequest{
		Scope: execution.Scope{TenantID: "1", AccountID: "2", WorkspaceID: "3", ConnectionID: "4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := second.Query(ctx, execution.QueryRequest{Handle: opened.Handle, SQL: "SELECT 1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || len(result.Result.Rows) != 1 {
		t.Fatalf("second API replica Query() = %+v", result)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newConnectorFixture(t *testing.T) (*httptest.Server, *execution.GrantAuthority, execution.SessionRuntime) {
	t.Helper()
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	local := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), sqliteCredentials(t, "4"), nil, time.Minute)
	authority, err := execution.NewGrantAuthority([]byte(testGrantKey), "api", "connector", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	serverRuntime, err := execution.NewRuntimeServer(local, authority)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(serverRuntime.Handler())
	t.Cleanup(func() {
		server.Close()
		cursors.Close()
		sessions.Close()
	})
	return server, authority, local
}

func TestRuntimeServerRejectsGrantForAnotherScope(t *testing.T) {
	server, authority, local := newConnectorFixture(t)
	scope := execution.Scope{TenantID: "1", AccountID: "2", WorkspaceID: "3", ConnectionID: "4"}
	opened, err := local.Open(context.Background(), execution.OpenRequest{
		Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	foreign, err := authority.Issue(execution.Scope{TenantID: "1", AccountID: "99", WorkspaceID: "3", ConnectionID: "4"}, []string{"query"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"session_handle": opened.Handle, "grant": foreign, "sql": "SELECT 1"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"method": "query", "payload": json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Post(server.URL+"/internal/execution/v2/call", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Error == nil || decoded.Error.Code != string(execution.FailureGrantInvalid) {
		t.Fatalf("response error = %+v, want grant_invalid", decoded.Error)
	}
}

func TestWorkerRuntimeStreamSurfacesRemoteFailure(t *testing.T) {
	server, authority, _ := newConnectorFixture(t)
	worker, err := execution.NewWorkerRuntime(
		execution.NewStaticSessionDirectory(strings.TrimPrefix(server.URL, "http://")),
		authority,
		execution.InsecureTransportCredentials{Client: server.Client()},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	opened, err := worker.Open(ctx, execution.OpenRequest{
		Scope: execution.Scope{TenantID: "1", AccountID: "2", WorkspaceID: "3", ConnectionID: "4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sink bytes.Buffer
	if _, err := worker.Stream(ctx, execution.SessionRequest{Handle: opened.Handle}, &sink, exports.StreamOptions{
		Format: "parquet", SQL: "SELECT 1", MaxBytes: 1 << 20,
	}); !errors.Is(err, exports.ErrUnsupportedFormat) {
		t.Fatalf("Stream() error = %v, want unsupported format", err)
	}

	sink.Reset()
	result, err := worker.Stream(ctx, execution.SessionRequest{Handle: opened.Handle}, &sink, exports.StreamOptions{
		Format: exports.FormatCSV, SQL: "SELECT 1 AS one", MaxBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 1 || result.Bytes == 0 || !strings.Contains(sink.String(), "one") {
		t.Fatalf("Stream() = %+v, body %q", result, sink.String())
	}
}
