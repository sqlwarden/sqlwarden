package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/token"
)

func newTestApplicationWithEnforcer(t *testing.T) *application {
	t.Helper()
	app := newTestApplication(t)
	enforcer, err := access.New(app.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer = enforcer
	return app
}

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
	authSession, err := app.db.InsertAuthSession(context.Background(), accountID, time.Now().Add(7*24*time.Hour), "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := token.IssueWithSessionTTL(strconv.FormatInt(accountID, 10), authSession.ID, email, name, app.config.JWT.SecretKey, app.config.JWT.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAuthenticateV1_ValidToken(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "auth-test@example.com", "Auth Test", nil)
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
	app := newTestApplicationWithEnforcer(t)

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
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "missing-session@example.com", "Missing Session", nil)
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
	app := newTestApplicationWithEnforcer(t)
	app.config.Sessions.RevocationEnabled = false

	account, err := app.db.InsertAccount(context.Background(), "revocation-disabled@example.com", "Revocation Disabled", nil)
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
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "revoked-session@example.com", "Revoked Session", nil)
	if err != nil {
		t.Fatal(err)
	}
	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)
	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = app.db.RevokeAuthSession(context.Background(), claims.AuthSessionID, &account.ID, "test"); err != nil {
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

func TestOrgCtx_DoesNotCreateOrgAccessSessionWhenRevocationDisabled(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)
	app.config.Sessions.RevocationEnabled = false

	account, err := app.db.InsertAccount(context.Background(), "orgctx-no-session@example.com", "Org Ctx No Session", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := app.db.InsertOrg(context.Background(), "orgctx-no-session", "Org Ctx No Session")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.db.AddOrgMember(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/orgctx-no-session", nil)
	req = contextSetAccount(req, account)
	req = testWithParams(req, map[string]string{"org_slug": org.Slug})

	rec := httptest.NewRecorder()
	app.orgCtx(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var count int
	err = app.db.NewSelect().
		TableExpr("org_access_sessions").
		ColumnExpr("COUNT(*)").
		Where("org_id = ? AND account_id = ?", org.ID, account.ID).
		Scan(context.Background(), &count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no org access session rows, got %d", count)
	}
}

func TestRequireAccount_NoAccount(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

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

func TestOrgCtx_UnknownSlug(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "orgctx-test@example.com", "Org Ctx Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/nonexistent", nil)
	req = contextSetAccount(req, account)
	req = testWithParams(req, map[string]string{"org_slug": "nonexistent"})

	rec := httptest.NewRecorder()

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app.orgCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestOrgCtx_NonMember(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "non-member@example.com", "Non Member", nil)
	if err != nil {
		t.Fatal(err)
	}

	org, err := app.db.InsertOrg(context.Background(), "non-member-org", "Non Member Org")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug, nil)
	authSession, err := app.db.InsertAuthSession(context.Background(), account.ID, time.Now().Add(7*24*time.Hour), "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	req = contextSetAuthSession(req, authSession)
	req = contextSetAccount(req, account)
	req = testWithParams(req, map[string]string{"org_slug": org.Slug})

	rec := httptest.NewRecorder()

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app.orgCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestOrgCtx_Member(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}

	org, err := app.db.InsertOrg(context.Background(), "member-org", "Member Org")
	if err != nil {
		t.Fatal(err)
	}

	err = app.db.AddOrgMember(context.Background(), org.ID, account.ID)
	if err != nil {
		t.Fatal(err)
	}

	var gotSlug string
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o := contextGetOrg(r)
		gotSlug = o.Slug
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+org.Slug, nil)
	authSession, err := app.db.InsertAuthSession(context.Background(), account.ID, time.Now().Add(7*24*time.Hour), "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	req = contextSetAuthSession(req, authSession)
	req = contextSetAccount(req, account)
	req = testWithParams(req, map[string]string{"org_slug": org.Slug})

	rec := httptest.NewRecorder()
	app.orgCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotSlug != org.Slug {
		t.Fatalf("expected slug %s, got %s", org.Slug, gotSlug)
	}
}

func TestWsCtx_UnknownWsID(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "wsctx-test@example.com", "WS Ctx Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	org, err := app.db.InsertOrg(context.Background(), "ws-org-ctx", "WS Org")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/ws-org/workspaces/99999", nil)
	req = contextSetAccount(req, account)
	req = contextSetOrg(req, org)
	req = testWithParams(req, map[string]string{"org_slug": org.Slug, "ws_id": "99999"})

	rec := httptest.NewRecorder()

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app.wsCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestWsCtx_InvalidOrWrongOrg(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "wsctx2@example.com", "WS Ctx 2", nil)
	if err != nil {
		t.Fatal(err)
	}
	orgA, err := app.db.InsertOrg(context.Background(), "ws-org-a", "WS Org A")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := app.db.InsertOrg(context.Background(), "ws-org-b", "WS Org B")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := app.db.InsertWorkspace(context.Background(), &orgB.ID, "org", orgB.ID, "Other Org WS", "")
	if err != nil {
		t.Fatal(err)
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/ws-org-a/workspaces/not-a-number", nil)
	invalidReq = contextSetAccount(invalidReq, account)
	invalidReq = contextSetOrg(invalidReq, orgA)
	invalidReq = testWithParams(invalidReq, map[string]string{"org_slug": orgA.Slug, "ws_id": "not-a-number"})
	invalidRec := httptest.NewRecorder()
	app.wsCtx(finalHandler).ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid workspace ID, got %d", invalidRec.Code)
	}

	wrongOrgReq := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/ws-org-a/workspaces/1", nil)
	wrongOrgReq = contextSetAccount(wrongOrgReq, account)
	wrongOrgReq = contextSetOrg(wrongOrgReq, orgA)
	wrongOrgReq = testWithParams(wrongOrgReq, map[string]string{"org_slug": orgA.Slug, "ws_id": strconv.FormatInt(ws.ID, 10)})
	wrongOrgRec := httptest.NewRecorder()
	app.wsCtx(finalHandler).ServeHTTP(wrongOrgRec, wrongOrgReq)
	if wrongOrgRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-org workspace, got %d", wrongOrgRec.Code)
	}
}

func TestEnvCtxBranches(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "envctx@example.com", "EnvCtx", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := app.db.InsertOrg(context.Background(), "env-org", "Env Org")
	if err != nil {
		t.Fatal(err)
	}
	ws1, err := app.db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "WS1", "")
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := app.db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "WS2", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.db.InsertEnvironment(context.Background(), ws2.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for name, envID := range map[string]string{"invalid": "bad", "missing": "99999"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/env-org/workspaces/1/environments/"+envID, nil)
		req = contextSetAccount(req, account)
		req = contextSetOrg(req, org)
		req = contextSetWorkspace(req, ws1)
		req = testWithParams(req, map[string]string{"env_id": envID})
		rec := httptest.NewRecorder()
		app.envCtx(finalHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", name, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/env-org/workspaces/1/environments/"+strconv.FormatInt(env.ID, 10), nil)
	req = contextSetAccount(req, account)
	req = contextSetOrg(req, org)
	req = contextSetWorkspace(req, ws1)
	req = testWithParams(req, map[string]string{"env_id": strconv.FormatInt(env.ID, 10)})
	rec := httptest.NewRecorder()
	app.envCtx(finalHandler).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-workspace environment, got %d", rec.Code)
	}
}

func TestConnCtxBranches(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "connctx@example.com", "ConnCtx", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := app.db.InsertOrg(context.Background(), "conn-org", "Conn Org")
	if err != nil {
		t.Fatal(err)
	}
	ws1, err := app.db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "WS1", "")
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := app.db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "WS2", "")
	if err != nil {
		t.Fatal(err)
	}
	env1ID, err := app.db.DefaultEnvironmentID(context.Background(), ws1.ID)
	if err != nil {
		t.Fatal(err)
	}
	env1, found, err := app.db.GetEnvironment(context.Background(), env1ID)
	if err != nil || !found {
		t.Fatal("expected default environment for ws1")
	}
	conn, err := app.db.InsertConnection(context.Background(), ws2.ID, nil, "db", "sqlite", ":memory:", "open")
	if err != nil {
		t.Fatal(err)
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for name, connID := range map[string]string{"invalid": "bad", "missing": "99999"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/conn-org/workspaces/1/environments/1/connections/"+connID, nil)
		req = contextSetAccount(req, account)
		req = contextSetOrg(req, org)
		req = contextSetWorkspace(req, ws1)
		req = contextSetEnvironment(req, env1)
		req = testWithParams(req, map[string]string{"conn_id": connID})
		rec := httptest.NewRecorder()
		app.connCtx(finalHandler).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", name, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/conn-org/workspaces/1/environments/1/connections/"+strconv.FormatInt(conn.ID, 10), nil)
	req = contextSetAccount(req, account)
	req = contextSetOrg(req, org)
	req = contextSetWorkspace(req, ws1)
	req = contextSetEnvironment(req, env1)
	req = testWithParams(req, map[string]string{"conn_id": strconv.FormatInt(conn.ID, 10)})
	rec := httptest.NewRecorder()
	app.connCtx(finalHandler).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-workspace connection, got %d", rec.Code)
	}
}

