package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/cache"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	schemaapp "github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/token"
)

func newTestApp(t *testing.T) *application {
	t.Helper()
	app := newTestApplication(t)
	enforcer, err := access.New(app.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer = enforcer
	app.connManager = connection.New(30 * time.Minute)
	app.schemaService = schemaapp.NewService(cache.NewMemCache(schemaCacheCapacity), schemaCacheTTL)
	t.Cleanup(func() { app.connManager.Close() })
	return app
}

// registerTestUser is a helper that registers a user and returns the response.
func registerTestUser(t *testing.T, app *application, email, name, pw string) testResponse {
	t.Helper()
	req := newTestRequest(t, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":    email,
		"name":     name,
		"password": pw,
	})
	return send(t, req, app.routes())
}

// loginTestUser is a helper that logs in and returns the response.
func loginTestUser(t *testing.T, app *application, email, pw string) testResponse {
	t.Helper()
	req := newTestRequest(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": pw,
	})
	return send(t, req, app.routes())
}

func extractAccessToken(t *testing.T, res testResponse) string {
	t.Helper()
	tok, ok := res.BodyFields["access_token"].(string)
	if !ok {
		t.Fatal("access_token not found in response")
	}
	return tok
}

// newAuthRequest creates a test request with an Authorization: Bearer token header.
func newAuthRequest(t *testing.T, method, path string, body map[string]any, token string) *http.Request {
	t.Helper()
	req := newTestRequest(t, method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func extractRefreshCookie(t *testing.T, res testResponse) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == "refresh_token" {
			return c
		}
	}
	t.Fatal("refresh_token cookie not found")
	return nil
}

func TestRegisterBlockedBeforeSetup(t *testing.T) {
	app := newTestApp(t)

	res := registerTestUser(t, app, "reg@example.com", "Reg User", "securepass99")
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestRegisterSuccess(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := registerTestUser(t, app, "reg@example.com", "Reg User", "securepass99")
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	assert.Equal(t, res.BodyFields["email"], "reg@example.com")
	assert.Equal(t, res.BodyFields["name"], "Reg User")

	if _, ok := res.BodyFields["id"]; !ok {
		t.Fatal("expected id in response")
	}

	// Duplicate email should fail
	res2 := registerTestUser(t, app, "reg@example.com", "Another", "securepass99")
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestRegisterValidation(t *testing.T) {
	app := newTestApp(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing email", map[string]any{"name": "Test", "password": "securepass99"}},
		{"missing name", map[string]any{"email": "test@example.com", "password": "securepass99"}},
		{"missing password", map[string]any{"email": "test@example.com", "name": "Test"}},
		{"short password", map[string]any{"email": "test@example.com", "name": "Test", "password": "short"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newTestRequest(t, http.MethodPost, "/api/v1/auth/register", tc.body)
			res := send(t, req, app.routes())
			assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "login@example.com", "Login User", "securepass99")

	res := loginTestUser(t, app, "login@example.com", "securepass99")
	assert.Equal(t, res.StatusCode, http.StatusOK)

	tok := extractAccessToken(t, res)
	if tok == "" {
		t.Fatal("expected non-empty access_token")
	}

	cookie := extractRefreshCookie(t, res)
	if cookie.Value == "" {
		t.Fatal("expected non-empty refresh_token cookie")
	}

	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	assert.Nil(t, err)
	if claims.AuthSessionID == "" {
		t.Fatal("expected auth_session_id claim")
	}
	rt, found, err := app.db.GetRefreshTokenByHash(context.Background(), token.Hash(cookie.Value))
	assert.Nil(t, err)
	assert.True(t, found)
	assert.Equal(t, rt.AuthSessionID, claims.AuthSessionID)
}

func TestLoginWrongPassword(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "wrongpw@example.com", "WrongPW", "securepass99")

	res := loginTestUser(t, app, "wrongpw@example.com", "wrongpassword1")
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestLoginUnknownEmail(t *testing.T) {
	app := newTestApp(t)

	res := loginTestUser(t, app, "noone@example.com", "securepass99")
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestLoginValidationAndInactiveAccount(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	validationRes := loginTestUser(t, app, "", "")
	assert.Equal(t, validationRes.StatusCode, http.StatusUnprocessableEntity)

	registerTestUser(t, app, "inactive@example.com", "Inactive", "securepass99")
	account, found, err := app.db.GetAccountByEmail(context.Background(), "inactive@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected inactive test account to exist")
	}
	if err := app.db.DeactivateAccount(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}

	inactiveRes := loginTestUser(t, app, "inactive@example.com", "securepass99")
	assert.Equal(t, inactiveRes.StatusCode, http.StatusUnauthorized)
}

func TestRefreshValid(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "refresh@example.com", "Refresh User", "securepass99")

	loginRes := loginTestUser(t, app, "refresh@example.com", "securepass99")
	assert.Equal(t, loginRes.StatusCode, http.StatusOK)

	cookie := extractRefreshCookie(t, loginRes)

	req := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(cookie)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusOK)

	tok := extractAccessToken(t, res)
	if tok == "" {
		t.Fatal("expected non-empty access_token after refresh")
	}

	loginClaims, err := token.Verify(extractAccessToken(t, loginRes), app.config.JWT.SecretKey)
	assert.Nil(t, err)
	refreshClaims, err := token.Verify(tok, app.config.JWT.SecretKey)
	assert.Nil(t, err)
	assert.Equal(t, refreshClaims.AuthSessionID, loginClaims.AuthSessionID)
}

func TestRefreshAfterLogout(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "rlogout@example.com", "RLogout User", "securepass99")

	loginRes := loginTestUser(t, app, "rlogout@example.com", "securepass99")
	cookie := extractRefreshCookie(t, loginRes)

	// Logout
	logoutReq := newTestRequest(t, http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRes := send(t, logoutReq, app.routes())
	assert.Equal(t, logoutRes.StatusCode, http.StatusNoContent)

	// Refresh with old cookie should fail
	refreshReq := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshReq.AddCookie(cookie)
	refreshRes := send(t, refreshReq, app.routes())
	assert.Equal(t, refreshRes.StatusCode, http.StatusUnauthorized)
}

func TestRefreshMissingOrUnknownCookie(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	missingReq := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	missingRes := send(t, missingReq, app.routes())
	assert.Equal(t, missingRes.StatusCode, http.StatusUnauthorized)

	unknownReq := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	unknownReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: "missing-family"})
	unknownRes := send(t, unknownReq, app.routes())
	assert.Equal(t, unknownRes.StatusCode, http.StatusUnauthorized)
}

