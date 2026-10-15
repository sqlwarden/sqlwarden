package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

type queryCharacterizationFixture struct {
	app                    *application
	ownerToken             string
	memberToken            string
	nonMemberToken         string
	differentConnToken     string
	differentAccountID     int64
	org                    database.Organization
	workspace              database.Workspace
	targetURL              string
	foreignConnectionURL   string
	ownerSessionID         string
	differentConnSessionID string
	queryCursorID          string
	historyID              string
	favoriteID             string
}

type queryCharacterizationRoute struct {
	name              string
	method            string
	path              func(queryCharacterizationFixture) string
	body              map[string]any
	usesSession       bool
	wantAllowedStatus int
	wantMemberStatus  int
	wantMemberCode    string
	wantNoWSStatus    int
	wantNoWSCode      string
}

var queryCharacterizationRoutes = []queryCharacterizationRoute{
	{name: "connect", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/connect" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "disconnect session", method: http.MethodDelete, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/session" }, usesSession: true, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusNoContent, wantNoWSStatus: http.StatusBadRequest, wantNoWSCode: apiErrorBadRequest},
	{name: "start query cursor", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/query-cursors" }, body: map[string]any{"sql": "SELECT 1 UNION ALL SELECT 2", "page_size": 1}, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "fetch query cursor", method: http.MethodPost, path: func(f queryCharacterizationFixture) string {
		return f.targetURL + "/query-cursors/" + f.queryCursorID + "/fetch"
	}, body: map[string]any{"page_size": 1}, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "close query cursor", method: http.MethodDelete, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/query-cursors/" + f.queryCursorID }, usesSession: true, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusNoContent},
	{name: "execute query", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/query" }, body: map[string]any{"sql": "SELECT 1", "use_cursor": false}, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "get transaction", method: http.MethodGet, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/transaction" }, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "set transaction mode", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/transaction/mode" }, body: map[string]any{"mode": "manual"}, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "commit transaction", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/transaction/commit" }, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "rollback transaction", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/transaction/rollback" }, usesSession: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "list connection history", method: http.MethodGet, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/history" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "create connection history", method: http.MethodPost, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/history" }, body: map[string]any{"sql_text": "SELECT 1", "status": "ok"}, wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "clear connection history", method: http.MethodDelete, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/history" }, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "delete connection history entry", method: http.MethodDelete, path: func(f queryCharacterizationFixture) string { return f.targetURL + "/history/" + f.historyID }, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "list workspace history", method: http.MethodGet, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/history", f.org.Slug, f.workspace.ID)
	}, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "list query favorites", method: http.MethodGet, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites", f.org.Slug, f.workspace.ID)
	}, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "create query favorite", method: http.MethodPost, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites", f.org.Slug, f.workspace.ID)
	}, body: map[string]any{"name": "Characterized", "sql_text": "SELECT 1"}, wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "update query favorite", method: http.MethodPatch, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites/%s", f.org.Slug, f.workspace.ID, f.favoriteID)
	}, body: map[string]any{"name": "Updated", "sql_text": "SELECT 2"}, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "delete query favorite", method: http.MethodDelete, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites/%s", f.org.Slug, f.workspace.ID, f.favoriteID)
	}, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "list sessions", method: http.MethodGet, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions", f.org.Slug, f.workspace.ID)
	}, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "revoke session", method: http.MethodDelete, path: func(f queryCharacterizationFixture) string {
		return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/sessions/%s", f.org.Slug, f.workspace.ID, f.ownerSessionID)
	}, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
}

