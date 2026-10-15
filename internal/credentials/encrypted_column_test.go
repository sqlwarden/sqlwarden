package credentials_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/credentials/credentialstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
)

const (
	columnOrgID        = int64(17)
	columnWorkspaceID  = int64(19)
	columnConnectionID = int64(31)
	columnSSHID        = int64(32)
	columnBadID        = int64(33)
	columnReferenceID  = int64(34)
	columnPassword     = "column-password-do-not-leak"
	columnSSHPassword  = "column-ssh-password-do-not-leak"
	columnSSHKey       = "column-ssh-key-do-not-leak"
	columnSSHPhrase    = "column-ssh-passphrase-do-not-leak"
	columnTLSKey       = "column-tls-key-do-not-leak"
	columnReference    = "vault/path/to/password"
)

type columnRecord struct {
	orgID        int64
	workspaceID  int64
	driver       string
	defaultScope string
	params       json.RawMessage
	tlsConfig    json.RawMessage
	sshConfig    json.RawMessage
}

type secretRecord struct {
	source     string
	ciphertext string
	keyID      string
}

type memoryColumnStore struct {
	connections    map[int64]columnRecord
	secrets        map[int64]map[string]secretRecord
	loadErr        error
	secretErr      error
	omitCiphertext bool
}

func (s *memoryColumnStore) LoadConnectionCredentials(_ context.Context, orgID, workspaceID, connectionID int64) (
	driver, defaultScope string,
	params, tlsConfig, sshConfig json.RawMessage,
	found bool,
	err error,
) {
	if s.loadErr != nil {
		return "", "", nil, nil, nil, false, s.loadErr
	}
	record, ok := s.connections[connectionID]
	if !ok || record.orgID != orgID || record.workspaceID != workspaceID {
		return "", "", nil, nil, nil, false, nil
	}
	return record.driver, record.defaultScope, record.params, record.tlsConfig, record.sshConfig, true, nil
}

func (s *memoryColumnStore) ListConnectionSecretValues(_ context.Context, connectionID int64) (
	sources, ciphertexts map[string]string,
	err error,
) {
	if s.secretErr != nil {
		return nil, nil, s.secretErr
	}
	sources = map[string]string{}
	ciphertexts = map[string]string{}
	for name, secret := range s.secrets[connectionID] {
		sources[name] = secret.source
		if !s.omitCiphertext {
			ciphertexts[name] = secret.ciphertext
		}
	}
	return sources, ciphertexts, nil
}

func (s *memoryColumnStore) UpsertConnectionSecretValue(_ context.Context, connectionID int64, name, source, ciphertext, keyID string) error {
	if s.secrets[connectionID] == nil {
		s.secrets[connectionID] = map[string]secretRecord{}
	}
	s.secrets[connectionID][name] = secretRecord{source: source, ciphertext: ciphertext, keyID: keyID}
	return nil
}

func (s *memoryColumnStore) DeleteConnectionSecret(_ context.Context, connectionID int64, name string) error {
	delete(s.secrets[connectionID], name)
	return nil
}

type memorySealer struct {
	values map[string]string
	next   int
}

func (s *memorySealer) Seal(plaintext string) (string, string, error) {
	s.next++
	ciphertext := fmt.Sprintf("cipher-%d", s.next)
	s.values[ciphertext] = plaintext
	return ciphertext, "test-key", nil
}

func (s *memorySealer) Open(ciphertext string) (string, error) {
	value, ok := s.values[ciphertext]
	if !ok {
		return "", errors.New("open failed and must not be returned")
	}
	return value, nil
}

type testConnectionSpec struct{}

func (testConnectionSpec) Fields() []engine.FieldSpec { return nil }

func (testConnectionSpec) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	if params["host"] == "" {
		return "", errors.New("host required")
	}
	return "test://" + params["username"] + ":" + secrets["password"] + "@" + params["host"], nil
}

func (testConnectionSpec) ParseDSN(string) (engine.Params, engine.Secrets, error) {
	return nil, nil, errors.New("not implemented")
}

func columnRef(connectionID int64) credentials.ConnectionRef {
	return credentials.ConnectionRef{
		OrgID:        strconv.FormatInt(columnOrgID, 10),
		WorkspaceID:  strconv.FormatInt(columnWorkspaceID, 10),
		ConnectionID: strconv.FormatInt(connectionID, 10),
	}
}