func TestRefreshWithExpiredOrMissingAccount(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "refresh-edge@example.com", "Refresh Edge", "securepass99")

	loginRes := loginTestUser(t, app, "refresh-edge@example.com", "securepass99")
	assert.Equal(t, loginRes.StatusCode, http.StatusOK)
	cookie := extractRefreshCookie(t, loginRes)

	rt, found, err := app.db.GetRefreshTokenByHash(context.Background(), token.Hash(cookie.Value))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected refresh token to exist")
	}

	if _, err := app.db.NewUpdate().Model((*database.RefreshToken)(nil)).
		Set("expires_at = ?", time.Now().Add(-time.Hour)).
		Where("id = ?", rt.ID).
		Exec(context.Background()); err != nil {
		t.Fatal(err)
	}

	expiredReq := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	expiredReq.AddCookie(cookie)
	expiredRes := send(t, expiredReq, app.routes())
	assert.Equal(t, expiredRes.StatusCode, http.StatusUnauthorized)

	registerTestUser(t, app, "refresh-missing-account@example.com", "Refresh Missing", "securepass99")
	loginRes = loginTestUser(t, app, "refresh-missing-account@example.com", "securepass99")
	assert.Equal(t, loginRes.StatusCode, http.StatusOK)
	cookie = extractRefreshCookie(t, loginRes)

	rt, found, err = app.db.GetRefreshTokenByHash(context.Background(), token.Hash(cookie.Value))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected refresh token to exist")
	}

	if _, err := app.db.NewDelete().Model((*database.Account)(nil)).Where("id = ?", rt.AccountID).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}

	missingAccountReq := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	missingAccountReq.AddCookie(cookie)
	missingAccountRes := send(t, missingAccountReq, app.routes())
	assert.Equal(t, missingAccountRes.StatusCode, http.StatusUnauthorized)
}