func TestQueryRoutesCharacterization(t *testing.T) {
	for _, route := range queryCharacterizationRoutes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			fixture := newQueryCharacterizationFixture(t)
			path := route.path(fixture)
			noWSStatus, noWSCode := route.wantNoWSStatus, route.wantNoWSCode
			if noWSStatus == 0 {
				noWSStatus, noWSCode = route.wantMemberStatus, route.wantMemberCode
			}
			cases := []struct {
				name       string
				token      string
				path       string
				sessionID  string
				wantStatus int
				wantCode   string
			}{
				{name: "unauthenticated", path: path, wantStatus: http.StatusUnauthorized, wantCode: apiErrorInvalidAuthenticationToken},
				{name: "non-member", token: fixture.nonMemberToken, path: path, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "org member without workspace access", token: fixture.memberToken, path: path, wantStatus: noWSStatus, wantCode: noWSCode},
				{name: "permission on different connection", token: fixture.differentConnToken, path: path, sessionID: fixture.differentConnSessionID, wantStatus: route.wantMemberStatus, wantCode: route.wantMemberCode},
			}
			if strings.HasPrefix(path, fixture.targetURL) {
				cases = append(cases, struct {
					name       string
					token      string
					path       string
					sessionID  string
					wantStatus int
					wantCode   string
				}{name: "connection from another workspace", token: fixture.ownerToken, path: replaceQueryConnectionURL(path, fixture.targetURL, fixture.foreignConnectionURL), sessionID: fixture.ownerSessionID, wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound})
			}
			cases = append(cases, struct {
				name       string
				token      string
				path       string
				sessionID  string
				wantStatus int
				wantCode   string
			}{name: "allowed", token: fixture.ownerToken, path: path, sessionID: fixture.ownerSessionID, wantStatus: route.wantAllowedStatus})
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					req := newAuthRequest(t, route.method, tc.path, route.body, tc.token)
					if route.usesSession {
						req.Header.Set("X-Warden-Session", tc.sessionID)
					}
					res := send(t, req, fixture.app.routes())
					if res.StatusCode != tc.wantStatus {
						t.Fatalf("status = %d, want %d", res.StatusCode, tc.wantStatus)
					}
					if got := res.ErrorCode(); got != tc.wantCode {
						t.Fatalf("error code = %q, want %q", got, tc.wantCode)
					}
				})
			}
		})
	}
}

func TestQueryWorkspaceRoutesRejectForeignWorkspace(t *testing.T) {
	fixture := newQueryCharacterizationFixture(t)
	_, _, foreignOrg := seedOrgOwner(t, fixture.app, uniqueEmail(t, "query-char-foreign-owner"), "Query Foreign Owner", "Query Foreign Org")
	foreignOwner, _ := seedAccountWithToken(t, fixture.app, uniqueEmail(t, "query-char-foreign-account"), "Query Foreign Account")
	if err := testDB(fixture.app).AddOrgMember(context.Background(), foreignOrg.ID, foreignOwner.ID); err != nil {
		t.Fatal(err)
	}
	foreignWorkspace := seedWorkspaceForAccount(t, fixture.app, foreignOrg, foreignOwner, "Query Foreign Workspace", "")

	base := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d", fixture.org.Slug, foreignWorkspace.ID)
	routes := []struct {
		name   string
		method string
		path   string
		body   map[string]any
	}{
		{"list workspace history", http.MethodGet, base + "/history", nil},
		{"list query favorites", http.MethodGet, base + "/query-favorites", nil},
		{"create query favorite", http.MethodPost, base + "/query-favorites", map[string]any{"name": "Foreign", "sql_text": "SELECT 1"}},
		{"update query favorite", http.MethodPatch, base + "/query-favorites/" + fixture.favoriteID, map[string]any{"name": "Foreign", "sql_text": "SELECT 1"}},
		{"delete query favorite", http.MethodDelete, base + "/query-favorites/" + fixture.favoriteID, nil},
		{"list sessions", http.MethodGet, base + "/sessions", nil},
		{"revoke session", http.MethodDelete, base + "/sessions/" + fixture.ownerSessionID, nil},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			res := send(t, newAuthRequest(t, route.method, route.path, route.body, fixture.ownerToken), fixture.app.routes())
			if res.StatusCode != http.StatusNotFound || res.ErrorCode() != apiErrorNotFound {
				t.Fatalf("status = %d, code = %q, want 404 %q", res.StatusCode, res.ErrorCode(), apiErrorNotFound)
			}
		})
	}
}

