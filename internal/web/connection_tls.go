package web

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/validator"
)

type tlsConfigDocument = credentials.TLSDocument

func (app *application) sealTLSDocument(d tlsConfigDocument) (string, error) {
	return credentials.EncodeTLSDocument(app.keyring, d)
}

func (app *application) decodeTLSDocument(encrypted string) (tlsConfigDocument, bool, error) {
	return credentials.DecodeTLSDocument(app.keyring, encrypted)
}

// tlsSpecForDriver returns the engine's TLS spec, or false when the engine
// declares no TLS support (SQLite / non-network engines).
func tlsSpecForDriver(driver string) (engine.TLSSpec, bool) {
	d, err := engine.New(driver)
	if err != nil {
		return engine.TLSSpec{}, false
	}
	tc, ok := d.(engine.TLSCapable)
	if !ok {
		return engine.TLSSpec{}, false
	}
	return tc.TLSSpec(), true
}

func (app *application) validateTLSDocument(driver string, doc tlsConfigDocument, v *validator.Validator) {
	if doc.IsEmpty() {
		return
	}
	spec, ok := tlsSpecForDriver(driver)
	if !ok {
		v.AddFieldError("tls", "This driver does not support TLS configuration.")
		return
	}
	mode := engine.TLSMode(doc.Mode)
	if !engine.ValidTLSMode(mode) || !slices.Contains(spec.Modes, mode) {
		v.AddFieldError("tls", "Unsupported TLS verification mode.")
	}
	if doc.ClientCertPEM == "" && doc.ClientKeyPEM != "" {
		v.AddFieldError("tls", "A client key requires a client certificate.")
	}
}

// deleteConnectionTLS removes the stored TLS configuration outright, so an
// operator can prune the encrypted document rather than only setting mode to
// disable. Gated by conn:update like the reveal and patch routes.
func (app *application) deleteConnectionTLS(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	if err := app.db.UpdateConnectionTLSConfig(r.Context(), conn.ID, ""); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "connection tls configuration removed", slog.Int64("connection_id", conn.ID))
	w.WriteHeader(http.StatusNoContent)
}
