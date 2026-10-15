package web

import (
	"net/http"
	"testing"
)

func TestCapabilitiesAreAvailableToAuthenticatedRegularAccounts(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	registered := registerTestUser(t, app, "member@example.com", "Member", "securepass99")
	if registered.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d: %s", registered.StatusCode, registered.BodyBytes)
	}
	loggedIn := loginTestUser(t, app, "member@example.com", "securepass99")
	if loggedIn.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d: %s", loggedIn.StatusCode, loggedIn.BodyBytes)
	}
	token := extractAccessToken(t, loggedIn)
	app.edition = EditionCapabilities{Name: "community", Features: []EditionFeature{{
		Key: "audit.tamper_evidence", Label: "Tamper-evident audit", Description: "Evidence", State: "upgrade",
	}}}

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/capabilities", nil, token), app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", res.StatusCode, res.BodyBytes)
	}
	if res.BodyFields["edition"] != "community" {
		t.Fatalf("edition = %#v", res.BodyFields["edition"])
	}
	features, ok := res.BodyFields["features"].([]any)
	if !ok || len(features) != 1 {
		t.Fatalf("features = %#v", res.BodyFields["features"])
	}
	feature := features[0].(map[string]any)
	if feature["state"] != "upgrade" || feature["docs_url"] != nil || feature["navigation"] != nil {
		t.Fatalf("feature = %#v", feature)
	}
}

func TestCapabilitiesRejectAnonymousRequests(t *testing.T) {
	app := newTestApp(t)
	res := send(t, newTestRequest(t, http.MethodGet, "/api/v1/instance/capabilities", nil), app.routes())
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnauthorized)
	}
}
