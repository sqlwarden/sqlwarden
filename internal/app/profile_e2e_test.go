package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/config"
)

func TestDesktopProfileEndToEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := testConfig(t)
	cfg.Profile = config.ProfileDesktop
	cfg.Desktop.AppDir = t.TempDir()
	built, err := Build(t.Context(), Options{Config: cfg, Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	h := built.httpHandler()

	do := func(method, path, body, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	status := do(http.MethodGet, "/api/setup/status", "", "")
	var s map[string]any
	if err := json.Unmarshal(status.Body.Bytes(), &s); err != nil {
		t.Fatalf("status body = %s", status.Body)
	}
	if s["setup_requires_input"] != false || s["invitations_enabled"] != false {
		t.Fatalf("status = %v", s)
	}

	setup := do(http.MethodPost, "/api/setup", "{}", "")
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s", setup.Code, setup.Body)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		Organization struct {
			Slug string `json:"slug"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(setup.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Organization.Slug != "local" || body.AccessToken == "" {
		t.Fatalf("setup body = %s", setup.Body)
	}

	if rec := do(http.MethodGet, "/api/v1/me", "", body.AccessToken); rec.Code != http.StatusOK {
		t.Fatalf("me = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/api/v1/auth/login", `{"email":"x@example.com","password":"y"}`, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("login = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, "/api/v1/orgs/local/invitations", "", body.AccessToken); rec.Code != http.StatusNotFound {
		t.Fatalf("invitations = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/api/setup", "{}", ""); rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d %s", rec.Code, rec.Body)
	}
}
