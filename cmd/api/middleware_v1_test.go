package main

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
	tok, _, err := token.Issue(strconv.FormatInt(accountID, 10), email, name, app.config.jwt.secretKey)
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
