package extension

import (
	"net/http"

	"github.com/sqlwarden/internal/capability"
	"github.com/sqlwarden/internal/response"
)

// WriteCapabilityUnavailable writes the standard refusal for an unavailable
// optional capability. Core calls it before dispatching to extension handlers.
func WriteCapabilityUnavailable(w http.ResponseWriter, name string) error {
	return response.JSON(w, http.StatusForbidden, response.APIErrorEnvelope{
		Error: response.APIError{
			Code:    capability.CodeUnavailable,
			Message: "This capability is not available in this deployment.",
			Details: map[string]any{"capability": name},
		},
	})
}
