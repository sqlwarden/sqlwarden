package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/execution"
)

// EncryptedColumnProvider resolves the encrypted credential columns stored in
// the SQLWarden metadata database.
type EncryptedColumnProvider struct {
	store   ConnectionStore
	keyring *encrypt.Keyring
}

// ConnectionStore is the metadata lookup required by the encrypted-column
// adapter.
type ConnectionStore interface {
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
}

// NewEncryptedColumnProvider returns the core credential provider.
func NewEncryptedColumnProvider(store ConnectionStore, keyring *encrypt.Keyring) *EncryptedColumnProvider {
	return &EncryptedColumnProvider{store: store, keyring: keyring}
}

// Resolve loads and decrypts one connection without exposing secret material
// in any returned error.
func (p *EncryptedColumnProvider) Resolve(ctx context.Context, connectionID string) (execution.Credentials, error) {
	id, err := strconv.ParseInt(connectionID, 10, 64)
	if err != nil || id <= 0 {
		return execution.Credentials{}, execution.ErrCredentialsNotFound
	}
	conn, found, err := p.store.GetConnection(ctx, id)
	if err != nil {
		return execution.Credentials{}, fmt.Errorf("resolve connection credentials: %w", err)
	}
	if !found {
		return execution.Credentials{}, execution.ErrCredentialsNotFound
	}

	dsn, err := p.decrypt(conn.DSNEncrypted, "dsn")
	if err != nil {
		return execution.Credentials{}, err
	}
	tlsConfig, err := p.resolveTLS(conn.TLSConfigEncrypted)
	if err != nil {
		return execution.Credentials{}, err
	}
	sshConfig, err := p.resolveSSH(conn.SSHConfigEncrypted)
	if err != nil {
		return execution.Credentials{}, err
	}
	return execution.Credentials{
		Driver: conn.Driver, DSN: dsn, DefaultScope: conn.DefaultScope,
		TLS: tlsConfig, SSH: sshConfig,
	}, nil
}

func (p *EncryptedColumnProvider) decrypt(ciphertext, field string) (string, error) {
	plaintext, err := p.keyring.Decrypt(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: %s", execution.ErrCredentialDecryption, field)
	}
	return plaintext, nil
}

type tlsDocument struct {
	Mode          string `json:"mode"`
	ServerName    string `json:"server_name,omitempty"`
	CAPEM         string `json:"ca_pem,omitempty"`
	ClientCertPEM string `json:"client_cert_pem,omitempty"`
	ClientKeyPEM  string `json:"client_key_pem,omitempty"`
}

func (p *EncryptedColumnProvider) resolveTLS(ciphertext string) (*engine.TLSConfig, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return nil, nil
	}
	plaintext, err := p.decrypt(ciphertext, "tls")
	if err != nil {
		return nil, err
	}
	var document tlsDocument
	if err := json.Unmarshal([]byte(plaintext), &document); err != nil {
		return nil, fmt.Errorf("%w: tls", execution.ErrCredentialsInvalid)
	}
	if strings.TrimSpace(document.Mode) == "" && document.ServerName == "" && document.CAPEM == "" && document.ClientCertPEM == "" && document.ClientKeyPEM == "" {
		return nil, nil
	}
	return &engine.TLSConfig{
		Mode: engine.TLSMode(document.Mode), ServerName: document.ServerName,
		CAPEM: document.CAPEM, ClientCertPEM: document.ClientCertPEM, ClientKeyPEM: document.ClientKeyPEM,
	}, nil
}

type sshDocument struct {
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
}

func (p *EncryptedColumnProvider) resolveSSH(ciphertext string) (*execution.SSHConfig, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return nil, nil
	}
	plaintext, err := p.decrypt(ciphertext, "ssh")
	if err != nil {
		return nil, err
	}
	var document sshDocument
	if err := json.Unmarshal([]byte(plaintext), &document); err != nil {
		return nil, fmt.Errorf("%w: ssh", execution.ErrCredentialsInvalid)
	}
	if !document.Enabled {
		return nil, nil
	}
	return &execution.SSHConfig{
		Host: document.Host, Port: document.Port, User: document.User,
		AuthMethod: execution.SSHAuthMethod(document.AuthMethod), Password: document.Password,
		PrivateKeyPEM: document.PrivateKeyPEM, Passphrase: document.Passphrase,
		KnownHostsEntry: document.KnownHostsEntry, Fingerprint: document.Fingerprint,
		InsecureSkipHostKey: document.InsecureSkipHostKey,
	}, nil
}

var _ execution.CredentialProvider = (*EncryptedColumnProvider)(nil)
