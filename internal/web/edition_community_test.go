//go:build !enterprise

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommunityBinaryReportsCommunityEdition(t *testing.T) {
	app := newTestApplication(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/edition")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body struct {
		Edition string `json:"edition"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Edition != "community" {
		t.Fatalf("edition = %q, want community", body.Edition)
	}
}

func TestCommunityBinaryHasNoEnterpriseRoutes(t *testing.T) {
	app := newTestApplication(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/ee/stub")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("enterprise stub route status = %d, want 404 in community build", resp.StatusCode)
	}
}
