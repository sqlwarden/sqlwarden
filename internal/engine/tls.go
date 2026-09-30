package engine

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
)

// TLSMode is the certificate verification mode for a TLS connection.
type TLSMode string

const (
	TLSModeDisable    TLSMode = "disable"
	TLSModeRequire    TLSMode = "require"
	TLSModeVerifyCA   TLSMode = "verify-ca"
	TLSModeVerifyFull TLSMode = "verify-full"
)

// ValidTLSMode reports whether m is one of the four supported modes.
func ValidTLSMode(m TLSMode) bool {
	switch m {
	case TLSModeDisable, TLSModeRequire, TLSModeVerifyCA, TLSModeVerifyFull:
		return true
	default:
		return false
	}
}

// TLSConfig is the decoded, structured TLS material for a connection. It is
// engine-agnostic; each driver only decides how its library receives the
// *tls.Config that Build produces.
type TLSConfig struct {
	Mode          TLSMode
	ServerName    string
	CAPEM         string
	ClientCertPEM string
	ClientKeyPEM  string
}

// Build assembles a *tls.Config from the structured material. It returns
// (nil, nil) when there is no TLS to apply (nil receiver or Mode "disable"),
// and callers must treat a nil result as "no TLS".
func (c *TLSConfig) Build() (*tls.Config, error) {
	if c == nil || c.Mode == "" || c.Mode == TLSModeDisable {
		return nil, nil
	}
	out := &tls.Config{MinVersion: tls.VersionTLS12}

	if c.ServerName != "" {
		out.ServerName = c.ServerName
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if c.CAPEM != "" {
		if !pool.AppendCertsFromPEM([]byte(c.CAPEM)) {
			return nil, errors.New("tls: ca bundle contains no valid PEM certificate")
		}
	}
	out.RootCAs = pool

	switch {
	case c.ClientCertPEM != "" && c.ClientKeyPEM != "":
		pair, err := tls.X509KeyPair([]byte(c.ClientCertPEM), []byte(c.ClientKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("tls: client key pair: %w", err)
		}
		out.Certificates = []tls.Certificate{pair}
	case c.ClientCertPEM != "" || c.ClientKeyPEM != "":
		return nil, errors.New("tls: client certificate and key must be provided together")
	}

	switch c.Mode {
	case TLSModeRequire:
		out.InsecureSkipVerify = true
	case TLSModeVerifyCA:
		out.InsecureSkipVerify = true
		roots := out.RootCAs
		out.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certs := make([]*x509.Certificate, 0, len(rawCerts))
			for _, raw := range rawCerts {
				cert, err := x509.ParseCertificate(raw)
				if err != nil {
					return fmt.Errorf("tls: parse peer certificate: %w", err)
				}
				certs = append(certs, cert)
			}
			if len(certs) == 0 {
				return errors.New("tls: server presented no certificate")
			}
			inter := x509.NewCertPool()
			for _, cert := range certs[1:] {
				inter.AddCert(cert)
			}
			_, err := certs[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter})
			return err
		}
	case TLSModeVerifyFull:
		out.InsecureSkipVerify = false
	}

	return out, nil
}

// TLSCapable is implemented by drivers that accept structured TLS material.
// Resolved by type assertion on an unconnected probe, like
// metadata.SchemaInspector.
type TLSCapable interface {
	TLSSpec() TLSSpec
}

// TLSSpec declares what TLS material an engine accepts. Modes is in UI order.
type TLSSpec struct {
	Modes              []TLSMode `json:"modes"`
	SupportsCABundle   bool      `json:"supports_ca_bundle"`
	SupportsClientCert bool      `json:"supports_client_cert"`
	SupportsServerName bool      `json:"supports_server_name"`
}
