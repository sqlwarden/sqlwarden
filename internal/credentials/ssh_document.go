package credentials

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Encrypter seals values with the application keyring.
type Encrypter interface {
	Encrypt(plaintext string) (string, error)
}

// SSHDocument is the plaintext shape of connections.ssh_config_encrypted. It is
// the single definition shared by sealing and decoding so their JSON tags
// cannot drift.
type SSHDocument struct {
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
	// instead of inheriting it when the incoming field is blank. They are zeroed
	// before sealing, so they never reach the encrypted document.
	ClearPassword   bool `json:"clear_password,omitempty"`
	ClearPrivateKey bool `json:"clear_private_key,omitempty"`
	ClearPassphrase bool `json:"clear_passphrase,omitempty"`
}

// IsEmpty reports whether the document carries no SSH configuration at all.
func (d SSHDocument) IsEmpty() bool {
	return !d.Enabled &&
		d.Host == "" && d.User == "" && d.AuthMethod == "" &&
		d.Password == "" && d.PrivateKeyPEM == "" && d.Passphrase == "" &&
		d.KnownHostsEntry == "" && d.Fingerprint == "" && !d.InsecureSkipHostKey
}

// ToConfig returns the tunnel config, or nil when the tunnel is disabled.
func (d SSHDocument) ToConfig() *SSHConfig {
	if !d.Enabled {
		return nil
	}
	return &SSHConfig{
		Host:                d.Host,
		Port:                d.Port,
		User:                d.User,
		AuthMethod:          SSHAuthMethod(d.AuthMethod),
		Password:            d.Password,
		PrivateKeyPEM:       d.PrivateKeyPEM,
		Passphrase:          d.Passphrase,
		KnownHostsEntry:     d.KnownHostsEntry,
		Fingerprint:         d.Fingerprint,
		InsecureSkipHostKey: d.InsecureSkipHostKey,
	}
}

// EncodeSSHDocument seals the document. An empty document seals to "".
func EncodeSSHDocument(enc Encrypter, d SSHDocument) (string, error) {
	if d.IsEmpty() {
		return "", nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("seal ssh config: marshal: %w", err)
	}
	return enc.Encrypt(string(raw))
}

// DecodeSSHDocument opens the stored document; has is false when the column is
// blank. Underlying decrypt and unmarshal causes are dropped from the returned
// error because a Decrypter may echo the value it failed on.
func DecodeSSHDocument(dec Decrypter, encrypted string) (doc SSHDocument, has bool, err error) {
	if strings.TrimSpace(encrypted) == "" {
		return SSHDocument{}, false, nil
	}
	plain, err := dec.Decrypt(encrypted)
	if err != nil {
		return SSHDocument{}, false, fmt.Errorf("decode ssh config: decrypt failed")
	}
	if err := json.Unmarshal([]byte(plain), &doc); err != nil {
		return SSHDocument{}, false, fmt.Errorf("decode ssh config: document is malformed")
	}
	return doc, true, nil
}

// DecodeSSHConfig opens the stored document as a tunnel config, nil when the
// column is blank or the tunnel is disabled.
func DecodeSSHConfig(dec Decrypter, encrypted string) (*SSHConfig, error) {
	doc, has, err := DecodeSSHDocument(dec, encrypted)
	if err != nil || !has {
		return nil, err
	}
	return doc.ToConfig(), nil
}
