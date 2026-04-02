package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/encrypt"
)

func newTestApp(t *testing.T) *application {
	t.Helper()
	app := newTestApplication(t)
	enforcer, err := access.New(app.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer = enforcer
	app.encKey = encrypt.DeriveKey("test-encryption-key-32bytes!!!!!")
	app.connManager = connection.New(30 * time.Minute)
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

func TestGetAccountOrgs(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "orgs@example.com", "Orgs User", "securepass99")

	loginRes := loginTestUser(t, app, "orgs@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	req := newTestRequest(t, http.MethodGet, "/api/v1/account/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	// Parse as array
	var tenants []map[string]any
	err := json.Unmarshal(res.BodyBytes, &tenants)
	if err != nil {
		t.Fatalf("expected JSON array, got error: %v", err)
	}
}