func TestLogout(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "logout@example.com", "Logout User", "securepass99")

	loginRes := loginTestUser(t, app, "logout@example.com", "securepass99")
	cookie := extractRefreshCookie(t, loginRes)

	req := newTestRequest(t, http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Check that the cookie is cleared
	for _, c := range res.Cookies() {
		if c.Name == "refresh_token" {
			assert.Equal(t, c.MaxAge, -1)
		}
	}

	claims, err := token.Verify(extractAccessToken(t, loginRes), app.config.JWT.SecretKey)
	assert.Nil(t, err)
	session, found, err := app.db.GetAuthSession(context.Background(), claims.AuthSessionID, mustParseInt64(t, claims.AccountID))
	assert.Nil(t, err)
	assert.True(t, found)
	assert.True(t, session.RevokedAt != nil)
}

func TestRevokedAuthSessionRejectsExistingAccessToken(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "revoked-token@example.com", "Revoked Token", "securepass99")
	loginRes := loginTestUser(t, app, "revoked-token@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)
	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	assert.Nil(t, err)
	accountID := mustParseInt64(t, claims.AccountID)

	err = app.db.RevokeAuthSession(context.Background(), claims.AuthSessionID, &accountID, "test")
	assert.Nil(t, err)

	var logs bytes.Buffer
	app.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/account", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
	assert.True(t, strings.Contains(logs.String(), "authentication session rejected"))
	assert.True(t, strings.Contains(logs.String(), "auth_session_revoked"))
	assert.True(t, strings.Contains(logs.String(), claims.AuthSessionID))
	assert.False(t, strings.Contains(logs.String(), tok))
}

func TestAccountSessionsListAndRevoke(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "account-sessions@example.com", "Account Sessions", "securepass99")
	loginRes := loginTestUser(t, app, "account-sessions@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)
	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	assert.Nil(t, err)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/account/sessions", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []database.AuthSession `json:"items"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0].ID, claims.AuthSessionID)

	revokeRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/account/sessions/"+claims.AuthSessionID, nil, tok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	blocked := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/account", nil, tok), app.routes())
	assert.Equal(t, blocked.StatusCode, http.StatusUnauthorized)
}

func TestOrgAccessSessionRevocationBlocksOnlyThatOrg(t *testing.T) {
	app := newTestApp(t)
	tok := setupInstance(t, app, "org-session-owner@example.com", "Org Session Owner", "securepass99")

	orgARes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "Org Session A"}, tok), app.routes())
	assert.Equal(t, orgARes.StatusCode, http.StatusCreated)
	orgASlug := orgARes.BodyFields["slug"].(string)

	orgBRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "Org Session B"}, tok), app.routes())
	assert.Equal(t, orgBRes.StatusCode, http.StatusCreated)
	orgBSlug := orgBRes.BodyFields["slug"].(string)

	orgAGet := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+orgASlug, nil, tok), app.routes())
	assert.Equal(t, orgAGet.StatusCode, http.StatusOK)
	orgBGet := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+orgBSlug, nil, tok), app.routes())
	assert.Equal(t, orgBGet.StatusCode, http.StatusOK)

	claims, err := token.Verify(tok, app.config.JWT.SecretKey)
	assert.Nil(t, err)
	accountID := mustParseInt64(t, claims.AccountID)
	orgAID := mustParseInt64(t, fmt.Sprintf("%v", orgARes.BodyFields["id"]))

	orgAccess, found, err := app.db.GetOrgAccessSession(context.Background(), claims.AuthSessionID, orgAID, accountID)
	assert.Nil(t, err)
	assert.True(t, found)
	err = app.db.RevokeOrgAccessSession(context.Background(), orgAccess.ID, orgAID, &accountID, "test")
	assert.Nil(t, err)

	blocked := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+orgASlug, nil, tok), app.routes())
	assert.Equal(t, blocked.StatusCode, http.StatusForbidden)

	stillAllowed := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+orgBSlug, nil, tok), app.routes())
	assert.Equal(t, stillAllowed.StatusCode, http.StatusOK)
}

func TestGetAccount(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "getacc@example.com", "GetAcc User", "securepass99")

	loginRes := loginTestUser(t, app, "getacc@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	// With valid JWT
	req := newTestRequest(t, http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["email"], "getacc@example.com")

	// Without JWT
	reqNoAuth := newTestRequest(t, http.MethodGet, "/api/v1/account", nil)
	resNoAuth := send(t, reqNoAuth, app.routes())
	assert.Equal(t, resNoAuth.StatusCode, http.StatusUnauthorized)
}

