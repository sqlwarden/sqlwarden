package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetInstanceCapabilitiesIsPublic(t *testing.T) {
	app := newTestApplication(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Capabilities == nil {
		t.Fatal("capabilities must be [] not null")
	}
	if len(body.Capabilities) != 0 {
		t.Fatalf("capabilities = %v, want empty", body.Capabilities)
	}
}
