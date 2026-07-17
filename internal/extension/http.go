package extension

import (
	"net/http"

	"github.com/sqlwarden/internal/license"
	"github.com/sqlwarden/internal/response"
)

// WriteLicenseRequired writes the standard refusal for a license-gated route.
// Core route composition calls it before dispatching to extension handlers.
func WriteLicenseRequired(w http.ResponseWriter, feature string) error {
	return response.JSON(w, http.StatusForbidden, response.APIErrorEnvelope{
		Error: response.APIError{
			Code:    license.CodeRequired,
			Message: "This feature requires an active SQLWarden Enterprise license.",
			Details: map[string]any{"feature": feature},
		},
	})
}