func TestUpdateAccount(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, uniqueEmail(t, "account-update-admin"), "Admin", "securepass99")

	registerTestUser(t, app, "update-account@example.com", "Old Name", "securepass99")
	loginRes := loginTestUser(t, app, "update-account@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/account", map[string]any{
		"name": "New Name",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["name"], "New Name")
	assert.Equal(t, res.BodyFields["email"], "update-account@example.com")

	account, found, err := app.db.GetAccountByEmail(t.Context(), "update-account@example.com")
	assert.Nil(t, err)
	assert.True(t, found)
	assert.Equal(t, account.Name, "New Name")
}

func TestUpdateAccountValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := seedAccountWithToken(t, app, uniqueEmail(t, "account-update-validation"), "Validation User")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/account", map[string]any{
		"name": " ",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "name")
}

func TestUpdateAccountPassword(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, uniqueEmail(t, "account-password-admin"), "Admin", "securepass99")

	registerTestUser(t, app, "password-account@example.com", "Password User", "oldpass99")
	loginRes := loginTestUser(t, app, "password-account@example.com", "oldpass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/account/password", map[string]any{
		"current_password": "oldpass99",
		"new_password":     "newpass99",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	oldLogin := loginTestUser(t, app, "password-account@example.com", "oldpass99")
	assert.Equal(t, oldLogin.StatusCode, http.StatusUnauthorized)

	newLogin := loginTestUser(t, app, "password-account@example.com", "newpass99")
	assert.Equal(t, newLogin.StatusCode, http.StatusOK)
}

func TestUpdateAccountPasswordRejectsIncorrectCurrentPassword(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, uniqueEmail(t, "account-password-wrong-admin"), "Admin", "securepass99")

	registerTestUser(t, app, "password-wrong@example.com", "Password Wrong", "oldpass99")
	loginRes := loginTestUser(t, app, "password-wrong@example.com", "oldpass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/account/password", map[string]any{
		"current_password": "wrongpass99",
		"new_password":     "newpass99",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "current_password")
}

func TestUpdateAccountPasswordRejectsSSOOnlyAccount(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := seedAccountWithToken(t, app, uniqueEmail(t, "account-password-sso"), "SSO User")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/account/password", map[string]any{
		"current_password": "currentpass99",
		"new_password":     "newpass99",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "current_password")
}

func TestGetAccountOrgs(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "orgs@example.com", "Orgs User", "securepass99")

	loginRes := loginTestUser(t, app, "orgs@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	req := newTestRequest(t, http.MethodGet, "/api/v1/account/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	err := json.Unmarshal(res.BodyBytes, &payload)
	if err != nil {
		t.Fatalf("expected paginated JSON payload, got error: %v", err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 25)
	assert.Equal(t, payload.Total, 0)
	assert.Equal(t, len(payload.Items), 0)
}

func TestGetAccountOrgs_SupportsPaginationSearchAndSort(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, uniqueEmail(t, "account-orgs-admin"), "Admin", "securepass99")

	account, token := seedAccountWithToken(t, app, uniqueEmail(t, "account-orgs"), "Account Orgs User")
	alpha := seedOrganizationForAccount(t, app, account, "Alpha Team")
	zeta := seedOrganizationForAccount(t, app, account, "Zeta Labs")

	res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/account/orgs?q=zeta&sort=name&order=desc&page=1&page_size=1", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	err := json.Unmarshal(res.BodyBytes, &payload)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	if payload.Items[0]["id"] != float64(zeta.ID) {
		t.Fatalf("expected org id %d, got %v", zeta.ID, payload.Items[0]["id"])
	}
	if payload.Items[0]["name"] != zeta.Name {
		t.Fatalf("expected org name %q, got %v", zeta.Name, payload.Items[0]["name"])
	}
	if payload.Items[0]["member_count"] != float64(1) {
		t.Fatalf("expected member_count=1, got %v", payload.Items[0]["member_count"])
	}
	if payload.Items[0]["team_count"] != float64(0) {
		t.Fatalf("expected team_count=0, got %v", payload.Items[0]["team_count"])
	}
	assert.Equal(t, alpha.Name, "Alpha Team")
}

