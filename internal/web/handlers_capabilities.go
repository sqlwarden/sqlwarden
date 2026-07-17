package web

import (
	"net/http"

	"github.com/sqlwarden/internal/response"
)

// getInstanceCapabilities reports optional capabilities available in this
// deployment. It is public because login-page extensions may need it before
// authentication. It exposes no edition, account, or instance identity data.
func (app *application) getInstanceCapabilities(w http.ResponseWriter, r *http.Request) {
	capabilities := app.capabilityGate.EnabledCapabilities()
	if capabilities == nil {
		capabilities = []string{}
	}
	err := response.JSON(w, http.StatusOK, map[string]any{
		"capabilities": capabilities,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