func TestRequireConcreteResourcePermissions(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)
	ctx := context.Background()

	account, err := app.db.InsertAccount(ctx, "resource-perms@example.com", "Resource Perms", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := app.db.InsertOrg(ctx, "resource-perms-org", "Resource Perms Org")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddOrgMember(ctx, org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	ws, err := app.db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Resource Workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.db.InsertEnvironment(ctx, ws.ID, "production", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := app.db.InsertConnection(ctx, ws.ID, &env.ID, "primary", "sqlite", "dsn", "open")
	if err != nil {
		t.Fatal(err)
	}

	orgRoleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgWrite)
	workspaceRoleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", access.PermWsWrite)
	envRoleID := createRoleForTest(t, app, org.ID, &ws.ID, "environment", access.PermEnvWrite)
	connRoleID := createRoleForTest(t, app, org.ID, &ws.ID, "connection", access.PermConnWrite)

	for _, binding := range []struct {
		roleID       int64
		resourceType string
		resourceID   int64
	}{
		{orgRoleID, "org", org.ID},
		{workspaceRoleID, "workspace", ws.ID},
		{envRoleID, "environment", env.ID},
		{connRoleID, "connection", conn.ID},
	} {
		if err := app.enforcer.BindRole(ctx, org.ID, binding.roleID, access.SubjectTypeAccount, account.ID, binding.resourceType, binding.resourceID, account.ID); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name       string
		middleware func(http.Handler) http.Handler
		request    func() *http.Request
	}{
		{
			name:       "org",
			middleware: app.requireOrgPermission(access.PermOrgWrite),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/orgs/resource-perms-org", nil)
				req = contextSetAccount(req, account)
				return contextSetOrg(req, org)
			},
		},
		{
			name:       "workspace",
			middleware: app.requireWorkspacePermission(access.PermWsWrite),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/orgs/resource-perms-org/workspaces/1", nil)
				req = contextSetAccount(req, account)
				req = contextSetOrg(req, org)
				return contextSetWorkspace(req, ws)
			},
		},
		{
			name:       "environment",
			middleware: app.requireEnvironmentPermission(access.PermEnvWrite),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/orgs/resource-perms-org/workspaces/1/environments/1", nil)
				req = contextSetAccount(req, account)
				req = contextSetOrg(req, org)
				req = contextSetWorkspace(req, ws)
				return contextSetEnvironment(req, env)
			},
		},
		{
			name:       "connection",
			middleware: app.requireConnectionPermission(access.PermConnWrite),
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/orgs/resource-perms-org/workspaces/1/environments/1/connections/1", nil)
				req = contextSetAccount(req, account)
				req = contextSetOrg(req, org)
				req = contextSetWorkspace(req, ws)
				req = contextSetEnvironment(req, env)
				return contextSetConnection(req, conn)
			},
		},
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		tt.middleware(finalHandler).ServeHTTP(rec, tt.request())
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", tt.name, rec.Code)
		}
	}
}

