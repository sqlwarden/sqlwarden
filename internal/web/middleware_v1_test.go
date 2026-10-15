package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/platform/clientip"
	"github.com/sqlwarden/internal/token"
)

// testWithParams injects chi URL params into the request context.
func testWithParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// issueTestToken creates a valid JWT for the given account using the test app's secret.
func issueTestToken(t *testing.T, app *application, accountID int64, email, name string) string {
	t.Helper()
	authSession, err := testDB(app).InsertAuthSession(context.Background(), accountID, time.Now().Add(7*24*time.Hour), "test-agent", "127.0.0.1", database.SessionAuth{Method: "password", Assurance: "aal1"})
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := token.IssueWithSessionTTL(strconv.FormatInt(accountID, 10), authSession.ID, email, name, app.config.JWT.SecretKey, time.Duration(database.DefaultJWTAccessTokenTTLSeconds)*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAuthenticateV1_ValidToken(t *testing.T) {
	app := newTestApplication(t)

	account, err := testDB(app).InsertAccount(context.Background(), "auth-test@example.com", "Auth Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)

	var gotID int64
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acc := contextGetAccount(r)
		gotID = acc.ID
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	app.authenticateV1(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotID != account.ID {
		t.Fatalf("expected account ID %d, got %d", account.ID, gotID)
	}
}

func TestAuthenticateV1_InvalidToken(t *testing.T) {
	app := newTestApplication(t)

	var gotID int64
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acc := contextGetAccount(r)
		gotID = acc.ID
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")

	rec := httptest.NewRecorder()
	app.authenticateV1(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if gotID != 0 {
		t.Fatal("expected no account in context for invalid token")
	}
}

func TestAuthenticateV1_RejectsTokenWithoutAuthSession(t *testing.T) {
	app := newTestApplication(t)

	account, err := testDB(app).InsertAccount(context.Background(), "missing-session@example.com", "Missing Session", nil)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := token.Issue(strconv.FormatInt(account.ID, 10), account.Email, account.Name, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	app.authenticateV1(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticateV1_AllowsTokenWithoutAuthSessionWhenRevocationDisabled(t *testing.T) {
	app := newTestApplication(t)
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.SessionsRevocationEnabled = false
	})

	account, err := testDB(app).InsertAccount(context.Background(), "revocation-disabled@example.com", "Revocation Disabled", nil)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := token.Issue(strconv.FormatInt(account.ID, 10), account.Email, account.Name, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}

	var gotID int64
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	app.authenticateV1(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = contextGetAccount(r).ID
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotID != account.ID {
		t.Fatalf("expected account ID %d, got %d", account.ID, gotID)
	}
}

func TestAuthenticateV1_RejectsRevokedAuthSession(t *testing.T) {
	app := newTestApplication(t)

	account, err := testDB(app).InsertAccount(context.Background(), "revoked-session@example.com", "Revoked Session", nil)
	if err != nil {
		t.Fatal(err)
	}
	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)
	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = testDB(app).RevokeAuthSession(context.Background(), claims.AuthSessionID, &account.ID, "test"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	app.authenticateV1(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAccount_NoAccount(t *testing.T) {
	app := newTestApplication(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app.requireAccount(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// serveOrgScope authenticates a bearer token for the account and runs
// orgScope for slug, returning the response and the path ids the next
// handler saw.
func serveOrgScope(t *testing.T, app *application, account database.Account, slug string) (*httptest.ResponseRecorder, pathIDs) {
	t.Helper()
	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)
	var got pathIDs
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = contextGetPathIDs(r)
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+slug, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req = testWithParams(req, map[string]string{"org_slug": slug})
	rec := httptest.NewRecorder()
	app.authenticateV1(app.requireAccount(app.orgScope(final))).ServeHTTP(rec, req)
	return rec, got
}

func countOrgAccessSessions(t *testing.T, app *application, orgID, accountID int64) int {
	t.Helper()
	var count int
	err := testDB(app).NewSelect().
		TableExpr("org_access_sessions").
		ColumnExpr("COUNT(*)").
		Where("org_id = ? AND account_id = ?", orgID, accountID).
		Scan(context.Background(), &count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestOrgScope_DoesNotCreateOrgAccessSessionWhenRevocationDisabled(t *testing.T) {
	app := newTestApplication(t)
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.SessionsRevocationEnabled = false
	})

	account, err := testDB(app).InsertAccount(context.Background(), "orgctx-no-session@example.com", "Org Ctx No Session", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := testDB(app).InsertOrg(context.Background(), "orgctx-no-session", "Org Ctx No Session")
	if err != nil {
		t.Fatal(err)
	}
	if err = testDB(app).AddOrgMember(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}

	rec, _ := serveOrgScope(t, app, account, org.Slug)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if count := countOrgAccessSessions(t, app, org.ID, account.ID); count != 0 {
		t.Fatalf("expected no org access session rows, got %d", count)
	}
}

func TestOrgScope_UnknownSlug(t *testing.T) {
	app := newTestApplication(t)

	account, err := testDB(app).InsertAccount(context.Background(), "orgctx-test@example.com", "Org Ctx Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	rec, _ := serveOrgScope(t, app, account, "nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestOrgScope_NonMember(t *testing.T) {
	app := newTestApplication(t)

	account, err := testDB(app).InsertAccount(context.Background(), "non-member@example.com", "Non Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := testDB(app).InsertOrg(context.Background(), "non-member-org", "Non Member Org")
	if err != nil {
		t.Fatal(err)
	}

	rec, _ := serveOrgScope(t, app, account, org.Slug)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if count := countOrgAccessSessions(t, app, org.ID, account.ID); count != 0 {
		t.Fatalf("expected no org access session rows for a non-member, got %d", count)
	}
}

func TestOrgScope_MemberRecordsPathIDsAndOrgAccessSession(t *testing.T) {
	app := newTestApplication(t)
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.SessionsRevocationEnabled = true
	})

	account, err := testDB(app).InsertAccount(context.Background(), "member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := testDB(app).InsertOrg(context.Background(), "member-org", "Member Org")
	if err != nil {
		t.Fatal(err)
	}
	if err = testDB(app).AddOrgMember(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}

	rec, ids := serveOrgScope(t, app, account, org.Slug)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ids.OrgID != org.ID || ids.OrgSlug != org.Slug {
		t.Fatalf("expected org %d/%s, got %d/%s", org.ID, org.Slug, ids.OrgID, ids.OrgSlug)
	}
	if ids.WorkspaceID != 0 || ids.EnvironmentID != 0 || ids.ConnectionID != 0 {
		t.Fatalf("expected no nested path ids, got %+v", ids)
	}
	if count := countOrgAccessSessions(t, app, org.ID, account.ID); count != 1 {
		t.Fatalf("expected one org access session row, got %d", count)
	}
}

func TestOrgScope_RejectsRevokedOrgAccessSession(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.SessionsRevocationEnabled = true
	})

	account, err := testDB(app).InsertAccount(ctx, "revoked-org-access@example.com", "Revoked Org Access", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := testDB(app).InsertOrg(ctx, "revoked-org-access", "Revoked Org Access")
	if err != nil {
		t.Fatal(err)
	}
	if err = testDB(app).AddOrgMember(ctx, org.ID, account.ID); err != nil {
		t.Fatal(err)
	}

	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)
	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	serve := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req = testWithParams(req, map[string]string{"org_slug": org.Slug})
		rec := httptest.NewRecorder()
		app.authenticateV1(app.requireAccount(app.orgScope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))).ServeHTTP(rec, req)
		return rec.Code
	}
	if code := serve(); code != http.StatusOK {
		t.Fatalf("expected 200 before revocation, got %d", code)
	}
	session, found, err := testDB(app).GetOrgAccessSession(ctx, claims.AuthSessionID, org.ID, account.ID)
	if err != nil || !found {
		t.Fatalf("expected org access session: found=%v err=%v", found, err)
	}
	if err := testDB(app).RevokeOrgAccessSession(ctx, session.ID, org.ID, &account.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if code := serve(); code != http.StatusForbidden {
		t.Fatalf("expected 403 after revocation, got %d", code)
	}
}

func TestPathIDMiddlewareRejectsMalformedIDs(t *testing.T) {
	app := newTestApplication(t)
	middlewares := map[string]struct {
		param string
		mw    func(http.Handler) http.Handler
		get   func(pathIDs) int64
	}{
		"workspace":   {"ws_id", app.workspacePath, func(ids pathIDs) int64 { return ids.WorkspaceID }},
		"environment": {"env_id", app.environmentPath, func(ids pathIDs) int64 { return ids.EnvironmentID }},
		"connection":  {"conn_id", app.connectionPath, func(ids pathIDs) int64 { return ids.ConnectionID }},
	}
	for name, tt := range middlewares {
		for _, raw := range []string{"", "bad", "0", "-1", "1.5", "99999999999999999999"} {
			called := false
			req := testWithParams(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{tt.param: raw})
			rec := httptest.NewRecorder()
			tt.mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound || called {
				t.Fatalf("%s %q: expected 404 without calling next, got %d (called=%v)", name, raw, rec.Code, called)
			}
		}

		var got int64
		req := testWithParams(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{tt.param: "42"})
		req = contextSetPathIDs(req, pathIDs{OrgID: 7, OrgSlug: "acme"})
		rec := httptest.NewRecorder()
		tt.mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ids := contextGetPathIDs(r)
			if ids.OrgID != 7 || ids.OrgSlug != "acme" {
				t.Fatalf("%s: parent path ids lost: %+v", name, ids)
			}
			got = tt.get(ids)
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || got != 42 {
			t.Fatalf("%s: expected 200 with id 42, got %d with id %d", name, rec.Code, got)
		}
	}
}

func sendWithAuthorization(t *testing.T, app *application, path, header string) testResponse {
	t.Helper()
	req := newTestRequest(t, http.MethodGet, path, nil)
	req.Header.Set("Authorization", header)
	return send(t, req, app.routes())
}

func newTestSessionToken(t *testing.T, app *application) (string, string) {
	t.Helper()
	account := seedAccount(t, app, uniqueEmail(t, "session"), "Session Account")
	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)
	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	assert.Nil(t, err)
	return tok, claims.AuthSessionID
}

func revokeTestSession(t *testing.T, app *application, sessionID string) {
	t.Helper()
	assert.Nil(t, testDB(app).RevokeAuthSession(context.Background(), sessionID, nil, "test"))
}

func TestAuthenticateRejectsUnclaimedCredential(t *testing.T) {
	app := newTestApplication(t)
	for _, header := range []string{"Basic abc", "Bearer", "Bearer a b"} {
		res := sendWithAuthorization(t, app, "/api/v1/me", header)
		assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
		assert.Equal(t, res.ErrorCode(), "invalid_authentication_token")
	}
}

func TestAuthenticateRevokedSessionReturnsReason(t *testing.T) {
	app := newTestApplication(t)
	token, sessionID := newTestSessionToken(t, app)
	revokeTestSession(t, app, sessionID)
	res := sendWithAuthorization(t, app, "/api/v1/me", "Bearer "+token)
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
	assert.Equal(t, res.ErrorReason(), "auth_session_revoked")
}

func TestAuthenticatePutsPrincipalOnContext(t *testing.T) {
	app := newTestApplication(t)
	token, sessionID := newTestSessionToken(t, app)
	var got access.Principal
	handler := app.authenticateV1(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = contextGetPrincipal(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "203.0.113.9:1234"
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, got.Subject.Kind, access.SubjectAccount)
	assert.Equal(t, got.Credential.ID, sessionID)
	assert.Equal(t, got.Request.ClientIP.String(), "203.0.113.9")
}

func TestClientIPHonorsTrustedProxy(t *testing.T) {
	app := newTestApplication(t)
	app.clientIPs = clientip.New([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	assert.Equal(t, app.clientIP(req), "198.51.100.7")
}
