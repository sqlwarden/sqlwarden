package response

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsResponseWriterTracksHeaderAndBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := NewMetricsResponseWriter(rec)

	mw.Header().Set("X-Test", "ok")
	mw.WriteHeader(http.StatusCreated)
	if mw.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", mw.StatusCode)
	}

	n, err := mw.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 written bytes, got %d", n)
	}
	if mw.BytesCount != 5 {
		t.Fatalf("expected byte count 5, got %d", mw.BytesCount)
	}
	if rec.Header().Get("X-Test") != "ok" {
		t.Fatalf("expected wrapped header to be set, got %q", rec.Header().Get("X-Test"))
	}
}

func TestMetricsResponseWriterWriteSetsDefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := NewMetricsResponseWriter(rec)

	if _, err := mw.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if mw.StatusCode != http.StatusOK {
		t.Fatalf("expected default status 200, got %d", mw.StatusCode)
	}
	if mw.BytesCount != 2 {
		t.Fatalf("expected byte count 2, got %d", mw.BytesCount)
	}
}

func TestMetricsResponseWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := NewMetricsResponseWriter(rec)

	if mw.Unwrap() != rec {
		t.Fatal("expected Unwrap to return wrapped writer")
	}
}
