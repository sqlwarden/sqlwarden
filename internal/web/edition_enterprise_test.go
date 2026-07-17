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

	resp2, err := http.Get(srv.URL + "/api/v1/ee/stub")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("unlicensed stub route status = %d, want 403", resp2.StatusCode)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != license.CodeRequired {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, license.CodeRequired)
	}
	if envelope.Error.Message == "" {
		t.Fatal("expected a user-facing error message")
	}
}
