package web

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
)

const authCharacterizationPassword = "securepass99"

type authCharacterizationFixture struct {
	app          *application
	email        string
	token        string
	cookie       *http.Cookie
	otherEmail   string
	otherToken   string
	otherSession string
	sessionID    string
}

func newAuthCharacterizationFixture(t *testing.T) *authCharacterizationFixture {
	t.Helper()
	app := newTestApp(t)
	setupInstance(t, app, uniqueEmail(t, "auth-char-admin"), "Auth Characterization Admin", authCharacterizationPassword)

	f := &authCharacterizationFixture{app: app, email: uniqueEmail(t, "auth-char-user"), otherEmail: uniqueEmail(t, "auth-char-other")}
	if res := registerTestUser(t, app, f.email, "Auth Characterization User", authCharacterizationPassword); res.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, body %s", res.StatusCode, res.BodyBytes)
	}
	if res := registerTestUser(t, app, f.otherEmail, "Auth Characterization Other", authCharacterizationPassword); res.StatusCode != http.StatusCreated {
		t.Fatalf("register other status = %d, body %s", res.StatusCode, res.BodyBytes)
	}
	login := loginTestUser(t, app, f.email, authCharacterizationPassword)
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, body %s", login.StatusCode, login.BodyBytes)
	}
	f.token = extractAccessToken(t, login)
	f.cookie = extractRefreshCookie(t, login)
	otherLogin := loginTestUser(t, app, f.otherEmail, authCharacterizationPassword)
	f.otherToken = extractAccessToken(t, otherLogin)

	f.sessionID = f.firstSessionID(t, f.token)
	f.otherSession = f.firstSessionID(t, f.otherToken)
	return f
}

func (f *authCharacterizationFixture) firstSessionID(t *testing.T, token string) string {
	t.Helper()
	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/account/sessions", nil, token), f.app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list sessions status = %d, body %s", res.StatusCode, res.BodyBytes)
	}
	items, _ := res.BodyFields["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("no sessions listed: %s", res.BodyBytes)
	}
	first, _ := items[0].(map[string]any)
	return fmt.Sprint(first["id"])
}

type authCase struct {
	name   string
	method string
	path   string
	body   func(*authCharacterizationFixture) map[string]any
	token  func(*authCharacterizationFixture) string
	cookie func(*testing.T, *authCharacterizationFixture) *http.Cookie
	path2  func(*authCharacterizationFixture) string
	prep   func(*testing.T, *authCharacterizationFixture)
	want   accessExpect
}

func runAuthCharacterization(t *testing.T, cases []authCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthCharacterizationFixture(t)
			if tc.prep != nil {
				tc.prep(t, f)
			}
			path := tc.path
			if tc.path2 != nil {
				path = tc.path2(f)
			}
			var payload map[string]any
			if tc.body != nil {
				payload = tc.body(f)
			}
			var req *http.Request
			if tc.token == nil {
				req = newTestRequest(t, tc.method, path, payload)
			} else {
				req = newAuthRequest(t, tc.method, path, payload, tc.token(f))
			}
			if tc.cookie != nil {
				if c := tc.cookie(t, f); c != nil {
					req.AddCookie(c)
				}
			}
			res := send(t, req, f.app.routes())
			if res.StatusCode != tc.want.status {
				t.Fatalf("status = %d, want %d, body %s", res.StatusCode, tc.want.status, res.BodyBytes)
			}
			if got := res.ErrorCode(); got != tc.want.code {
				t.Fatalf("error code = %q, want %q", got, tc.want.code)
			}
		})
	}
}

func userToken(f *authCharacterizationFixture) string  { return f.token }
func emptyToken(*authCharacterizationFixture) string   { return "" }
func garbageToken(*authCharacterizationFixture) string { return "not-a-token" }

func userBody(body map[string]any) func(*authCharacterizationFixture) map[string]any {
	return func(*authCharacterizationFixture) map[string]any { return body }
}

