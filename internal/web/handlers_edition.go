package web

import (
	"net/http"

	"github.com/sqlwarden/internal/response"
)

// getInstanceEdition reports the binary edition and licensed features for
// frontend capability gating. It is intentionally public: the login page
// needs it before authentication (e.g. to list SSO providers in enterprise
// builds). It exposes no instance-identifying or account data.
func (app *application) getInstanceEdition(w http.ResponseWriter, r *http.Request) {
	features := app.licenseService.LicensedFeatures()
	if features == nil {
		features = []string{}
	}
	err := response.JSON(w, http.StatusOK, map[string]any{
		"edition":           app.licenseService.Edition(),
		"licensed_features": features,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
