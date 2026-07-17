//go:build enterprise

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnterpriseBinaryMountsStubAndReportsEdition(t *testing.T) {
	app := newTestApplication(t)
	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/instance/edition")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var edition struct {
		Edition string `json:"edition"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&edition); err != nil {
		t.Fatal(err)
	}
	if edition.Edition != "enterprise" {
		t.Fatalf("edition = %q, want enterprise", edition.Edition)
	}

	resp2, err := http.Get(srv.URL + "/api/v1/ee/stub")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("stub route status = %d, want 200", resp2.StatusCode)
	}
	var stub struct {
		Extension    string `json:"extension"`
		StubLicensed bool   `json:"stub_licensed"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&stub); err != nil {
		t.Fatal(err)
	}
	if stub.Extension != "ee" || stub.StubLicensed {
		t.Fatalf("unexpected stub payload: %+v", stub)
	}
}
