package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"golang.org/x/crypto/ssh"
)

type sshConfigDocument = credentials.SSHDocument

func (app *application) sealSSHDocument(d sshConfigDocument) (string, error) {
	return credentials.EncodeSSHDocument(app.keyring, d)
}

func (app *application) decodeSSHDocument(encrypted string) (sshConfigDocument, bool, error) {
	return credentials.DecodeSSHDocument(app.keyring, encrypted)
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
	if doc.IsEmpty() || !doc.Enabled {
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
	switch credentials.SSHAuthMethod(doc.AuthMethod) {
	case credentials.SSHAuthPassword:
		if doc.Password == "" {
			v.AddFieldError("ssh", "SSH password is required for password authentication.")
		}
	case credentials.SSHAuthPrivateKey:
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
