//go:build !bindings

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIMiddlewareRoutesOnlyAPIRequestsToApplication(t *testing.T) {
	apiCalls := 0
	assetCalls := 0
	handler := apiMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls++
		w.WriteHeader(http.StatusNoContent)
	}))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		assetCalls++
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/api", "/api/setup/status", "/api/v1/session"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusNoContent)
		}
	}
	for _, path := range []string{"/", "/ide/local", "/assets/app.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}
	if apiCalls != 3 || assetCalls != 3 {
		t.Fatalf("calls = api:%d assets:%d, want 3 each", apiCalls, assetCalls)
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