func TestGetAccountOrgs_IncludesComputedRole(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, uniqueEmail(t, "account-orgs-role-admin"), "Admin", "securepass99")

	account, token, org := seedOrgOwner(t, app, uniqueEmail(t, "account-orgs-role"), "Role User", "Role Org")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/account/orgs", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items []struct {
			ID          int64  `json:"id"`
			Role        string `json:"role"`
			MemberCount int    `json:"member_count"`
			TeamCount   int    `json:"team_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 org, got %d", len(payload.Items))
	}
	if payload.Items[0].ID != org.ID {
		t.Fatalf("expected org id %d, got %d", org.ID, payload.Items[0].ID)
	}
	if payload.Items[0].Role != access.BuiltinOrgOwnerRole {
		t.Fatalf("expected role %q, got %s", access.BuiltinOrgOwnerRole, payload.Items[0].Role)
	}
	if payload.Items[0].MemberCount != 1 {
		t.Fatalf("expected member_count=1, got %d", payload.Items[0].MemberCount)
	}
	if payload.Items[0].TeamCount != 0 {
		t.Fatalf("expected team_count=0, got %d", payload.Items[0].TeamCount)
	}
	_ = account
}

func TestGetSession_ReturnsStableBootstrapPayload(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	account, token := seedAccountWithToken(t, app, uniqueEmail(t, "session"), "Session User")
	org := seedOrganizationForAccount(t, app, account, "Acme")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	assertBodyContainsJSONKeys(t, res.BodyBytes, "account", "organizations", "is_instance_admin", "personal_spaces_enabled", "feature_flags")

	var payload struct {
		Account               map[string]any   `json:"account"`
		Organizations         []map[string]any `json:"organizations"`
		IsInstanceAdmin       bool             `json:"is_instance_admin"`
		PersonalSpacesEnabled bool             `json:"personal_spaces_enabled"`
		FeatureFlags          []string         `json:"feature_flags"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)

	if payload.Account == nil {
		t.Fatal("expected account in bootstrap payload")
	}
	assert.Equal(t, len(payload.Organizations), 1)
	if payload.Organizations[0]["name"] != org.Name {
		t.Fatalf("expected first organization name %q, got %v", org.Name, payload.Organizations[0]["name"])
	}
	assert.Equal(t, payload.IsInstanceAdmin, false)
	assert.Equal(t, payload.PersonalSpacesEnabled, true)
	assert.Equal(t, len(payload.FeatureFlags), 0)
}

func TestGetSession(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "session@example.com", "Session User", "securepass99")
	loginRes := loginTestUser(t, app, "session@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	account, found, err := app.db.GetAccountByEmail(t.Context(), "session@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected account to exist")
	}
	if err := app.db.InsertInstanceAdmin(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}

	orgRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "Session Org"}, tok), app.routes())
	assert.Equal(t, orgRes.StatusCode, http.StatusCreated)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/session", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["is_instance_admin"], true)
	assert.Equal(t, res.BodyFields["personal_spaces_enabled"], true)

	accountBody, ok := res.BodyFields["account"].(map[string]any)
	if !ok {
		t.Fatal("expected account object in session response")
	}
	assert.Equal(t, accountBody["email"], "session@example.com")

	orgsBody, ok := res.BodyFields["organizations"].([]any)
	if !ok {
		t.Fatal("expected organizations array in session response")
	}
	if len(orgsBody) != 1 {
		t.Fatalf("expected 1 organization, got %d", len(orgsBody))
	}
}

func TestGetSessionRequiresAuth(t *testing.T) {
	app := newTestApp(t)

	res := send(t, newTestRequest(t, http.MethodGet, "/api/v1/session", nil), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestGetSessionWithoutOrgOrAdmin(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "session-plain@example.com", "Session Plain", "securepass99")
	loginRes := loginTestUser(t, app, "session-plain@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/session", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["is_instance_admin"], false)
	assert.Equal(t, res.BodyFields["personal_spaces_enabled"], true)

	var body struct {
		Organizations []map[string]any `json:"organizations"`
	}
	if err := json.Unmarshal(res.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Organizations) != 0 {
		t.Fatalf("expected no organizations, got %d", len(body.Organizations))
	}
}

func TestGetSessionIncludesPersistedPersonalSpaceFlag(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, token := seedAccountWithToken(t, app, uniqueEmail(t, "session-flag"), "Session Flag")
	if _, err := app.db.UpsertInstanceSettings(context.Background(), database.InstanceSettings{PersonalSpacesEnabled: false}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["personal_spaces_enabled"], false)
}