// A caller permitted on the connection who presents another account's valid
// session or cursor must not be able to use it or tell it apart from an
// unknown one, and must leave it intact.
func TestQueryRuntimeRoutesHideForeignSessions(t *testing.T) {
	fixture := newQueryCharacterizationFixture(t)
	app := fixture.app
	targetID, err := strconv.ParseInt(fixture.targetURL[strings.LastIndex(fixture.targetURL, "/")+1:], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB(app).AddWorkspaceMember(context.Background(), fixture.workspace.ID, fixture.differentAccountID, nil); err != nil {
		t.Fatal(err)
	}
	roleID := createRoleForTest(t, app, fixture.org.ID, nil, "connection", access.PermConnRead, access.PermConnExecute, access.PermConnDQL, access.PermConnDML, access.PermConnDDL)
	grant := grantWorkspacePolicyRole(t, app, fixture.ownerToken, fixture.org.Slug, strconv.FormatInt(fixture.workspace.ID, 10), roleID, access.SubjectTypeAccount, fixture.differentAccountID, "connection", targetID)
	if grant.StatusCode != http.StatusNoContent {
		t.Fatalf("grant status = %d, want %d", grant.StatusCode, http.StatusNoContent)
	}

	routes := []struct {
		name       string
		method     string
		path       string
		body       map[string]any
		wantStatus int
	}{
		{"execute query", http.MethodPost, fixture.targetURL + "/query", map[string]any{"sql": "SELECT 1", "use_cursor": false}, http.StatusGone},
		{"start query cursor", http.MethodPost, fixture.targetURL + "/query-cursors", map[string]any{"sql": "SELECT 1 UNION ALL SELECT 2", "page_size": 1}, http.StatusGone},
		{"fetch query cursor", http.MethodPost, fixture.targetURL + "/query-cursors/" + fixture.queryCursorID + "/fetch", map[string]any{"page_size": 1}, http.StatusGone},
		{"close query cursor", http.MethodDelete, fixture.targetURL + "/query-cursors/" + fixture.queryCursorID, nil, http.StatusNoContent},
		{"get transaction", http.MethodGet, fixture.targetURL + "/transaction", nil, http.StatusGone},
		{"set transaction mode", http.MethodPost, fixture.targetURL + "/transaction/mode", map[string]any{"mode": "auto"}, http.StatusGone},
		{"commit transaction", http.MethodPost, fixture.targetURL + "/transaction/commit", nil, http.StatusGone},
		{"rollback transaction", http.MethodPost, fixture.targetURL + "/transaction/rollback", nil, http.StatusGone},
		{"disconnect session", http.MethodDelete, fixture.targetURL + "/session", nil, http.StatusNoContent},
	}
	for _, route := range routes {
		req := newAuthRequest(t, route.method, route.path, route.body, fixture.differentConnToken)
		req.Header.Set("X-Warden-Session", fixture.ownerSessionID)
		if res := send(t, req, app.routes()); res.StatusCode != route.wantStatus {
			t.Fatalf("%s: status = %d, want %d", route.name, res.StatusCode, route.wantStatus)
		}
	}

	// The mode is validated after the session is resolved, so an invalid mode
	// is a 410 on a foreign or unknown session and a 422 on the caller's own.
	for _, sessionID := range []string{fixture.ownerSessionID, "no-such-session"} {
		req := newAuthRequest(t, http.MethodPost, fixture.targetURL+"/transaction/mode", map[string]any{"mode": 5}, fixture.differentConnToken)
		req.Header.Set("X-Warden-Session", sessionID)
		if res := send(t, req, app.routes()); res.StatusCode != http.StatusGone {
			t.Fatalf("bad transaction mode on session %q: status = %d, want 410", sessionID, res.StatusCode)
		}
	}
	ownReq := newAuthRequest(t, http.MethodPost, fixture.targetURL+"/transaction/mode", map[string]any{"mode": 5}, fixture.ownerToken)
	ownReq.Header.Set("X-Warden-Session", fixture.ownerSessionID)
	if res := send(t, ownReq, app.routes()); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad transaction mode on own session: status = %d, want 422", res.StatusCode)
	}

	statusReq := newAuthRequest(t, http.MethodGet, fixture.targetURL+"/transaction", nil, fixture.ownerToken)
	statusReq.Header.Set("X-Warden-Session", fixture.ownerSessionID)
	statusRes := send(t, statusReq, app.routes())
	if statusRes.StatusCode != http.StatusOK || statusRes.BodyFields["open"] != true {
		t.Fatalf("owner transaction status = %d %v, want 200 with the transaction still open", statusRes.StatusCode, statusRes.BodyFields)
	}
	fetchReq := newAuthRequest(t, http.MethodPost, fixture.targetURL+"/query-cursors/"+fixture.queryCursorID+"/fetch", map[string]any{"page_size": 1}, fixture.ownerToken)
	fetchReq.Header.Set("X-Warden-Session", fixture.ownerSessionID)
	if res := send(t, fetchReq, app.routes()); res.StatusCode != http.StatusOK {
		t.Fatalf("owner cursor fetch status = %d, want 200", res.StatusCode)
	}
}

