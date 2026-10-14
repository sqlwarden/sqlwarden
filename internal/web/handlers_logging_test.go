package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/execution"
)

func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func findLog(records []map[string]any, msg string) map[string]any {
	for _, rec := range records {
		if rec["msg"] == msg {
			return rec
		}
	}
	return nil
}

func TestSchemaSessionFailuresLogWarnings(t *testing.T) {
	var logs bytes.Buffer
	app := newTestAppWith(t, testAppOptions{logger: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-log"), "Schema Log", "Schema Log Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema Log WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, navTestEngine, "Schema Log Conn", "open")
	base := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))

	missing := send(t, newAuthRequest(t, http.MethodPost, base+"/schema/refresh", map[string]any{"path": []any{}}, tok), app.routes())
	assert.Equal(t, missing.StatusCode, http.StatusConflict)

	stale := newAuthRequest(t, http.MethodPost, base+"/schema/refresh", map[string]any{"path": []any{}}, tok)
	stale.Header.Set("X-Warden-Session", "no-such-session")
	assert.Equal(t, send(t, stale, app.routes()).StatusCode, http.StatusGone)

	records := logRecords(t, &logs)
	for _, msg := range []string{"schema session missing", "schema session unavailable"} {
		rec := findLog(records, msg)
		if rec == nil {
			t.Fatalf("no %q record in %s", msg, logs.String())
		}
		assert.Equal(t, rec["level"], "WARN")
		assert.Equal(t, rec["connection_id"], any(float64(conn.ID)))
		if _, ok := rec["request"]; !ok {
			t.Fatalf("%q record has no request correlation: %v", msg, rec)
		}
	}
	if strings.Contains(logs.String(), "no-such-session") {
		t.Fatalf("session id leaked into logs: %s", logs.String())
	}
}

func TestConnectFailureLogsDriverWithoutSecrets(t *testing.T) {
	const password = "connect-log-secret"
	var logs bytes.Buffer
	app, org, ws, tok := setupWorkspaceOwnerWith(t, testAppOptions{
		logger: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		runtime: func(rt execution.Runtime) execution.Runtime {
			return failingOpenRuntime{Runtime: rt, err: &execution.ConnectError{Err: errors.New("authentication failed for " + password), Driver: "postgres"}}
		},
	})
	envID := defaultEnvironmentID(t, app, ws.ID)
	connID := createPostgresConnectionWithPassword(t, app, org.Slug, ws.ID, envID, tok, password)

	res := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/connect", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)

	rec := findLog(logRecords(t, &logs), "database session open failed")
	if rec == nil {
		t.Fatalf("no connect failure record in %s", logs.String())
	}
	assert.Equal(t, rec["level"], "WARN")
	assert.Equal(t, rec["driver"], "postgres")
	if strings.Contains(logs.String(), password) {
		t.Fatalf("secret leaked into logs: %s", logs.String())
	}
}

func TestCompleteConnectionSQLOversizedBodyIsBadRequest(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-oversized"), "Completion", "Completion Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Completion WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Completion DB", "open")

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/completion",
		map[string]any{"sql": strings.Repeat("a", 1<<20+1), "cursor_offset": 0}, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
	if !strings.Contains(fmt.Sprint(res.BodyFields["error"]), "body must not be larger than") {
		t.Fatalf("unexpected body: %s", res.BodyBytes)
	}
}
