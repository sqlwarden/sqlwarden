package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sqlwarden/internal/engine"
)

// EncryptedColumnProvider resolves structured connection rows and individually
// sealed secret values.
type EncryptedColumnProvider struct {
	store  Store
	sealer Sealer
	specs  SpecLookup
}

// NewEncryptedColumnProvider constructs the built-in structured credentials
// provider.
func NewEncryptedColumnProvider(store Store, sealer Sealer, specs SpecLookup) *EncryptedColumnProvider {
	return &EncryptedColumnProvider{store: store, sealer: sealer, specs: specs}
}

type storedConnection struct {
	id           int64
	driver       string
	defaultScope string
	params       json.RawMessage
	tlsConfig    json.RawMessage
	sshConfig    json.RawMessage
}

func (p *EncryptedColumnProvider) load(ctx context.Context, ref ConnectionRef) (storedConnection, error) {
	orgID, err1 := positiveID(ref.OrgID)
	workspaceID, err2 := positiveID(ref.WorkspaceID)
	connectionID, err3 := positiveID(ref.ConnectionID)
	if err1 != nil || err2 != nil || err3 != nil {
		return storedConnection{}, ErrNotFound
	}

	driver, defaultScope, params, tlsConfig, sshConfig, found, err := p.store.LoadConnectionCredentials(
		ctx, orgID, workspaceID, connectionID,
	)
	if err != nil {
		return storedConnection{}, fmt.Errorf("credentials: load connection %d failed", connectionID)
	}
	if !found {
		return storedConnection{}, ErrNotFound
	}
	return storedConnection{
		id:           connectionID,
		driver:       driver,
		defaultScope: defaultScope,
		params:       params,
		tlsConfig:    tlsConfig,
		sshConfig:    sshConfig,
	}, nil
}

func positiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrNotFound
	}
	return id, nil
}

func (p *EncryptedColumnProvider) loadSecrets(ctx context.Context, connectionID int64) (map[SecretName]string, map[SecretName]SecretSource, error) {
	sources, ciphertexts, err := p.store.ListConnectionSecretValues(ctx, connectionID)
	if err != nil {
		return nil, nil, fmt.Errorf("credentials: load secrets for connection %d failed", connectionID)
	}

	values := make(map[SecretName]string, len(ciphertexts))
	secretSources := make(map[SecretName]SecretSource, len(sources))
	for rawName, rawSource := range sources {
		name := SecretName(rawName)
		if !knownSecretName(name) {
			continue
		}
		source := SecretSource(rawSource)
		secretSources[name] = source
		if source != SourceStored {
			continue
		}
		ciphertext, ok := ciphertexts[rawName]
		if !ok {
			return nil, nil, fmt.Errorf("credentials: decrypt secret %q for connection %d failed", name, connectionID)
		}
		value, err := p.sealer.Open(ciphertext)
		if err != nil {
			return nil, nil, fmt.Errorf("credentials: decrypt secret %q for connection %d failed", name, connectionID)
		}
		values[name] = value
	}
	return values, secretSources, nil
}

// Resolve builds runtime credentials from structured parameters and secrets.
func (p *EncryptedColumnProvider) Resolve(ctx context.Context, ref ConnectionRef) (Credentials, error) {
	conn, err := p.load(ctx, ref)
	if err != nil {
		return Credentials{}, err
	}

	var params engine.Params
	if err := json.Unmarshal(defaultJSONObject(conn.params), &params); err != nil {
		return Credentials{}, fmt.Errorf("credentials: parameters for connection %d are malformed", conn.id)
	}
	values, sources, err := p.loadSecrets(ctx, conn.id)
	if err != nil {
		return Credentials{}, err
	}
	for name, source := range sources {
		if source != SourceStored {
			return Credentials{}, fmt.Errorf("credentials: secret %q for connection %d is not resolvable by this provider", name, conn.id)
		}
	}

	if p.specs == nil {
		return Credentials{}, fmt.Errorf("credentials: connection spec for connection %d is unavailable", conn.id)
	}
	spec, ok := p.specs(conn.driver)
	if !ok || spec == nil {
		return Credentials{}, fmt.Errorf("credentials: connection spec for connection %d is unavailable", conn.id)
	}
	dsn, err := spec.BuildDSN(params, engine.Secrets{string(SecretPassword): values[SecretPassword]})
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: build dsn for connection %d failed", conn.id)
	}

	tls, err := decodeStructuredTLS(conn.id, conn.tlsConfig, values)
	if err != nil {
		return Credentials{}, err
	}
	ssh, err := decodeStructuredSSH(conn.id, conn.sshConfig, values)
	if err != nil {
		return Credentials{}, err
	}

	return Credentials{
		Driver:       conn.driver,
		DefaultScope: conn.defaultScope,
		DSN:          dsn,
		SSH:          ssh,
		TLS:          tls,
	}, nil
}

// Describe returns safe state for every supported secret name.
func (p *EncryptedColumnProvider) Describe(ctx context.Context, ref ConnectionRef) (map[SecretName]SecretState, error) {
	conn, err := p.load(ctx, ref)
	if err != nil {
		return nil, err
	}
	sources, _, err := p.store.ListConnectionSecretValues(ctx, conn.id)
	if err != nil {
		return nil, fmt.Errorf("credentials: load secrets for connection %d failed", conn.id)
	}
	states := emptySecretStates()
	for rawName, rawSource := range sources {
		name := SecretName(rawName)
		if knownSecretName(name) {
			states[name] = SecretState{Set: true, Source: SecretSource(rawSource)}
		}
	}
	return states, nil
}

// Reveal opens one stored value. Reference and unset values are deliberately
// indistinguishable to callers of this low-level port.
func (p *EncryptedColumnProvider) Reveal(ctx context.Context, ref ConnectionRef, name SecretName) (string, error) {
	if !knownSecretName(name) {
		return "", ErrNotRevealable
	}
	conn, err := p.load(ctx, ref)
	if err != nil {
		return "", err
	}
	sources, ciphertexts, err := p.store.ListConnectionSecretValues(ctx, conn.id)
	if err != nil {
		return "", fmt.Errorf("credentials: load secrets for connection %d failed", conn.id)
	}
	if SecretSource(sources[string(name)]) != SourceStored {
		return "", ErrNotRevealable
	}
	ciphertext, ok := ciphertexts[string(name)]
	if !ok {
		return "", fmt.Errorf("credentials: decrypt secret %q for connection %d failed", name, conn.id)
	}
	value, err := p.sealer.Open(ciphertext)
	if err != nil {
		return "", fmt.Errorf("credentials: decrypt secret %q for connection %d failed", name, conn.id)
	}
	return value, nil
}

func defaultJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func decodeStructuredTLS(connectionID int64, raw json.RawMessage, values map[SecretName]string) (*engine.TLSConfig, error) {
	var doc TLSDocument
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("credentials: tls config for connection %d is malformed", connectionID)
		}
	}
	doc.ClientKeyPEM = values[SecretTLSClientKey]
	return doc.ToEngine(), nil
}

func decodeStructuredSSH(connectionID int64, raw json.RawMessage, values map[SecretName]string) (*SSHConfig, error) {
	var doc SSHDocument
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("credentials: ssh config for connection %d is malformed", connectionID)
		}
	}
	doc.Password = values[SecretSSHPassword]
	doc.PrivateKeyPEM = values[SecretSSHPrivateKey]
	doc.Passphrase = values[SecretSSHPassphrase]
	return doc.ToConfig(), nil
}

var _ Provider = (*EncryptedColumnProvider)(nil)
