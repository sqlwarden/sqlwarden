package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
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
func issueTestToken(t *testing.T, app *application, accountID, email, name string) string {
	t.Helper()
	tok, _, err := token.Issue(accountID, email, name, app.config.jwt.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAuthenticateV1_ValidToken(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount("auth-test@example.com", "Auth Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	tok := issueTestToken(t, app, account.ID, account.Email, account.Name)

	// The final handler checks if the account was set in context.
	var gotAccount bool
	var gotID string
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acc, ok := contextGetAccount(r)
		gotAccount = ok
		if ok {
			gotID = acc.ID
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec := httptest.NewRecorder()
	app.authenticateV1(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !gotAccount {
		t.Fatal("expected account in context, got none")
	}
	if gotID != account.ID {
		t.Fatalf("expected account ID %s, got %s", account.ID, gotID)
	}
}

func TestAuthenticateV1_InvalidToken(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	var gotAccount bool
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := contextGetAccount(r)
		gotAccount = ok
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")

	rec := httptest.NewRecorder()
	app.authenticateV1(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (pass-through), got %d", rec.Code)
	}
	if gotAccount {
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

	account, err := app.db.InsertAccount("orgctx-test@example.com", "Org Ctx Test", nil)
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

	account, err := app.db.InsertAccount("non-member@example.com", "Non Member", nil)
	if err != nil {
		t.Fatal(err)
	}

	tenant, err := app.db.InsertTenant("test-org", "Test Org")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+tenant.Slug, nil)
	req = contextSetAccount(req, account)
	req = testWithParams(req, map[string]string{"org_slug": tenant.Slug})

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

	account, err := app.db.InsertAccount("member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}

	tenant, err := app.db.InsertTenant("member-org", "Member Org")
	if err != nil {
		t.Fatal(err)
	}

	err = app.db.AddTenantMember(tenant.ID, account.ID, "member")
	if err != nil {
		t.Fatal(err)
	}

	var gotTenant bool
	var gotSlug string
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tn, ok := contextGetTenant(r)
		gotTenant = ok
		if ok {
			gotSlug = tn.Slug
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/"+tenant.Slug, nil)
	req = contextSetAccount(req, account)
	req = testWithParams(req, map[string]string{"org_slug": tenant.Slug})

	rec := httptest.NewRecorder()
	app.orgCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !gotTenant {
		t.Fatal("expected tenant in context")
	}
	if gotSlug != tenant.Slug {
		t.Fatalf("expected slug %s, got %s", tenant.Slug, gotSlug)
	}
}

func TestWsCtx_UnknownWsID(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount("wsctx-test@example.com", "WS Ctx Test", nil)
	if err != nil {
		t.Fatal(err)
	}

	tenant, err := app.db.InsertTenant("ws-org", "WS Org")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/ws-org/workspaces/nonexistent", nil)
	req = contextSetAccount(req, account)
	req = contextSetTenant(req, tenant)
	req = testWithParams(req, map[string]string{"org_slug": tenant.Slug, "ws_id": "nonexistent"})

	rec := httptest.NewRecorder()

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app.wsCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestWsCtx_WsBelongsToDifferentTenant(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	account, err := app.db.InsertAccount("ws-cross@example.com", "WS Cross", nil)
	if err != nil {
		t.Fatal(err)
	}

	tenant1, err := app.db.InsertTenant("tenant-one", "Tenant One")
	if err != nil {
		t.Fatal(err)
	}

	tenant2, err := app.db.InsertTenant("tenant-two", "Tenant Two")
	if err != nil {
		t.Fatal(err)
	}

	// Create workspace under tenant2
	ws, err := app.db.InsertWorkspace(tenant2.ID, "Cross WS", "A workspace in tenant2")
	if err != nil {
		t.Fatal(err)
	}

	// Request with tenant1 in context, but ws belongs to tenant2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/tenant-one/workspaces/"+ws.ID, nil)
	req = contextSetAccount(req, account)
	req = contextSetTenant(req, tenant1)
	req = testWithParams(req, map[string]string{"org_slug": tenant1.Slug, "ws_id": ws.ID})

	rec := httptest.NewRecorder()

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app.wsCtx(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRequireOrgRole_Admin(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	// Create tenant and three accounts: owner, admin, member
	tenant, err := app.db.InsertTenant("role-org", "Role Org")
	if err != nil {
		t.Fatal(err)
	}

	ownerAccount, err := app.db.InsertAccount("owner@example.com", "Owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	adminAccount, err := app.db.InsertAccount("admin@example.com", "Admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	memberAccount, err := app.db.InsertAccount("member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add all as tenant members
	for _, acc := range []struct {
		id   string
		role string
	}{
		{ownerAccount.ID, "owner"},
		{adminAccount.ID, "admin"},
		{memberAccount.ID, "member"},
	} {
		err = app.db.AddTenantMember(tenant.ID, acc.id, acc.role)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Seed Casbin policies for the org and assign roles
	err = app.enforcer.SeedOrgPolicies(tenant.Slug, ownerAccount.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = app.enforcer.SetOrgRole(adminAccount.ID, "admin", tenant.Slug)
	if err != nil {
		t.Fatal(err)
	}
	err = app.enforcer.SetOrgRole(memberAccount.ID, "member", tenant.Slug)
	if err != nil {
		t.Fatal(err)
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := app.requireOrgRole("admin")

	tests := []struct {
		name       string
		accountID  string
		account    func() *http.Request
		wantStatus int
	}{
		{
			name: "owner passes admin check",
			account: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req = contextSetAccount(req, ownerAccount)
				req = contextSetTenant(req, tenant)
				return req
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "admin passes admin check",
			account: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req = contextSetAccount(req, adminAccount)
				req = contextSetTenant(req, tenant)
				return req
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "member fails admin check",
			account: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req = contextSetAccount(req, memberAccount)
				req = contextSetTenant(req, tenant)
				return req
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.account()
			rec := httptest.NewRecorder()
			middleware(finalHandler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestRequireSuperadmin_NoAccount(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	rec := httptest.NewRecorder()
	app.requireSuperadmin(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing account, got %d", rec.Code)
	}
}

func TestRequireSuperadmin_NonSuperadmin(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	regularAccount := database.Account{
		ID:           "regular-account-id",
		Email:        "regular@example.com",
		Name:         "Regular User",
		IsActive:     true,
		IsSuperadmin: false,
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req = contextSetAccount(req, regularAccount)
	rec := httptest.NewRecorder()
	app.requireSuperadmin(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-superadmin, got %d", rec.Code)
	}
}

func TestRequireSuperadmin_Superadmin(t *testing.T) {
	app := newTestApplicationWithEnforcer(t)

	superadminAccount := database.Account{
		ID:           "superadmin-account-id",
		Email:        "superadmin@example.com",
		Name:         "Super Admin",
		IsActive:     true,
		IsSuperadmin: true,
	}

	var reached bool
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req = contextSetAccount(req, superadminAccount)
	rec := httptest.NewRecorder()
	app.requireSuperadmin(finalHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for superadmin, got %d", rec.Code)
	}
	if !reached {
		t.Fatal("expected next handler to be called for superadmin")
	}
}
