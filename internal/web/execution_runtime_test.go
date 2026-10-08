package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/execution"
)

func TestExecutionErrorMapping(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	internal := errors.New("credentials: load connection: sql: connection refused at 10.0.0.5")
	failure := func(code execution.FailureCode, err error) error {
		return &execution.Failure{Code: code, Err: err}
	}

	tests := []struct {
		name       string
		err        error
		cancelled  bool
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{name: "session not found", err: failure(execution.FailureSessionNotFound, execution.ErrSessionNotFound), wantStatus: http.StatusGone, wantMsg: "Session has expired or does not exist."},
		{name: "session lost", err: failure(execution.FailureSessionLost, execution.ErrSessionLost), wantStatus: http.StatusGone, wantMsg: "Session has expired or does not exist."},
		{name: "transaction lost", err: failure(execution.FailureTransactionLost, execution.ErrTransactionLost), wantStatus: http.StatusGone, wantMsg: "Session has expired or does not exist."},
		{name: "cursor lost", err: failure(execution.FailureCursorLost, execution.ErrCursorLost), wantStatus: http.StatusGone, wantCode: apiErrorQueryCursorUnavailable},
		{name: "outcome unknown", err: failure(execution.FailureExecutionOutcomeUnknown, execution.ErrOutcomeUnknown), wantStatus: statusClientClosedRequest, wantMsg: "Query was cancelled."},
		{name: "limit exceeded", err: failure(execution.FailureLimitExceeded, execution.ErrLimitExceeded), wantStatus: http.StatusUnprocessableEntity, wantMsg: "The result exceeded the configured size limit."},
		{name: "unknown failure code", err: failure("future_code", internal), wantStatus: http.StatusInternalServerError},
		{name: "context cancelled", err: context.Canceled, wantStatus: statusClientClosedRequest, wantMsg: "Query was cancelled."},
		{name: "request cancelled", err: internal, cancelled: true, wantStatus: statusClientClosedRequest, wantMsg: "Query was cancelled."},
		{name: "target error", err: &execution.TargetError{Err: errors.New(`relation "missing" does not exist`)}, wantStatus: http.StatusUnprocessableEntity, wantMsg: `relation "missing" does not exist`},
		{name: "cursors unsupported", err: execution.ErrCursorsUnsupported, wantStatus: http.StatusUnprocessableEntity, wantMsg: "Connection driver does not support query cursors."},
		{name: "transaction open", err: execution.ErrTransactionOpen, wantStatus: http.StatusConflict},
		{name: "no open transaction", err: execution.ErrNoOpenTransaction, wantStatus: http.StatusConflict},
		{name: "ddl unsupported", err: execution.ErrDDLUnsupported, wantStatus: http.StatusNotImplemented},
		{name: "schema unsupported", err: execution.ErrSchemaUnsupported, wantStatus: http.StatusNotImplemented},
		{name: "relationships unsupported", err: execution.ErrRelationshipsUnsupported, wantStatus: http.StatusNotImplemented},
		{name: "definition unsupported", err: execution.ErrDefinitionUnsupported, wantStatus: http.StatusNotImplemented},
		{name: "unclassified", err: internal, wantStatus: http.StatusInternalServerError},
		{name: "invalid tx mode", err: execution.ErrInvalidTxMode, wantStatus: http.StatusInternalServerError},
		{name: "ddl requires apply", err: execution.ErrDDLRequiresApply, wantStatus: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.cancelled {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/query", nil)
			rec := httptest.NewRecorder()
			app.executionError(rec, req, tc.err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.wantCode != "" && body.Error.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", body.Error.Code, tc.wantCode)
			}
			if tc.wantMsg != "" && body.Error.Message != tc.wantMsg {
				t.Fatalf("message = %q, want %q", body.Error.Message, tc.wantMsg)
			}
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Fatalf("internal error text leaked: %s", rec.Body.String())
			}
		})
	}
}
