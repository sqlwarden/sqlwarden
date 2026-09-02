package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"golang.org/x/crypto/ssh"
)

// sshConfigDocument is the decrypted plaintext shape of
// connections.ssh_config_encrypted. Password, private key, and passphrase never
// appear in a Connection JSON response or the reveal endpoint.
type sshConfigDocument struct {
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host,omitempty"`
	Port                int    `json:"port,omitempty"`
	User                string `json:"user,omitempty"`
	AuthMethod          string `json:"auth_method,omitempty"`
	Password            string `json:"password,omitempty"`
	PrivateKeyPEM       string `json:"private_key_pem,omitempty"`
	Passphrase          string `json:"passphrase,omitempty"`
	KnownHostsEntry     string `json:"known_hosts_entry,omitempty"`
	Fingerprint         string `json:"fingerprint,omitempty"`
	InsecureSkipHostKey bool   `json:"insecure_skip_host_key,omitempty"`

	// Clear* are request-only signals on update: drop the matching stored secret
	// instead of inheriting it when the incoming field is blank. updateConnection
	// zeroes them before sealing, so they never reach the encrypted document.
	ClearPassword   bool `json:"clear_password,omitempty"`
	ClearPrivateKey bool `json:"clear_private_key,omitempty"`
	ClearPassphrase bool `json:"clear_passphrase,omitempty"`
}

func (d sshConfigDocument) isEmpty() bool {
	return !d.Enabled &&
		d.Host == "" && d.User == "" && d.AuthMethod == "" &&
		d.Password == "" && d.PrivateKeyPEM == "" && d.Passphrase == "" &&
		d.KnownHostsEntry == "" && d.Fingerprint == "" && !d.InsecureSkipHostKey
}

func (d sshConfigDocument) toConnection() *connection.SSHConfig {
	if !d.Enabled {
		return nil
	}
	return &connection.SSHConfig{
		Host:                d.Host,
		Port:                d.Port,
		User:                d.User,
		AuthMethod:          connection.SSHAuthMethod(d.AuthMethod),
		Password:            d.Password,
		PrivateKeyPEM:       d.PrivateKeyPEM,
		Passphrase:          d.Passphrase,
		KnownHostsEntry:     d.KnownHostsEntry,
		Fingerprint:         d.Fingerprint,
		InsecureSkipHostKey: d.InsecureSkipHostKey,
	}
}

func (app *application) sealSSHDocument(d sshConfigDocument) (string, error) {
	if d.isEmpty() {
		return "", nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("seal ssh config: marshal: %w", err)
	}
	return app.keyring.Encrypt(string(raw))
}

func (app *application) decodeSSHDocument(encrypted string) (sshConfigDocument, bool, error) {
	if strings.TrimSpace(encrypted) == "" {
		return sshConfigDocument{}, false, nil
	}
	plain, err := app.keyring.Decrypt(encrypted)
	if err != nil {
		return sshConfigDocument{}, false, fmt.Errorf("decode ssh config: decrypt: %w", err)
	}
	var d sshConfigDocument
	if err := json.Unmarshal([]byte(plain), &d); err != nil {
		return sshConfigDocument{}, false, fmt.Errorf("decode ssh config: unmarshal: %w", err)
	}
	return d, true, nil
}

func (app *application) openSSHConfig(conn database.Connection) (*connection.SSHConfig, error) {
	doc, has, err := app.decodeSSHDocument(conn.SSHConfigEncrypted)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return doc.toConnection(), nil
}

// sshTunnelSupported reports whether the engine can route its transport through
// a caller-supplied dialer (network engines yes, SQLite no).
func sshTunnelSupported(driver string) bool {
	d, err := engine.New(driver)
	if err != nil {
		return false
	}
	sc, ok := d.(engine.SSHTunnelCapable)
	return ok && sc.SupportsSSHTunnel()
}