func newColumnFixture() (*credentials.EncryptedColumnProvider, *memoryColumnStore, credentialstest.Fixture) {
	params := json.RawMessage(`{"host":"db.internal","username":"app"}`)
	store := &memoryColumnStore{
		connections: map[int64]columnRecord{
			columnConnectionID: {orgID: columnOrgID, workspaceID: columnWorkspaceID, driver: "test", defaultScope: "public", params: params},
			columnSSHID: {
				orgID: columnOrgID, workspaceID: columnWorkspaceID, driver: "test", params: params,
				sshConfig: json.RawMessage(`{"enabled":true,"host":"bastion","port":2222,"user":"ops","auth_method":"private_key"}`),
				tlsConfig: json.RawMessage(`{"mode":"verify-full","server_name":"db.internal","client_cert_pem":"client-cert"}`),
			},
			columnBadID:       {orgID: columnOrgID, workspaceID: columnWorkspaceID, driver: "test", params: params},
			columnReferenceID: {orgID: columnOrgID, workspaceID: columnWorkspaceID, driver: "test", params: params},
		},
		secrets: map[int64]map[string]secretRecord{
			columnConnectionID: {"password": {source: "stored", ciphertext: "password-cipher", keyID: "test-key"}},
			columnSSHID: {
				"password":        {source: "stored", ciphertext: "password-cipher", keyID: "test-key"},
				"ssh_password":    {source: "stored", ciphertext: "ssh-cipher", keyID: "test-key"},
				"ssh_private_key": {source: "stored", ciphertext: "ssh-key-cipher", keyID: "test-key"},
				"ssh_passphrase":  {source: "stored", ciphertext: "ssh-phrase-cipher", keyID: "test-key"},
				"tls_client_key":  {source: "stored", ciphertext: "tls-key-cipher", keyID: "test-key"},
			},
			columnBadID:       {"password": {source: "stored", ciphertext: "broken-cipher", keyID: "old-key"}},
			columnReferenceID: {"password": {source: "reference", ciphertext: columnReference, keyID: ""}},
		},
	}
	sealer := &memorySealer{values: map[string]string{
		"password-cipher":   columnPassword,
		"ssh-cipher":        columnSSHPassword,
		"ssh-key-cipher":    columnSSHKey,
		"ssh-phrase-cipher": columnSSHPhrase,
		"tls-key-cipher":    columnTLSKey,
	}}
	provider := credentials.NewEncryptedColumnProvider(store, sealer, func(driver string) (engine.ConnectionSpec, bool) {
		return testConnectionSpec{}, driver == "test"
	})
	wantDSN := "test://app:" + columnPassword + "@db.internal"
	return provider, store, credentialstest.Fixture{
		Ref:                  columnRef(columnConnectionID),
		WantDSN:              wantDSN,
		WithSSH:              columnRef(columnSSHID),
		WrongWorkspace:       credentials.ConnectionRef{OrgID: strconv.FormatInt(columnOrgID, 10), WorkspaceID: "999", ConnectionID: strconv.FormatInt(columnConnectionID, 10)},
		WrongOrg:             credentials.ConnectionRef{OrgID: "999", WorkspaceID: strconv.FormatInt(columnWorkspaceID, 10), ConnectionID: strconv.FormatInt(columnConnectionID, 10)},
		Missing:              columnRef(999),
		Undecryptable:        columnRef(columnBadID),
		UndecryptableMention: "connection 33",
		Reference:            columnRef(columnReferenceID),
		ReferenceName:        credentials.SecretPassword,
		StoredName:           credentials.SecretPassword,
		StoredValue:          columnPassword,
		WantStates: map[credentials.SecretName]credentials.SecretState{
			credentials.SecretPassword:      {Set: true, Source: credentials.SourceStored},
			credentials.SecretSSHPassword:   {},
			credentials.SecretSSHPrivateKey: {},
			credentials.SecretSSHPassphrase: {},
			credentials.SecretTLSClientKey:  {},
		},
		Secrets: []string{columnPassword, columnSSHPassword, columnSSHKey, columnSSHPhrase, columnTLSKey, columnReference},
	}
}

func TestEncryptedColumnProviderContract(t *testing.T) {
	provider, _, fixture := newColumnFixture()
	credentialstest.RunProviderContract(t, provider, fixture)
}

func TestEncryptedColumnProviderBuildsStructuredCredentials(t *testing.T) {
	provider, _, _ := newColumnFixture()
	got, err := provider.Resolve(context.Background(), columnRef(columnSSHID))
	if err != nil {
		t.Fatal(err)
	}
	if got.Driver != "test" || got.SSH == nil || got.SSH.Host != "bastion" ||
		got.SSH.Password != columnSSHPassword || got.SSH.PrivateKeyPEM != columnSSHKey || got.SSH.Passphrase != columnSSHPhrase {
		t.Fatalf("unexpected structured credentials")
	}
	if got.TLS == nil || got.TLS.ServerName != "db.internal" || got.TLS.ClientKeyPEM != columnTLSKey {
		t.Fatalf("unexpected structured TLS credentials")
	}
}

