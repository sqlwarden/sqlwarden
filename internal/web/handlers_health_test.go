package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
)

func TestHealthEndpointsReportUnbuiltApplication(t *testing.T) {
	handler := healthHandler(coreapp.NewHealth(), []string{config.ProcessKindConnector})

	for _, path := range []string{healthLivePath, healthReadyPath} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
			var payload healthResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != "unavailable" || payload.Error == "" {
				t.Fatalf("payload = %+v", payload)
			}
			if len(payload.ProcessKinds) != 1 || payload.ProcessKinds[0] != config.ProcessKindConnector {
				t.Fatalf("process kinds = %v", payload.ProcessKinds)
			}
		})
	}
}

func TestHealthEndpointsFollowProcessLifecycle(t *testing.T) {
	address := freeLocalAddress(t)
	built := buildRuntimeProcess(t, []string{config.ProcessKindConnector}, address)
	handler := healthHandler(built.Services.Health, built.Services.Config.ProcessKinds)

	if status := probeStatus(t, handler, healthLivePath); status != http.StatusOK {
		t.Fatalf("liveness before start = %d, want %d", status, http.StatusOK)
	}
	if status := probeStatus(t, handler, healthReadyPath); status != http.StatusServiceUnavailable {
		t.Fatalf("readiness before start = %d, want %d", status, http.StatusServiceUnavailable)
	}

	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := probeStatus(t, handler, healthReadyPath); status != http.StatusOK {
		t.Fatalf("readiness while serving = %d, want %d", status, http.StatusOK)
	}

	if err := built.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := probeStatus(t, handler, healthLivePath); status != http.StatusServiceUnavailable {
		t.Fatalf("liveness after shutdown = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if status := probeStatus(t, handler, healthReadyPath); status != http.StatusServiceUnavailable {
		t.Fatalf("readiness after shutdown = %d, want %d", status, http.StatusServiceUnavailable)
	}
}

func TestConnectorServesProbesOnItsOwnListener(t *testing.T) {
	built := buildRuntimeProcess(t, []string{config.ProcessKindConnector}, freeLocalAddress(t))
	if err := built.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := http.Get("http://" + built.Services.Config.Connector.HealthAddress + healthReadyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("connector readiness = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func probeStatus(t *testing.T, handler http.Handler, path string) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Code
}
