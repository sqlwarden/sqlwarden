package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/validator"
)

// tlsConfigDocument is the decrypted plaintext shape of
// connections.tls_config_encrypted. It never appears in a Connection JSON
// response; the private key is never returned by any endpoint.
type tlsConfigDocument struct {
	Mode          string `json:"mode"`
	ServerName    string `json:"server_name,omitempty"`
	CAPEM         string `json:"ca_pem,omitempty"`
	ClientCertPEM string `json:"client_cert_pem,omitempty"`
	ClientKeyPEM  string `json:"client_key_pem,omitempty"`

	// ClearClientKey is a request-only signal on update: drop the stored client
	// key instead of inheriting it when client_key_pem is blank. updateConnection
	// zeroes it before sealing, so it never reaches the encrypted document.
	ClearClientKey bool `json:"clear_client_key,omitempty"`
}

func (d tlsConfigDocument) isEmpty() bool {
	return strings.TrimSpace(d.Mode) == "" &&
		d.ServerName == "" && d.CAPEM == "" && d.ClientCertPEM == "" && d.ClientKeyPEM == ""
}

func (d tlsConfigDocument) toEngine() *engine.TLSConfig {
	if d.isEmpty() {
		return nil
	}
	return &engine.TLSConfig{
		Mode:          engine.TLSMode(d.Mode),
		ServerName:    d.ServerName,
		CAPEM:         d.CAPEM,
		ClientCertPEM: d.ClientCertPEM,
		ClientKeyPEM:  d.ClientKeyPEM,
	}
}

func (app *application) sealTLSDocument(d tlsConfigDocument) (string, error) {
	if d.isEmpty() {
		return "", nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("seal tls config: marshal: %w", err)
	}
	return app.keyring.Encrypt(string(raw))
}

func (app *application) decodeTLSDocument(encrypted string) (tlsConfigDocument, bool, error) {
	if strings.TrimSpace(encrypted) == "" {
		return tlsConfigDocument{}, false, nil
	}
	plain, err := app.keyring.Decrypt(encrypted)
	if err != nil {
		return tlsConfigDocument{}, false, fmt.Errorf("decode tls config: decrypt: %w", err)
	}
	var d tlsConfigDocument
	if err := json.Unmarshal([]byte(plain), &d); err != nil {
		return tlsConfigDocument{}, false, fmt.Errorf("decode tls config: unmarshal: %w", err)
	}
	return d, true, nil
}

func (app *application) openTLSConfig(conn database.Connection) (*engine.TLSConfig, error) {
	doc, has, err := app.decodeTLSDocument(conn.TLSConfigEncrypted)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return doc.toEngine(), nil
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
	if doc.isEmpty() {
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
