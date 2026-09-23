package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/observability"
)

var errTestServerFailure = errors.New("test server failure")

func TestAccessLogLevelLeavesErrorOwnershipToFailureLogger(t *testing.T) {
	if got := accessLogLevel(http.StatusInternalServerError); got != slog.LevelWarn {
		t.Fatalf("accessLogLevel(500) = %v, want WARN", got)
	}
	if got := accessLogLevel(http.StatusBadRequest); got != slog.LevelWarn {
		t.Fatalf("accessLogLevel(400) = %v, want WARN", got)
	}
	if got := accessLogLevel(http.StatusOK); got != slog.LevelInfo {
		t.Fatalf("accessLogLevel(200) = %v, want INFO", got)
	}
}

func TestExecutionErrorCategoryDoesNotExposeRawErrors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.Join(execution.ErrSessionLost, errors.New("driver-secret")), "session_lost"},
		{errors.Join(execution.ErrTargetConnection, errors.New("postgres://user:secret@host/db")), "target_connection"},
		{errors.New("SELECT secret FROM private_table"), "target_error"},
	}
	for _, tt := range tests {
		if got := executionErrorCategory(tt.err); got != tt.want {
			t.Fatalf("executionErrorCategory(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestLoggerLevelChangesAtRuntime(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(config.Default(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden")
	if buf.Len() != 0 {
		t.Fatalf("debug log emitted at info level: %s", buf.String())
	}
	if err := setLoggerLevel(logger, config.LogLevelDebug); err != nil {
		t.Fatal(err)
	}
	logger.Debug("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("debug log missing after live level change: %s", buf.String())
	}
	if err := setLoggerLevel(logger, config.LogLevelError); err != nil {
		t.Fatal(err)
	}
	before := buf.Len()
	logger.Info("hidden again")
	if buf.Len() != before {
		t.Fatalf("info log emitted at error level: %s", buf.String())
	}
}

func TestRequestLoggingContextGeneratesRequestIDAndSafeAccessLog(t *testing.T) {
	var buf bytes.Buffer
	app := &application{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	app.accessLogsEnabled.Store(true)

	router := chi.NewRouter()
	router.Use(app.requestLoggingContext)
	router.Use(app.logAccess)
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/health?token=secret-token", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Header().Get(observability.RequestIDHeader) == "" {
		t.Fatal("expected generated request ID response header")
	}

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected access log")
	}
	if strings.Contains(line, "secret-token") {
		t.Fatalf("access log leaked raw query string: %s", line)
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatal(err)
	}

	request, ok := entry["request"].(map[string]any)
	if !ok {
		t.Fatalf("request group missing from log: %#v", entry)
	}
	if entry["request_id"] == "" {
		t.Fatalf("request_id missing from log: %#v", entry)
	}
	if request["path"] != "/health" {
		t.Fatalf("request.path = %v, want /health", request["path"])
	}
	if request["route"] != "/health" {
		t.Fatalf("request.route = %v, want /health", request["route"])
	}
	if _, ok := request["url"]; ok {
		t.Fatalf("request.url should not be logged: %#v", request)
	}
}

func TestAccessLogsCanBeToggledLive(t *testing.T) {
	var buf bytes.Buffer
	app := &application{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
	router := chi.NewRouter()
	router.Use(app.requestLoggingContext)
	router.Use(app.logAccess)
	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if buf.Len() != 0 {
		t.Fatalf("access log emitted while disabled by default: %s", buf.String())
	}

	app.accessLogsEnabled.Store(true)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if !strings.Contains(buf.String(), "http request") {
		t.Fatalf("access log missing after live enable: %s", buf.String())
	}

	app.accessLogsEnabled.Store(false)
	buf.Reset()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if buf.Len() != 0 {
		t.Fatalf("access log emitted after live disable: %s", buf.String())
	}
}

func TestRequestLoggingContextPreservesValidIncomingRequestID(t *testing.T) {
	var buf bytes.Buffer
	app := &application{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	router := chi.NewRouter()
	router.Use(app.requestLoggingContext)
	router.Use(app.logAccess)
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(observability.RequestIDHeader, "client-request-123")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if got := rr.Header().Get(observability.RequestIDHeader); got != "client-request-123" {
		t.Fatalf("response request ID = %q, want client-request-123", got)
	}
}

func TestRequestLoggingContextReplacesInvalidIncomingRequestID(t *testing.T) {
	var buf bytes.Buffer
	app := &application{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	router := chi.NewRouter()
	router.Use(app.requestLoggingContext)
	router.Use(app.logAccess)
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(observability.RequestIDHeader, "bad request id\n")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if got := rr.Header().Get(observability.RequestIDHeader); got == "" || got == "bad request id\n" {
		t.Fatalf("invalid request ID was not replaced, got %q", got)
	}
}

func TestReportServerErrorLogsRequestIDAndSafeRequestPath(t *testing.T) {
	var buf bytes.Buffer
	app := &application{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	router := chi.NewRouter()
	router.Use(app.requestLoggingContext)
	router.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		app.serverError(w, r, errTestServerFailure)
	})

	req := httptest.NewRequest(http.MethodGet, "/boom?token=secret-token", nil)
	req.Header.Set(observability.RequestIDHeader, "req-error-1")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	line := strings.Split(strings.TrimSpace(buf.String()), "\n")[0]
	if strings.Contains(line, "secret-token") {
		t.Fatalf("server error log leaked raw query string: %s", line)
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatal(err)
	}
	request, ok := entry["request"].(map[string]any)
	if !ok {
		t.Fatalf("request group missing from log: %#v", entry)
	}
	if entry["request_id"] != "req-error-1" {
		t.Fatalf("request_id = %v, want req-error-1", entry["request_id"])
	}
	if request["path"] != "/boom" {
		t.Fatalf("request.path = %v, want /boom", request["path"])
	}
	if _, ok := request["url"]; ok {
		t.Fatalf("request.url should not be logged: %#v", request)
	}
	if entry["trace"] == "" {
		t.Fatal("expected trace in server error log")
	}
}

func TestRequestPathRedactsInvitationTokens(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		path string
		want string
	}{
		{"/api/v1/invitations/super-secret", "/api/v1/invitations/[redacted]"},
		{"/api/v1/invitations/super-secret/accept", "/api/v1/invitations/[redacted]/accept"},
		{"/api/v1/orgs/acme/invitations", "/api/v1/orgs/acme/invitations"},
	} {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		if got := requestPath(req); got != tt.want {
			t.Errorf("requestPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
