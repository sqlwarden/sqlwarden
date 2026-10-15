package web

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type schemaCharacterizationRoute struct {
	name        string
	method      string
	suffix      string
	body        map[string]any
	usesSession bool
	allow       accessExpect
	extra       []schemaCharacterizationCase
	unknown     accessExpect
}

type schemaCharacterizationCase struct {
	name      string
	suffix    string
	body      map[string]any
	sessionID string
	noSession bool
	want      accessExpect
}

func TestSchemaRoutesCharacterization(t *testing.T) {
	tableRef := map[string]any{
		"scope": []map[string]any{{"kind": "database", "name": "main"}},
		"kind":  "table",
		"name":  "characterized",
	}
	viewRef := map[string]any{
		"scope": []map[string]any{{"kind": "database", "name": "main"}},
		"kind":  "view",
		"name":  "characterized",
	}
	missingRef := map[string]any{
		"scope": []map[string]any{{"kind": "database", "name": "main"}},
		"kind":  "table",
		"name":  "no_such_table",
	}
	createTable := map[string]any{
		"operation": "create_table",
		"scope":     []map[string]any{{"kind": "database", "name": "main"}},
		"name":      "events",
		"columns":   []map[string]any{{"name": "id", "data_type": "integer", "primary_key": true}},
	}
	completionBody := func(overrides map[string]any) map[string]any {
		body := map[string]any{"sql": "SELECT * FROM ch", "cursor_offset": 15}
		for k, v := range overrides {
			body[k] = v
		}
		return body
	}

	routes := []schemaCharacterizationRoute{
		{name: "schema tree", method: http.MethodGet, suffix: "/schema/tree", allow: expectStatus(http.StatusOK), unknown: expectStatus(http.StatusOK)},
		{
			name: "schema nodes", method: http.MethodGet, suffix: "/schema/nodes?folder=databases", usesSession: true,
			allow:   expectStatus(http.StatusOK),
			unknown: accessExpect{http.StatusGone, "http_410"},
			extra: []schemaCharacterizationCase{
				{name: "missing folder", suffix: "/schema/nodes", want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
				{name: "malformed path", suffix: "/schema/nodes?folder=databases&path=%7Bnot-json", want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			},
		},
		{
			name: "completion", method: http.MethodPost, suffix: "/completion", body: completionBody(nil),
			allow:   expectStatus(http.StatusOK),
			unknown: expectStatus(http.StatusOK),
			extra: []schemaCharacterizationCase{
				{name: "negative cursor offset", body: completionBody(map[string]any{"cursor_offset": -1}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
				{name: "cursor offset past end", body: completionBody(map[string]any{"cursor_offset": 999}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
				{name: "invalid trigger kind", body: completionBody(map[string]any{"trigger_kind": "bogus"}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
				{name: "oversized sql", body: completionBody(map[string]any{"sql": strings.Repeat("a", 1<<20+1), "cursor_offset": 0}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			},
		},
		{name: "completion index", method: http.MethodGet, suffix: "/schema/completion-index", allow: expectStatus(http.StatusOK), unknown: expectStatus(http.StatusOK)},
		{
			name: "statement generation", method: http.MethodPost, suffix: "/schema/statements", usesSession: true,
			body:    map[string]any{"operation": "select", "ref": tableRef},
			allow:   accessExpect{http.StatusUnprocessableEntity, "statement_generation_failed"},
			unknown: accessExpect{http.StatusGone, "http_410"},
			extra: []schemaCharacterizationCase{
				{name: "unsupported operation for kind", body: map[string]any{"operation": "delete", "ref": viewRef}, want: accessExpect{http.StatusUnprocessableEntity, "statement_generation_failed"}},
				{name: "object missing from the catalog", body: map[string]any{"operation": "select", "ref": missingRef}, want: accessExpect{http.StatusUnprocessableEntity, "statement_generation_failed"}},
				{name: "malformed body", body: map[string]any{"operation": 5}, want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			},
		},
		{
			name: "schema mutation", method: http.MethodPost, suffix: "/schema/mutations", usesSession: true,
			body:    createTable,
			allow:   expectStatus(http.StatusOK),
			unknown: accessExpect{http.StatusGone, "http_410"},
			extra: []schemaCharacterizationCase{
				{name: "missing session header", noSession: true, want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
				{name: "invalid edit", body: map[string]any{"operation": "bogus"}, want: accessExpect{http.StatusUnprocessableEntity, "invalid_schema_edit"}},
				{name: "malformed body", body: map[string]any{"operation": 5}, want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			},
		},
	}

	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			fixture := newQueryCharacterizationFixture(t)
			app := fixture.app
			run := func(token, url, sessionID string, body map[string]any, noSession bool) testResponse {
				req := newAuthRequest(t, route.method, url, body, token)
				if !noSession {
					req.Header.Set("X-Warden-Session", sessionID)
				}
				return send(t, req, app.routes())
			}
			check := func(t *testing.T, res testResponse, want accessExpect) {
				t.Helper()
				if res.StatusCode != want.status {
					t.Fatalf("status = %d, want %d, body %s", res.StatusCode, want.status, res.BodyBytes)
				}
				if got := res.ErrorCode(); got != want.code {
					t.Fatalf("error code = %q, want %q", got, want.code)
				}
			}
			url := fixture.targetURL + route.suffix

			t.Run("unauthenticated", func(t *testing.T) {
				check(t, run("", url, fixture.ownerSessionID, route.body, false), expectInvalidToken)
			})
			t.Run("non-member of org", func(t *testing.T) {
				check(t, run(fixture.nonMemberToken, url, fixture.ownerSessionID, route.body, false), expectNotPermitted)
			})
			t.Run("org member without workspace access", func(t *testing.T) {
				check(t, run(fixture.memberToken, url, fixture.ownerSessionID, route.body, false), expectNotPermitted)
			})
			t.Run("permission on a different connection", func(t *testing.T) {
				check(t, run(fixture.differentConnToken, url, fixture.differentConnSessionID, route.body, false), expectNotPermitted)
			})
			t.Run("connection of another workspace", func(t *testing.T) {
				foreign := fixture.foreignConnectionURL + route.suffix
				check(t, run(fixture.ownerToken, foreign, fixture.ownerSessionID, route.body, false), expectNotFound)
			})
			t.Run("unknown session", func(t *testing.T) {
				res := run(fixture.ownerToken, url, "no-such-session", route.body, false)
				check(t, res, route.unknown)
			})
			for _, tc := range route.extra {
				t.Run(tc.name, func(t *testing.T) {
					path := url
					if tc.suffix != "" {
						path = fixture.targetURL + tc.suffix
					}
					body := route.body
					if tc.body != nil {
						body = tc.body
					}
					session := fixture.ownerSessionID
					if tc.sessionID != "" {
						session = tc.sessionID
					}
					check(t, run(fixture.ownerToken, path, session, body, tc.noSession), tc.want)
				})
			}
			t.Run("allowed", func(t *testing.T) {
				check(t, run(fixture.ownerToken, url, fixture.ownerSessionID, route.body, false), route.allow)
			})
		})
	}
}

func TestSchemaStatementGenerationAllowedCharacterization(t *testing.T) {
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-char-owner"), "Schema Characterization Owner", "Schema Characterization")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema Characterization Workspace", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Characterization Conn", "open")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})
	endpoint := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10)) + "/schema/statements"
	body := map[string]any{"operation": "select", "ref": map[string]any{
		"scope": []map[string]any{{"kind": "database", "name": "main"}},
		"kind":  "table",
		"name":  "widgets",
	}}
	req := newAuthRequest(t, http.MethodPost, endpoint, body, token)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusOK || res.ErrorCode() != "" {
		t.Fatalf("status = %d code = %q, want 200", res.StatusCode, res.ErrorCode())
	}
}
