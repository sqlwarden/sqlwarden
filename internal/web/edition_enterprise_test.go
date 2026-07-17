//go:build enterprise

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sqlwarden/internal/license"
)

// The unlicensed-enterprise invariant: an enterprise binary without a
// license key must behave exactly like community. Core parity is enforced
// by running the whole web suite under -tags enterprise; this test enforces
// the enterprise-surface half — gated extension routes refuse with the
// standard envelope instead of working.
func TestEnterpriseBinaryUnlicensedActsAsCommunity(t *testing.T) {
	app := newTestApplication(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/edition")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var edition struct {
		Edition          string   `json:"edition"`
		LicensedFeatures []string `json:"licensed_features"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&edition); err != nil {
		t.Fatal(err)
	}
	// Reporting the enterprise edition (with no licensed features) is the
	// single sanctioned difference from community — the apply-key UX needs it.
	if edition.Edition != "enterprise" {
		t.Fatalf("edition = %q, want enterprise", edition.Edition)
	}
	if len(edition.LicensedFeatures) != 0 {
		t.Fatalf("licensed_features = %v, want empty without a key", edition.LicensedFeatures)
	}

	_, token := seedInstanceAdminAccount(t, app, uniqueEmail(t, "edition-admin"), "Edition Admin")
	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/ee/stub", nil, token), app.routes())
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("unlicensed stub route status = %d, want 403", res.StatusCode)
	}
	errorBody, _ := res.BodyFields["error"].(map[string]any)
	if code, _ := errorBody["code"].(string); code != license.CodeRequired {
		t.Fatalf("error code = %q, want %q", code, license.CodeRequired)
	}
	if message, _ := errorBody["message"].(string); message == "" {
		t.Fatal("expected a user-facing error message")
	}
}