func replaceQueryConnectionURL(path, targetURL, foreignURL string) string {
	if len(path) >= len(targetURL) && path[:len(targetURL)] == targetURL {
		return foreignURL + path[len(targetURL):]
	}
	return path
}

func newQueryCharacterizationFixture(t *testing.T) queryCharacterizationFixture {
	t.Helper()
	app := newTestApp(t)
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "backend"
		settings.QueryFavoritesMode = "backend"
	})
	owner, ownerToken, org := seedOrgOwner(t, app, uniqueEmail(t, "query-char-owner"), "Query Characterization Owner", "Query Characterization")
	member, memberToken := seedAccountWithToken(t, app, uniqueEmail(t, "query-char-member"), "Query Characterization Member")
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	different, differentToken := seedAccountWithToken(t, app, uniqueEmail(t, "query-char-different"), "Query Characterization Different")
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, different.ID); err != nil {
		t.Fatal(err)
	}
	_, nonMemberToken := seedAccountWithToken(t, app, uniqueEmail(t, "query-char-non-member"), "Query Characterization Non-member")

	workspace := seedWorkspaceForAccount(t, app, org, owner, "Query Characterization Workspace", "")
	environmentID := defaultEnvironmentID(t, app, workspace.ID)
	target := insertStructuredConnection(t, app, workspace.ID, &environmentID, "Query Characterization Target", "sqlite", map[string]any{"path": ":memory:"})
	differentConn := insertStructuredConnection(t, app, workspace.ID, &environmentID, "Query Characterization Different", "sqlite", map[string]any{"path": ":memory:"})
	foreignWorkspace := seedWorkspaceForAccount(t, app, org, owner, "Query Characterization Foreign Workspace", "")
	foreignEnvironmentID := defaultEnvironmentID(t, app, foreignWorkspace.ID)
	foreignConn := insertStructuredConnection(t, app, foreignWorkspace.ID, &foreignEnvironmentID, "Query Characterization Foreign", "sqlite", map[string]any{"path": ":memory:"})

	roleID := createRoleForTest(t, app, org.ID, nil, "connection", access.PermConnRead, access.PermConnExecute, access.PermConnDQL, access.PermConnDML, access.PermConnDDL)
	grant := grantWorkspacePolicyRole(t, app, ownerToken, org.Slug, strconv.FormatInt(workspace.ID, 10), roleID, access.SubjectTypeAccount, different.ID, "connection", differentConn.ID)
	if grant.StatusCode != http.StatusNoContent {
		t.Fatalf("grant status = %d, want %d", grant.StatusCode, http.StatusNoContent)
	}

	targetURL := orgConnectionURL(org.Slug, workspace.ID, environmentID, strconv.FormatInt(target.ID, 10))
	differentURL := orgConnectionURL(org.Slug, workspace.ID, environmentID, strconv.FormatInt(differentConn.ID, 10))
	ownerSessionID := connectCharacterizationSession(t, app, targetURL, ownerToken)
	differentSessionID := connectCharacterizationSession(t, app, differentURL, differentToken)

	modeReq := newAuthRequest(t, http.MethodPost, targetURL+"/transaction/mode", map[string]any{"mode": "manual"}, ownerToken)
	modeReq.Header.Set("X-Warden-Session", ownerSessionID)
	if res := send(t, modeReq, app.routes()); res.StatusCode != http.StatusOK {
		t.Fatalf("set transaction mode status = %d", res.StatusCode)
	}
	queryReq := newAuthRequest(t, http.MethodPost, targetURL+"/query", map[string]any{"sql": "CREATE TABLE characterized (id INTEGER)"}, ownerToken)
	queryReq.Header.Set("X-Warden-Session", ownerSessionID)
	if res := send(t, queryReq, app.routes()); res.StatusCode != http.StatusOK {
		t.Fatalf("open transaction status = %d", res.StatusCode)
	}
	cursorReq := newAuthRequest(t, http.MethodPost, targetURL+"/query-cursors", map[string]any{"sql": "SELECT 1 UNION ALL SELECT 2", "page_size": 1}, ownerToken)
	cursorReq.Header.Set("X-Warden-Session", ownerSessionID)
	cursorRes := send(t, cursorReq, app.routes())
	if cursorRes.StatusCode != http.StatusOK {
		t.Fatalf("create cursor status = %d", cursorRes.StatusCode)
	}

	historyRes := send(t, newAuthRequest(t, http.MethodPost, targetURL+"/history", map[string]any{"sql_text": "SELECT 1", "status": "ok"}, ownerToken), app.routes())
	if historyRes.StatusCode != http.StatusCreated {
		t.Fatalf("create history status = %d", historyRes.StatusCode)
	}
	favoritesURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/query-favorites", org.Slug, workspace.ID)
	favoriteRes := send(t, newAuthRequest(t, http.MethodPost, favoritesURL, map[string]any{"name": "Original", "sql_text": "SELECT 1"}, ownerToken), app.routes())
	if favoriteRes.StatusCode != http.StatusCreated {
		t.Fatalf("create favorite status = %d", favoriteRes.StatusCode)
	}

	return queryCharacterizationFixture{
		app: app, ownerToken: ownerToken, memberToken: memberToken, nonMemberToken: nonMemberToken, differentConnToken: differentToken,
		differentAccountID: different.ID,
		org:                org, workspace: workspace, targetURL: targetURL,
		foreignConnectionURL: orgConnectionURL(org.Slug, workspace.ID, environmentID, strconv.FormatInt(foreignConn.ID, 10)),
		ownerSessionID:       ownerSessionID, differentConnSessionID: differentSessionID,
		queryCursorID: fmt.Sprintf("%v", cursorRes.BodyFields["query_cursor_id"]),
		historyID:     fmt.Sprintf("%v", historyRes.BodyFields["id"]), favoriteID: fmt.Sprintf("%v", favoriteRes.BodyFields["id"]),
	}
}

func connectCharacterizationSession(t *testing.T, app *application, url, token string) string {
	t.Helper()
	res := send(t, newAuthRequest(t, http.MethodPost, url+"/connect", nil, token), app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("connect status = %d", res.StatusCode)
	}
	return res.BodyFields["session_id"].(string)
}
