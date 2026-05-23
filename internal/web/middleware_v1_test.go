package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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
	tok, _, err := token.Issue(strconv.FormatInt(accountID, 10), email, name, app.config.JWT.SecretKey)
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
