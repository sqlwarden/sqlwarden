package credentials

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sqlwarden/internal/engine"
)

// SecretName is a stable connection-secret key.
type SecretName string

const (
	SecretPassword      SecretName = "password"
	SecretSSHPassword   SecretName = "ssh_password"
	SecretSSHPrivateKey SecretName = "ssh_private_key"
	SecretSSHPassphrase SecretName = "ssh_passphrase"
	SecretTLSClientKey  SecretName = "tls_client_key"
)

var secretNames = [...]SecretName{
	SecretPassword,
	SecretSSHPassword,
	SecretSSHPrivateKey,
	SecretSSHPassphrase,
	SecretTLSClientKey,
}

// SecretSource describes where a provider obtains a secret.
type SecretSource string

const (
	SourceStored    SecretSource = "stored"
	SourceReference SecretSource = "reference"
)

// SecretState is safe metadata about a secret. It never contains the value.
type SecretState struct {
	Set    bool         `json:"set"`
	Source SecretSource `json:"source,omitempty"`
}

// ErrNotRevealable reports an unset secret or one managed by a reference
// provider.
var ErrNotRevealable = errors.New("credentials: secret is not revealable")

// ErrUnknownSecret reports a name outside the stable connection-secret set.
var ErrUnknownSecret = errors.New("credentials: unknown secret name")

// Store is the metadata port used by EncryptedColumnProvider. Its aggregate
// methods deliberately use standard-library and primitive types so metadata
// stores can satisfy it without importing this package.
type Store interface {
	LoadConnectionCredentials(ctx context.Context, orgID, workspaceID, connectionID int64) (
		driver, defaultScope string,
		params, tlsConfig, sshConfig json.RawMessage,
		found bool,
		err error,
	)
	ListConnectionSecretValues(ctx context.Context, connectionID int64) (
		sources, ciphertexts map[string]string,
		err error,
	)
	UpsertConnectionSecretValue(ctx context.Context, connectionID int64, name, source, ciphertext, keyID string) error
	DeleteConnectionSecret(ctx context.Context, connectionID int64, name string) error
}

// Sealer seals new values and opens stored ciphertext. Open selects the key
// from the ciphertext envelope; keyID is retained separately for inventory and
// rotation.
type Sealer interface {
	Seal(plaintext string) (ciphertext, keyID string, err error)
	Open(ciphertext string) (string, error)
}

// SpecLookup resolves a driver's connection capability without coupling the
// provider to a registry implementation.
type SpecLookup func(driver string) (engine.ConnectionSpec, bool)

func knownSecretName(name SecretName) bool {
	for _, candidate := range secretNames {
		if name == candidate {
			return true
		}
	}
	return false
}

func emptySecretStates() map[SecretName]SecretState {
	states := make(map[SecretName]SecretState, len(secretNames))
	for _, name := range secretNames {
		states[name] = SecretState{}
	}
	return states
}
