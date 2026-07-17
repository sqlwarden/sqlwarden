//go:build !enterprise

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultApplicationHasNoOptionalExtensionRoutes(t *testing.T) {
	app := newTestApplication(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/ee/stub")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("optional stub route status = %d, want 404 in default build", resp.StatusCode)
	}
}
