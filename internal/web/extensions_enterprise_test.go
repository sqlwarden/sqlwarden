//go:build enterprise

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sqlwarden/enterprise/register"
	"github.com/sqlwarden/internal/capability"
)

// An unavailable optional capability remains inaccessible even when its
// implementation is compiled into the distribution.
func TestEnterpriseCompositionRejectsUnavailableCapability(t *testing.T) {
	app := newTestApplicationWithExtensions(t, register.Registry())
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var state struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if len(state.Capabilities) != 0 {
		t.Fatalf("capabilities = %v, want empty", state.Capabilities)
	}

	_, token := seedInstanceAdminAccount(t, app, uniqueEmail(t, "edition-admin"), "Edition Admin")
	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/ee/stub", nil, token), app.routes())
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("unavailable stub route status = %d, want 403", res.StatusCode)
	}
	errorBody, _ := res.BodyFields["error"].(map[string]any)
	if code, _ := errorBody["code"].(string); code != capability.CodeUnavailable {
		t.Fatalf("error code = %q, want %q", code, capability.CodeUnavailable)
	}
	if message, _ := errorBody["message"].(string); message == "" {
		t.Fatal("expected a user-facing error message")
	}
}