func (app *application) validateSSHDocument(driver string, doc sshConfigDocument, v *validator.Validator) {
	if doc.isEmpty() || !doc.Enabled {
		return
	}
	if !sshTunnelSupported(driver) {
		v.AddFieldError("ssh", "This driver does not support SSH tunneling.")
		return
	}
	if strings.TrimSpace(doc.Host) == "" {
		v.AddFieldError("ssh", "SSH host is required.")
	}
	if strings.TrimSpace(doc.User) == "" {
		v.AddFieldError("ssh", "SSH user is required.")
	}
	if doc.Port != 0 && (doc.Port < 1 || doc.Port > 65535) {
		v.AddFieldError("ssh", "SSH port must be between 1 and 65535.")
	}
	switch connection.SSHAuthMethod(doc.AuthMethod) {
	case connection.SSHAuthPassword:
		if doc.Password == "" {
			v.AddFieldError("ssh", "SSH password is required for password authentication.")
		}
	case connection.SSHAuthPrivateKey:
		if doc.PrivateKeyPEM == "" {
			v.AddFieldError("ssh", "SSH private key is required for key authentication.")
		} else if _, err := parseSSHPrivateKey(doc.PrivateKeyPEM, doc.Passphrase); err != nil {
			v.AddFieldError("ssh", "SSH private key could not be parsed (check the passphrase).")
		}
	default:
		v.AddFieldError("ssh", "SSH auth method must be password or private_key.")
	}
	if !doc.InsecureSkipHostKey {
		switch {
		case doc.KnownHostsEntry != "":
			if _, _, _, _, _, err := ssh.ParseKnownHosts([]byte(doc.KnownHostsEntry)); err != nil {
				v.AddFieldError("ssh", "known_hosts entry could not be parsed.")
			}
		case doc.Fingerprint != "":
			if !strings.HasPrefix(doc.Fingerprint, "SHA256:") {
				v.AddFieldError("ssh", "Host key fingerprint must be in SHA256:... form.")
			}
		default:
			v.AddFieldError("ssh", "Provide a known_hosts entry or SHA256 fingerprint, or explicitly disable host key verification.")
		}
	}
}

func parseSSHPrivateKey(pemStr, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase([]byte(pemStr), []byte(passphrase))
	}
	return ssh.ParsePrivateKey([]byte(pemStr))
}

// getConnectionSSH reveals the stored SSH tunnel config, minus every secret, so
// the edit form can pre-fill it. Gated by conn:update like getConnectionDSN.
func (app *application) getConnectionSSH(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	if org.MaskConnectionCredentialsOnEdit {
		app.notPermitted(w, r)
		return
	}

	conn := contextGetConnection(r)
	doc, has, err := app.decodeSSHDocument(conn.SSHConfigEncrypted)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	port := doc.Port
	if port == 0 {
		port = 22
	}
	app.logInfo(r, "connection ssh revealed", slog.Int64("connection_id", conn.ID))
	if err := response.JSON(w, http.StatusOK, map[string]any{
		"configured":             has,
		"enabled":                has && doc.Enabled,
		"host":                   doc.Host,
		"port":                   port,
		"user":                   doc.User,
		"auth_method":            doc.AuthMethod,
		"known_hosts_entry":      doc.KnownHostsEntry,
		"fingerprint":            doc.Fingerprint,
		"insecure_skip_host_key": doc.InsecureSkipHostKey,
		"password_set":           doc.Password != "",
		"private_key_set":        doc.PrivateKeyPEM != "",
	}); err != nil {
		app.serverError(w, r, err)
	}
}

// deleteConnectionSSH removes the stored SSH tunnel configuration outright, so an
// operator can prune the encrypted document rather than only disabling it. Gated
// by conn:update like the reveal and patch routes.
func (app *application) deleteConnectionSSH(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	if err := app.db.UpdateConnectionSSHConfig(r.Context(), conn.ID, ""); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "connection ssh configuration removed", slog.Int64("connection_id", conn.ID))
	w.WriteHeader(http.StatusNoContent)
}
