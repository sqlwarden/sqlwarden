package web

import (
	"context"
	"net/http"

	"github.com/sqlwarden/internal/keyrotation"
	"github.com/sqlwarden/internal/response"
)

// EncryptionRotationReport summarizes a key-rotation pass over all
// application-encrypted data.
type EncryptionRotationReport = keyrotation.Report

// rotateEncryptionKeysHandler re-encrypts all application-encrypted data with
// the current primary key and returns a summary of what was rotated.
func (app *application) rotateEncryptionKeysHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	report, err := app.services.KeyRotation.Rotate(r.Context(), principal, keyrotation.Options{})
	if err != nil {
		app.serviceError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, report); err != nil {
		app.serverError(w, r, err)
	}
}

// RotateEncryptionKeys runs key rotation for the rotate-keys command.
func (app *application) RotateEncryptionKeys(ctx context.Context) (EncryptionRotationReport, error) {
	return app.services.KeyRotation.Rotate(ctx, keyrotation.OperatorPrincipal(), keyrotation.Options{})
}