func TestAuthRegisterAndLoginCharacterization(t *testing.T) {
	const register, login = "/api/v1/auth/register", "/api/v1/auth/login"
	runAuthCharacterization(t, []authCase{
		{name: "register missing fields", method: http.MethodPost, path: register, body: userBody(map[string]any{}), want: expectValidation},
		{name: "register short password", method: http.MethodPost, path: register, body: userBody(map[string]any{"email": "short-pw@example.com", "name": "Short", "password": "short"}), want: expectValidation},
		{name: "register invalid email", method: http.MethodPost, path: register, body: userBody(map[string]any{"email": "not-an-email", "name": "Invalid", "password": authCharacterizationPassword}), want: expectValidation},
		{name: "register duplicate email", method: http.MethodPost, path: register, body: func(f *authCharacterizationFixture) map[string]any {
			return map[string]any{"email": f.email, "name": "Duplicate", "password": authCharacterizationPassword}
		}, want: expectValidation},
		{name: "register malformed body", method: http.MethodPost, path: register, body: userBody(map[string]any{"email": 5}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
		{name: "register succeeds", method: http.MethodPost, path: register, body: userBody(map[string]any{"email": "fresh-auth-char@example.com", "name": "Fresh", "password": authCharacterizationPassword}), want: expectStatus(http.StatusCreated)},
		{name: "login missing fields", method: http.MethodPost, path: login, body: userBody(map[string]any{}), want: expectValidation},
		{name: "login wrong password", method: http.MethodPost, path: login, body: func(f *authCharacterizationFixture) map[string]any {
			return map[string]any{"email": f.email, "password": "wrong-password-1"}
		}, want: expectInvalidToken},
		{name: "login unknown email", method: http.MethodPost, path: login, body: userBody(map[string]any{"email": "nobody-auth-char@example.com", "password": authCharacterizationPassword}), want: expectInvalidToken},
		{name: "login deactivated account", method: http.MethodPost, path: login, body: func(f *authCharacterizationFixture) map[string]any {
			return map[string]any{"email": f.email, "password": authCharacterizationPassword}
		}, prep: func(t *testing.T, f *authCharacterizationFixture) {
			account, found, err := testDB(f.app).GetAccountByEmail(context.Background(), f.email)
			if err != nil || !found {
				t.Fatalf("lookup account: found=%v err=%v", found, err)
			}
			if err := testDB(f.app).DeactivateAccount(context.Background(), account.ID); err != nil {
				t.Fatal(err)
			}
		}, want: expectInvalidToken},
		{name: "login succeeds", method: http.MethodPost, path: login, body: func(f *authCharacterizationFixture) map[string]any {
			return map[string]any{"email": f.email, "password": authCharacterizationPassword}
		}, want: expectStatus(http.StatusOK)},
	})
}

func TestAuthRegisterBeforeSetupCharacterization(t *testing.T) {
	app := newTestApp(t)
	res := registerTestUser(t, app, uniqueEmail(t, "auth-char-early"), "Early", authCharacterizationPassword)
	if res.StatusCode != http.StatusForbidden || res.ErrorCode() != apiErrorNotPermitted {
		t.Fatalf("register before setup = %d %q, want 403 %q", res.StatusCode, res.ErrorCode(), apiErrorNotPermitted)
	}
}

func TestAuthRefreshAndLogoutCharacterization(t *testing.T) {
	const refresh, logout = "/api/v1/auth/refresh", "/api/v1/auth/logout"
	withCookie := func(_ *testing.T, f *authCharacterizationFixture) *http.Cookie { return f.cookie }
	runAuthCharacterization(t, []authCase{
		{name: "refresh without cookie", method: http.MethodPost, path: refresh, want: expectInvalidToken},
		{name: "refresh with unknown token", method: http.MethodPost, path: refresh, cookie: func(*testing.T, *authCharacterizationFixture) *http.Cookie {
			return &http.Cookie{Name: "refresh_token", Value: "unknown-refresh-token"}
		}, want: expectInvalidToken},
		{name: "refresh with expired token", method: http.MethodPost, path: refresh, cookie: withCookie, prep: func(t *testing.T, f *authCharacterizationFixture) {
			if _, err := testDB(f.app).NewUpdate().Model((*database.RefreshToken)(nil)).
				Set("expires_at = ?", time.Now().Add(-time.Hour)).Where("1 = 1").Exec(context.Background()); err != nil {
				t.Fatal(err)
			}
		}, want: expectInvalidToken},
		{name: "refresh after logout", method: http.MethodPost, path: refresh, cookie: withCookie, prep: func(t *testing.T, f *authCharacterizationFixture) {
			req := newTestRequest(t, http.MethodPost, logout, nil)
			req.AddCookie(f.cookie)
			if res := send(t, req, f.app.routes()); res.StatusCode != http.StatusNoContent {
				t.Fatalf("logout status = %d", res.StatusCode)
			}
		}, want: expectInvalidToken},
		{name: "refresh after session revocation", method: http.MethodPost, path: refresh, cookie: withCookie, prep: func(t *testing.T, f *authCharacterizationFixture) {
			res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/account/sessions", nil, f.token), f.app.routes())
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("revoke sessions status = %d", res.StatusCode)
			}
		}, want: expectInvalidToken},
		{name: "refresh succeeds", method: http.MethodPost, path: refresh, cookie: withCookie, want: expectStatus(http.StatusOK)},
		{name: "logout without cookie", method: http.MethodPost, path: logout, want: expectStatus(http.StatusNoContent)},
		{name: "logout with unknown token", method: http.MethodPost, path: logout, cookie: func(*testing.T, *authCharacterizationFixture) *http.Cookie {
			return &http.Cookie{Name: "refresh_token", Value: "unknown-refresh-token"}
		}, want: expectStatus(http.StatusNoContent)},
		{name: "logout succeeds", method: http.MethodPost, path: logout, cookie: withCookie, want: expectStatus(http.StatusNoContent)},
	})
}

func TestAuthAccountRoutesCharacterization(t *testing.T) {
	anonymous := accessExpect{http.StatusUnauthorized, apiErrorAuthenticationRequired}
	type route struct {
		name    string
		method  string
		path    string
		body    map[string]any
		allowed accessExpect
	}
	routes := []route{
		{"get account", http.MethodGet, "/api/v1/account", nil, expectStatus(http.StatusOK)},
		{"update account", http.MethodPatch, "/api/v1/account", map[string]any{"name": "Renamed User"}, expectStatus(http.StatusOK)},
		{"update password", http.MethodPatch, "/api/v1/account/password", map[string]any{"current_password": authCharacterizationPassword, "new_password": "another-secure-pass1"}, expectStatus(http.StatusNoContent)},
		{"list account organizations", http.MethodGet, "/api/v1/account/orgs", nil, expectStatus(http.StatusOK)},
		{"list account sessions", http.MethodGet, "/api/v1/account/sessions", nil, expectStatus(http.StatusOK)},
		{"revoke all account sessions", http.MethodDelete, "/api/v1/account/sessions", nil, expectStatus(http.StatusNoContent)},
		{"get session", http.MethodGet, "/api/v1/session", nil, expectStatus(http.StatusOK)},
	}
	var cases []authCase
	for _, r := range routes {
		cases = append(cases,
			authCase{name: r.name + " without authorization", method: r.method, path: r.path, body: userBody(r.body), want: anonymous},
			authCase{name: r.name + " with empty token", method: r.method, path: r.path, body: userBody(r.body), token: emptyToken, want: expectInvalidToken},
			authCase{name: r.name + " with garbage token", method: r.method, path: r.path, body: userBody(r.body), token: garbageToken, want: expectInvalidToken},
			authCase{name: r.name + " allowed", method: r.method, path: r.path, body: userBody(r.body), token: userToken, want: r.allowed},
		)
	}
	cases = append(cases,
		authCase{name: "update account empty name", method: http.MethodPatch, path: "/api/v1/account", body: userBody(map[string]any{"name": ""}), token: userToken, want: expectValidation},
		authCase{name: "update password missing fields", method: http.MethodPatch, path: "/api/v1/account/password", body: userBody(map[string]any{}), token: userToken, want: expectValidation},
		authCase{name: "update password wrong current password", method: http.MethodPatch, path: "/api/v1/account/password",
			body: userBody(map[string]any{"current_password": "wrong-password-1", "new_password": "another-secure-pass1"}), token: userToken, want: expectValidation},
		authCase{name: "update password too short", method: http.MethodPatch, path: "/api/v1/account/password",
			body: userBody(map[string]any{"current_password": authCharacterizationPassword, "new_password": "short"}), token: userToken, want: expectValidation},
		authCase{name: "token of a revoked session", method: http.MethodGet, path: "/api/v1/account", token: userToken, prep: func(t *testing.T, f *authCharacterizationFixture) {
			res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/account/sessions/"+f.sessionID, nil, f.token), f.app.routes())
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("revoke session status = %d", res.StatusCode)
			}
		}, want: expectInvalidToken},
	)
	runAuthCharacterization(t, cases)
}

func TestAuthOwnSessionRevokeCharacterization(t *testing.T) {
	sessionPath := func(f *authCharacterizationFixture) string { return "/api/v1/account/sessions/" + f.sessionID }
	runAuthCharacterization(t, []authCase{
		{name: "revoke without authorization", method: http.MethodDelete, path2: sessionPath, want: accessExpect{http.StatusUnauthorized, apiErrorAuthenticationRequired}},
		{name: "revoke with empty token", method: http.MethodDelete, path2: sessionPath, token: emptyToken, want: expectInvalidToken},
		{name: "revoke unknown session", method: http.MethodDelete, path: "/api/v1/account/sessions/does-not-exist", token: userToken, want: expectNotFound},
		{name: "revoke another account's session", method: http.MethodDelete, path2: func(f *authCharacterizationFixture) string {
			return "/api/v1/account/sessions/" + f.otherSession
		}, token: userToken, want: expectNotFound},
		{name: "revoke own session", method: http.MethodDelete, path2: sessionPath, token: userToken, want: expectStatus(http.StatusNoContent)},
	})
}
