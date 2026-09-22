package web

import (
	"context"
	"net/http"
	"time"

	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/response"
)

// Probe paths are unauthenticated and stable across process kinds so one
// infrastructure probe definition works for every deployment topology.
const (
	healthLivePath  = "/healthz"
	healthReadyPath = "/readyz"
)

// readinessTimeout bounds the dependency checks a readiness probe performs so a
// stalled dependency fails the probe instead of holding the request open until
// the probe client gives up.
const readinessTimeout = 3 * time.Second

// healthResponse is the probe payload. Probes decide on the status code; the
// body exists for operators reading the endpoint by hand.
type healthResponse struct {
	Status       string   `json:"status"`
	ProcessKinds []string `json:"process_kinds"`
	Error        string   `json:"error,omitempty"`
}

func (app *application) healthLive(w http.ResponseWriter, r *http.Request) {
	writeHealth(w, app.config.ProcessKinds, app.health.Live())
}

func (app *application) healthReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()
	writeHealth(w, app.config.ProcessKinds, app.health.Ready(ctx))
}

// healthHandler serves the probe paths on a listener that has no other routes.
// The connector process kind uses it: its execution listener may require
// transport credentials a probe client does not have.
func healthHandler(health *coreapp.Health, processKinds []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthLivePath, func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, processKinds, health.Live())
	})
	mux.HandleFunc("GET "+healthReadyPath, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		writeHealth(w, processKinds, health.Ready(ctx))
	})
	return mux
}

func writeHealth(w http.ResponseWriter, processKinds []string, err error) {
	w.Header().Set("Cache-Control", "no-store")
	payload := healthResponse{Status: "ok", ProcessKinds: processKinds}
	status := http.StatusOK
	if err != nil {
		payload.Status = "unavailable"
		payload.Error = err.Error()
		status = http.StatusServiceUnavailable
	}
	if writeErr := response.JSON(w, status, payload); writeErr != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
