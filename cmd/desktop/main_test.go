//go:build !bindings

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopRoutingMiddlewareRoutesAPIAndSPAFallback(t *testing.T) {
	apiCalls := 0
	var assetPaths []string
	handler := desktopRoutingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls++
		w.WriteHeader(http.StatusNoContent)
	}))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assetPaths = append(assetPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/api", "/api/setup/status", "/api/v1/session"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusNoContent)
		}
	}
	assets := []struct {
		path   string
		accept string
	}{
		{path: "/", accept: "text/html"},
		{path: "/ide/local", accept: "text/html,application/xhtml+xml"},
		{path: "/assets/app.js", accept: "*/*"},
	}
	for _, tc := range assets {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		request.Header.Set("Accept", tc.accept)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", tc.path, response.Code, http.StatusOK)
		}
	}
	if apiCalls != 3 {
		t.Fatalf("api calls = %d, want 3", apiCalls)
	}
	wantAssetPaths := []string{"/", "/", "/assets/app.js"}
	if len(assetPaths) != len(wantAssetPaths) {
		t.Fatalf("asset paths = %v, want %v", assetPaths, wantAssetPaths)
	}
	for i := range assetPaths {
		if assetPaths[i] != wantAssetPaths[i] {
			t.Fatalf("asset paths = %v, want %v", assetPaths, wantAssetPaths)
		}
	}
}

func TestDesktopRoutingMiddlewareDoesNotMaskMissingAssets(t *testing.T) {
	var gotPath string
	handler := desktopRoutingMiddleware(http.NotFoundHandler())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))

	request := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || gotPath != "/assets/missing.js" {
		t.Fatalf("status/path = %d %q, want %d %q", response.Code, gotPath, http.StatusNotFound, "/assets/missing.js")
	}
}

func TestUnavailableAPIReturnsStructuredStartupFailure(t *testing.T) {
	response := httptest.NewRecorder()
	unavailableAPI(errors.New("database is locked")).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if got := response.Body.String(); got != "{\"error\":{\"code\":\"desktop_startup_failed\",\"message\":\"database is locked\"}}\n" {
		t.Fatalf("body = %q", got)
	}
}
