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
)

var errTestServerFailure = errors.New("test server failure")

func TestRequestLoggingContextGeneratesRequestIDAndSafeAccessLog(t *testing.T) {
	var buf bytes.Buffer
	app := &application{
		logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	router := chi.NewRouter()
	router.Use(app.requestLoggingContext)
	router.Use(app.logAccess)
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/health?token=secret-token", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Header().Get(requestIDHeader) == "" {
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
	if request["id"] == "" {
		t.Fatalf("request.id missing from log: %#v", request)
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
	req.Header.Set(requestIDHeader, "client-request-123")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if got := rr.Header().Get(requestIDHeader); got != "client-request-123" {
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
	req.Header.Set(requestIDHeader, "bad request id\n")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if got := rr.Header().Get(requestIDHeader); got == "" || got == "bad request id\n" {
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
	req.Header.Set(requestIDHeader, "req-error-1")
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
	if request["id"] != "req-error-1" {
		t.Fatalf("request.id = %v, want req-error-1", request["id"])
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