func TestRequireConcreteResourcePermissionMissingContext(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)
	account, err := app.db.InsertAccount(context.Background(), "missing-context@example.com", "Missing Context", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := app.db.InsertOrg(context.Background(), "missing-context-org", "Missing Context Org")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		middleware func(http.Handler) http.Handler
		request    *http.Request
	}{
		{name: "org", middleware: app.requireOrgPermission(access.PermOrgRead), request: contextSetAccount(httptest.NewRequest(http.MethodGet, "/", nil), account)},
		{name: "workspace", middleware: app.requireWorkspacePermission(access.PermWsRead), request: contextSetOrg(contextSetAccount(httptest.NewRequest(http.MethodGet, "/", nil), account), org)},
		{name: "environment", middleware: app.requireEnvironmentPermission(access.PermEnvRead), request: contextSetOrg(contextSetAccount(httptest.NewRequest(http.MethodGet, "/", nil), account), org)},
		{name: "connection", middleware: app.requireConnectionPermission(access.PermConnRead), request: contextSetOrg(contextSetAccount(httptest.NewRequest(http.MethodGet, "/", nil), account), org)},
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		tt.middleware(finalHandler).ServeHTTP(rec, tt.request)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", tt.name, rec.Code)
		}
	}
}

func TestSpaceEnvCtxBranches(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount(context.Background(), "spaceenv@example.com", "SpaceEnv", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := app.db.InsertWorkspace(context.Background(), nil, "space", account.ID, "Personal", "")
	if err != nil {
		t.Fatal(err)
	}
	otherWS, err := app.db.InsertWorkspace(context.Background(), nil, "space", account.ID, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.db.InsertEnvironment(context.Background(), otherWS.ID, "prod", "")
	if err != nil {
		t.Fatal(err)
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/workspaces/1/environments/not-a-number", nil)
	invalidReq = contextSetAccount(invalidReq, account)
	invalidReq = contextSetWorkspace(invalidReq, ws)
	invalidReq = testWithParams(invalidReq, map[string]string{"env_id": "not-a-number"})
	invalidRec := httptest.NewRecorder()
	app.spaceEnvCtx(finalHandler).ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid personal env ID, got %d", invalidRec.Code)
	}

	crossReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/workspaces/1/environments/"+strconv.FormatInt(env.ID, 10), nil)
	crossReq = contextSetAccount(crossReq, account)
	crossReq = contextSetWorkspace(crossReq, ws)
	crossReq = testWithParams(crossReq, map[string]string{"env_id": strconv.FormatInt(env.ID, 10)})
	crossRec := httptest.NewRecorder()
	app.spaceEnvCtx(finalHandler).ServeHTTP(crossRec, crossReq)
	if crossRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for env in different personal workspace, got %d", crossRec.Code)
	}
}
