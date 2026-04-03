package response

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSONAndJSONWithHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := JSON(rec, http.StatusCreated, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type: %s", rec.Header().Get("Content-Type"))
	}

	rec = httptest.NewRecorder()
	headers := http.Header{}
	headers.Add("X-Test", "one")
	headers.Add("X-Test", "two")
	if err := JSONWithHeaders(rec, http.StatusOK, map[string]any{"name": "sqlwarden"}, headers); err != nil {
		t.Fatal(err)
	}
	values := rec.Header().Values("X-Test")
	if len(values) != 2 {
		t.Fatalf("expected 2 X-Test values, got %v", values)
	}
}