func TestEncryptedColumnProviderRevealDecryptFailureNamesConnection(t *testing.T) {
	provider, _, fixture := newColumnFixture()
	_, err := provider.Reveal(context.Background(), fixture.Undecryptable, credentials.SecretPassword)
	if err == nil || errors.Is(err, credentials.ErrNotFound) || errors.Is(err, credentials.ErrNotRevealable) {
		t.Fatalf("Reveal error = %v", err)
	}
	if !strings.Contains(err.Error(), "connection 33") || strings.Contains(err.Error(), "must not be returned") {
		t.Fatalf("Reveal error text is wrong: %v", err)
	}
}

func TestEncryptedColumnProviderWriter(t *testing.T) {
	provider, store, _ := newColumnFixture()
	ctx := context.Background()

	if err := provider.Set(ctx, columnRef(columnConnectionID), credentials.SecretTLSClientKey, "new-client-key"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	stored := store.secrets[columnConnectionID][string(credentials.SecretTLSClientKey)]
	if stored.source != string(credentials.SourceStored) || stored.keyID != "test-key" || stored.ciphertext == "new-client-key" {
		t.Fatalf("secret was not sealed correctly: %+v", stored)
	}
	got, err := provider.Reveal(ctx, columnRef(columnConnectionID), credentials.SecretTLSClientKey)
	if err != nil || got != "new-client-key" {
		t.Fatalf("Reveal after Set: value matched=%v err=%v", got == "new-client-key", err)
	}

	if err := provider.Clear(ctx, columnRef(columnConnectionID), credentials.SecretTLSClientKey); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok := store.secrets[columnConnectionID][string(credentials.SecretTLSClientKey)]; ok {
		t.Fatal("secret still stored after Clear")
	}
	if _, err := provider.Reveal(ctx, columnRef(columnConnectionID), credentials.SecretTLSClientKey); !errors.Is(err, credentials.ErrNotRevealable) {
		t.Fatalf("Reveal after Clear: %v", err)
	}
}

func TestEncryptedColumnProviderResolveRejectsReferenceSecret(t *testing.T) {
	provider, _, fixture := newColumnFixture()
	_, err := provider.Resolve(context.Background(), fixture.Reference)
	if err == nil || errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Resolve error = %v", err)
	}
	text := err.Error()
	if !strings.Contains(text, "connection 34") || !strings.Contains(text, `"password"`) || strings.Contains(text, columnReference) {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestEncryptedColumnProviderStoredRowWithoutCiphertext(t *testing.T) {
	provider, store, fixture := newColumnFixture()
	delete(store.secrets[columnConnectionID], "password")
	store.secrets[columnConnectionID]["password"] = secretRecord{source: "stored"}
	store.omitCiphertext = true

	_, resolveErr := provider.Resolve(context.Background(), fixture.Ref)
	_, revealErr := provider.Reveal(context.Background(), fixture.Ref, credentials.SecretPassword)
	for _, err := range []error{resolveErr, revealErr} {
		if err == nil || errors.Is(err, credentials.ErrNotRevealable) || !strings.Contains(err.Error(), "connection 31") {
			t.Fatalf("expected decrypt failure naming the connection, got %v", err)
		}
	}
}

func TestEncryptedColumnProviderClearIsIdempotent(t *testing.T) {
	provider, _, fixture := newColumnFixture()
	ctx := context.Background()
	if err := provider.Clear(ctx, fixture.Ref, credentials.SecretTLSClientKey); err != nil {
		t.Fatalf("Clear never-set: %v", err)
	}
	if err := provider.Clear(ctx, fixture.Ref, credentials.SecretPassword); err != nil {
		t.Fatalf("first Clear: %v", err)
	}
	if err := provider.Clear(ctx, fixture.Ref, credentials.SecretPassword); err != nil {
		t.Fatalf("second Clear: %v", err)
	}
}

func TestEncryptedColumnProviderWriterRejectsUnknownAndForeignRefs(t *testing.T) {
	provider, _, fixture := newColumnFixture()
	if err := provider.Set(context.Background(), fixture.Ref, credentials.SecretName("other"), "value"); !errors.Is(err, credentials.ErrUnknownSecret) {
		t.Fatalf("unknown Set error = %v", err)
	}
	if err := provider.Clear(context.Background(), fixture.WrongOrg, credentials.SecretPassword); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("foreign Clear error = %v", err)
	}
}

var _ credentials.Store = (*memoryColumnStore)(nil)
var _ credentials.Store = (*database.DB)(nil)
