package credentials_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/credentials/credentialstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
)

type connectionStore map[int64]database.Connection

func (s connectionStore) GetConnection(_ context.Context, id int64) (database.Connection, bool, error) {
	connection, ok := s[id]
	return connection, ok, nil
}

func TestEncryptedColumnProviderContract(t *testing.T) {
	keyring, err := encrypt.NewKeyring("provider-contract-key")
	if err != nil {
		t.Fatal(err)
	}
	dsn := "postgres://contract-user:contract-password@database/contract"
	tlsJSON := `{"mode":"verify-full","server_name":"database","client_key_pem":"tls-private-marker"}`
	sshJSON := `{"enabled":true,"host":"bastion","port":22,"user":"tunnel-user","auth_method":"password","password":"ssh-password-marker","fingerprint":"SHA256:contract"}`
	encryptedDSN := mustEncrypt(t, keyring, dsn)
	encryptedTLS := mustEncrypt(t, keyring, tlsJSON)
	encryptedSSH := mustEncrypt(t, keyring, sshJSON)
	brokenCiphertext := "broken-ciphertext-marker"

	provider := credentials.NewEncryptedColumnProvider(connectionStore{
		41: {
			ID: 41, Driver: "postgres", DSNEncrypted: encryptedDSN,
			TLSConfigEncrypted: encryptedTLS, SSHConfigEncrypted: encryptedSSH,
			DefaultScope: metadata.ScopePath("contract"),
		},
		42: {ID: 42, Driver: "postgres", DSNEncrypted: brokenCiphertext},
	}, keyring)

	credentialstest.RunProviderContract(t, credentialstest.Contract{
		Provider: provider, ExistingID: "41", MissingID: "404", BrokenID: "42",
		Expected: execution.Credentials{
			Driver: "postgres", DSN: dsn, DefaultScope: metadata.ScopePath("contract"),
			TLS: &engine.TLSConfig{
				Mode: engine.TLSModeVerifyFull, ServerName: "database", ClientKeyPEM: "tls-private-marker",
			},
			SSH: &execution.SSHConfig{
				Host: "bastion", Port: 22, User: "tunnel-user", AuthMethod: "password",
				Password: "ssh-password-marker", Fingerprint: "SHA256:contract",
			},
		},
		SecretMarkers: []string{dsn, "contract-password", "tls-private-marker", "ssh-password-marker", encryptedDSN, encryptedTLS, encryptedSSH, brokenCiphertext},
	})
}

func TestEncryptedColumnProviderRejectsMalformedStructuredCredentialsWithoutLeak(t *testing.T) {
	keyring, err := encrypt.NewKeyring("malformed-provider-key")
	if err != nil {
		t.Fatal(err)
	}
	malformed := `{"client_key_pem":"malformed-secret-marker"`
	provider := credentials.NewEncryptedColumnProvider(connectionStore{7: {
		ID: 7, Driver: "postgres", DSNEncrypted: mustEncrypt(t, keyring, "dsn"),
		TLSConfigEncrypted: mustEncrypt(t, keyring, malformed),
	}}, keyring)

	_, err = provider.Resolve(context.Background(), "7")
	if !errors.Is(err, execution.ErrCredentialsInvalid) {
		t.Fatalf("Resolve() error = %v, want invalid credentials", err)
	}
	if got := err.Error(); got == "" || strings.Contains(got, "malformed-secret-marker") {
		t.Fatalf("Resolve() error leaked structured credentials: %q", got)
	}
}

func TestEncryptedColumnProviderRejectsNonPositiveOrMalformedIDsAsNotFound(t *testing.T) {
	keyring, err := encrypt.NewKeyring("invalid-id-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := credentials.NewEncryptedColumnProvider(connectionStore{
		0: {ID: 0, Driver: "postgres", DSNEncrypted: mustEncrypt(t, keyring, "zero-id-secret-marker")},
	}, keyring)

	for _, id := range []string{"", "abc", "12x", "id-secret-marker", "0", "-1", "-9223372036854775808", "99999999999999999999"} {
		t.Run(id, func(t *testing.T) {
			_, err := provider.Resolve(context.Background(), id)
			if !errors.Is(err, execution.ErrCredentialsNotFound) {
				t.Fatalf("Resolve(%q) error = %v, want credentials not found", id, err)
			}
			if got := err.Error(); got != execution.ErrCredentialsNotFound.Error() {
				t.Fatalf("Resolve(%q) error = %q, want sentinel message only", id, got)
			}
		})
	}
}

func TestEncryptedColumnProviderTreatsWhitespaceTLSModeAsEmpty(t *testing.T) {
	keyring, err := encrypt.NewKeyring("whitespace-tls-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := credentials.NewEncryptedColumnProvider(connectionStore{8: {
		ID: 8, Driver: "postgres", DSNEncrypted: mustEncrypt(t, keyring, "dsn"),
		TLSConfigEncrypted: mustEncrypt(t, keyring, `{"mode":"   "}`),
	}}, keyring)

	resolved, err := provider.Resolve(context.Background(), "8")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.TLS != nil {
		t.Fatalf("Resolve() TLS = %#v, want nil", resolved.TLS)
	}
}

func mustEncrypt(t testing.TB, keyring *encrypt.Keyring, plaintext string) string {
	t.Helper()
	ciphertext, err := keyring.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return ciphertext
}
