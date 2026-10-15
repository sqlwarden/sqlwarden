package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/engine"
)

// ErrLegacyParse reports a legacy connection row that could not be converted
// to the structured format. Errors wrap only the connection id: the cause is
// dropped because decrypt and parse errors can echo the DSN.
var ErrLegacyParse = errors.New("credentials: legacy connection could not be converted")

// ErrSchemaOutdated reports a metadata schema older than the structured
// connection migration.
var ErrSchemaOutdated = errors.New("credentials: database schema predates structured connections; run migrate first")

// LegacyConnectionsError reports connections still stored in the legacy
// format.
type LegacyConnectionsError struct{ Count int }

func (e *LegacyConnectionsError) Error() string {
	return fmt.Sprintf("%d connections use the legacy format. Run sqlwarden rotate-keys.", e.Count)
}

func legacyParseError(connectionID int64) error {
	return fmt.Errorf("connection %d: %w", connectionID, ErrLegacyParse)
}

// LegacyCounter counts connection rows that still use the legacy columns.
type LegacyCounter interface {
	CountLegacyConnections(ctx context.Context) (int, error)
}

// CountLegacy returns the number of connections stored in the legacy format.
func CountLegacy(ctx context.Context, store LegacyCounter) (int, error) {
	return store.CountLegacyConnections(ctx)
}

// StartupStore is the metadata access the startup check needs.
type StartupStore interface {
	LegacyCounter
	StructuredConnectionSchemaPresent(ctx context.Context) (bool, error)
}

// VerifyStartup fails when the schema predates structured connections or any
// connection still uses the legacy format.
func VerifyStartup(ctx context.Context, store StartupStore) error {
	present, err := store.StructuredConnectionSchemaPresent(ctx)
	if err != nil {
		return fmt.Errorf("credentials: check connection schema: %w", err)
	}
	if !present {
		return ErrSchemaOutdated
	}
	count, err := CountLegacy(ctx, store)
	if err != nil {
		return fmt.Errorf("credentials: count legacy connections: %w", err)
	}
	if count > 0 {
		return &LegacyConnectionsError{Count: count}
	}
	return nil
}

// structuredConnection is the result of splitting one legacy row. Secrets
// hold plaintext until sealed by the caller.
type structuredConnection struct {
	params    json.RawMessage
	tlsConfig json.RawMessage
	sshConfig json.RawMessage
	secrets   map[SecretName]string
}

// splitLegacy converts the decrypted legacy values of one connection. dsn may
// be empty when only the TLS or SSH documents remain.
func splitLegacy(specs SpecLookup, driver, dsn string, tlsDoc TLSDocument, hasTLS bool, sshDoc SSHDocument, hasSSH bool) (structuredConnection, error) {
	out := structuredConnection{secrets: map[SecretName]string{}}

	params := engine.Params{}
	if dsn != "" {
		if specs == nil {
			return out, errors.New("connection spec lookup is unavailable")
		}
		spec, ok := specs(driver)
		if !ok || spec == nil {
			return out, errors.New("connection spec is unavailable")
		}
		parsed, secrets, err := spec.ParseDSN(dsn)
		if err != nil {
			return out, errors.New("dsn is malformed")
		}
		params = parsed
		if password := secrets[string(SecretPassword)]; password != "" {
			out.secrets[SecretPassword] = password
		}
		if native, present := params["tls_mode"]; present {
			delete(params, "tls_mode")
			if !hasTLS || tlsDoc.Mode == "" {
				if mapper, ok := spec.(engine.LegacyTLSMapper); ok {
					tlsDoc.Mode = string(mapper.LegacyTLSMode(native))
					hasTLS = true
				}
			}
		}
	}
	if dsn != "" {
		rawParams, err := json.Marshal(params)
		if err != nil {
			return out, errors.New("parameters could not be encoded")
		}
		out.params = rawParams
	}

	if hasTLS {
		if tlsDoc.ClientKeyPEM != "" {
			out.secrets[SecretTLSClientKey] = tlsDoc.ClientKeyPEM
		}
		tlsDoc.ClientKeyPEM = ""
		if !tlsDoc.IsEmpty() {
			rawTLS, err := json.Marshal(tlsDoc)
			if err != nil {
				return out, errors.New("tls config could not be encoded")
			}
			out.tlsConfig = rawTLS
		}
	}
	if hasSSH {
		for name, value := range map[SecretName]string{
			SecretSSHPassword:   sshDoc.Password,
			SecretSSHPrivateKey: sshDoc.PrivateKeyPEM,
			SecretSSHPassphrase: sshDoc.Passphrase,
		} {
			if value != "" {
				out.secrets[name] = value
			}
		}
		sshDoc.Password, sshDoc.PrivateKeyPEM, sshDoc.Passphrase = "", "", ""
		if !sshDoc.IsEmpty() {
			rawSSH, err := json.Marshal(sshDoc)
			if err != nil {
				return out, errors.New("ssh config could not be encoded")
			}
			out.sshConfig = rawSSH
		}
	}
	return out, nil
}
