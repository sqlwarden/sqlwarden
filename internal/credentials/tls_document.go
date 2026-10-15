package credentials

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

// TLSDocument is the plaintext shape of connections.tls_config_encrypted. It
// never appears in a connection JSON response; the private key is never
// returned by any endpoint.
type TLSDocument struct {
	Mode          string `json:"mode"`
	ServerName    string `json:"server_name,omitempty"`
	CAPEM         string `json:"ca_pem,omitempty"`
	ClientCertPEM string `json:"client_cert_pem,omitempty"`
	ClientKeyPEM  string `json:"client_key_pem,omitempty"`
}

// IsEmpty reports whether the document carries no TLS configuration at all.
func (d TLSDocument) IsEmpty() bool {
	return strings.TrimSpace(d.Mode) == "" &&
		d.ServerName == "" && d.CAPEM == "" && d.ClientCertPEM == "" && d.ClientKeyPEM == ""
}

// ToEngine returns the engine TLS config, or nil for an empty document.
func (d TLSDocument) ToEngine() *engine.TLSConfig {
	if d.IsEmpty() {
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

// EncodeTLSDocument seals the document. An empty document seals to "".
func EncodeTLSDocument(enc Encrypter, d TLSDocument) (string, error) {
	if d.IsEmpty() {
		return "", nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("seal tls config: marshal: %w", err)
	}
	return enc.Encrypt(string(raw))
}

// DecodeTLSDocument opens the stored document; has is false when the column is
// blank. Underlying decrypt and unmarshal causes are dropped from the returned
// error because a Decrypter may echo the value it failed on.
func DecodeTLSDocument(dec Decrypter, encrypted string) (doc TLSDocument, has bool, err error) {
	if strings.TrimSpace(encrypted) == "" {
		return TLSDocument{}, false, nil
	}
	plain, err := dec.Decrypt(encrypted)
	if err != nil {
		return TLSDocument{}, false, fmt.Errorf("decode tls config: decrypt failed")
	}
	if err := json.Unmarshal([]byte(plain), &doc); err != nil {
		return TLSDocument{}, false, fmt.Errorf("decode tls config: document is malformed")
	}
	return doc, true, nil
}
