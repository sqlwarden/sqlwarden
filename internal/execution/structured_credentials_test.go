package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/credentials/credentialstest"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/execution"
)

const (
	structuredOrgID       = 7
	structuredWorkspaceID = 11
	structuredPassword    = "structured-password-value"
	structuredSSHPassword = "structured-ssh-password-value"
	structuredReference   = "vault://structured/reference"
)

type structuredRow struct {
	driver    string
	params    json.RawMessage
	sshConfig json.RawMessage
}

type structuredSecret struct {
	source     string
	ciphertext string
}

type structuredStore struct {
	rows    map[int64]structuredRow
	secrets map[int64]map[string]structuredSecret
}

func (s *structuredStore) LoadConnectionCredentials(_ context.Context, orgID, workspaceID, connectionID int64) (
	driver, defaultScope string,
	params, tlsConfig, sshConfig json.RawMessage,
	found bool,
	err error,
) {
	row, ok := s.rows[connectionID]
	if !ok || orgID != structuredOrgID || workspaceID != structuredWorkspaceID {
		return "", "", nil, nil, nil, false, nil
	}
	return row.driver, "", row.params, nil, row.sshConfig, true, nil
}

func (s *structuredStore) ListConnectionSecretValues(_ context.Context, connectionID int64) (sources, ciphertexts map[string]string, err error) {
	sources, ciphertexts = map[string]string{}, map[string]string{}
	for name, secret := range s.secrets[connectionID] {
		sources[name] = secret.source
		ciphertexts[name] = secret.ciphertext
	}
	return sources, ciphertexts, nil
}

func (s *structuredStore) UpsertConnectionSecretValue(context.Context, int64, string, string, string, string) error {
	return errors.New("read-only store")
}

func (s *structuredStore) DeleteConnectionSecret(context.Context, int64, string) error {
	return errors.New("read-only store")
}

type structuredSealer map[string]string

func (s structuredSealer) Seal(string) (string, string, error) {
	return "", "", errors.New("read-only sealer")
}

func (s structuredSealer) Open(ciphertext string) (string, error) {
	value, ok := s[ciphertext]
	if !ok {
		return "", errors.New("open failed")
	}
	return value, nil
}

func structuredRef(connectionID int64) credentials.ConnectionRef {
	return credentials.ConnectionRef{
		OrgID:        strconv.Itoa(structuredOrgID),
		WorkspaceID:  strconv.Itoa(structuredWorkspaceID),
		ConnectionID: strconv.FormatInt(connectionID, 10),
	}
}

func newStructuredProvider(t *testing.T) (*credentials.EncryptedColumnProvider, credentialstest.Fixture, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "structured.db")
	params, err := json.Marshal(map[string]string{"path": dbPath})
	if err != nil {
		t.Fatal(err)
	}
	store := &structuredStore{
		rows: map[int64]structuredRow{
			1: {driver: "sqlite", params: params},
			2: {
				driver: "sqlite", params: params,
				sshConfig: json.RawMessage(`{"enabled":true,"host":"bastion","port":2222,"user":"ops","auth_method":"password"}`),
			},
			3: {driver: "sqlite", params: params},
			4: {driver: "sqlite", params: params},
		},
		secrets: map[int64]map[string]structuredSecret{
			1: {"password": {source: "stored", ciphertext: "password-cipher"}},
			2: {
				"password":     {source: "stored", ciphertext: "password-cipher"},
				"ssh_password": {source: "stored", ciphertext: "ssh-cipher"},
			},
			3: {"password": {source: "stored", ciphertext: "broken-cipher"}},
			4: {"password": {source: "reference", ciphertext: structuredReference}},
		},
	}
	sealer := structuredSealer{"password-cipher": structuredPassword, "ssh-cipher": structuredSSHPassword}
	provider := credentials.NewEncryptedColumnProvider(store, sealer, engine.ConnectionSpecFor)

	fx := credentialstest.Fixture{
		Ref:            structuredRef(1),
		WantDSN:        "file:" + dbPath,
		WithSSH:        structuredRef(2),
		WrongWorkspace: credentials.ConnectionRef{OrgID: strconv.Itoa(structuredOrgID), WorkspaceID: "999", ConnectionID: "1"},
		WrongOrg:       credentials.ConnectionRef{OrgID: "999", WorkspaceID: strconv.Itoa(structuredWorkspaceID), ConnectionID: "1"},
		Missing:        structuredRef(999),
		Undecryptable:  structuredRef(3),
		Reference:      structuredRef(4),
		ReferenceName:  credentials.SecretPassword,
		StoredName:     credentials.SecretPassword,
		StoredValue:    structuredPassword,
		WantStates: map[credentials.SecretName]credentials.SecretState{
			credentials.SecretPassword:      {Set: true, Source: credentials.SourceStored},
			credentials.SecretSSHPassword:   {},
			credentials.SecretSSHPrivateKey: {},
			credentials.SecretSSHPassphrase: {},
			credentials.SecretTLSClientKey:  {},
		},
		Secrets: []string{structuredPassword, structuredSSHPassword, structuredReference},
	}
	return provider, fx, dbPath
}

func TestRuntimeProviderContractIncludesDescribe(t *testing.T) {
	provider, fx, _ := newStructuredProvider(t)
	credentialstest.RunProviderContract(t, provider, fx)
}

func TestRuntimeOpensSessionFromStructuredConnection(t *testing.T) {
	provider, _, _ := newStructuredProvider(t)
	rt := execution.NewLocal(execution.LocalConfig{
		Credentials: provider,
		Policy:      allowPolicy{},
		IdleTimeout: time.Hour,
	})
	t.Cleanup(rt.Shutdown)

	scope := execution.Scope{OrgID: "7", WorkspaceID: "11", AccountID: "a1", ConnectionID: "1"}
	info, err := rt.Open(context.Background(), execution.OpenRequest{Scope: scope})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if info.Driver != "sqlite" || info.Reused {
		t.Fatalf("unexpected session info: driver=%q reused=%v", info.Driver, info.Reused)
	}

	foreign := scope
	foreign.WorkspaceID = "12"
	if _, err := rt.Open(context.Background(), execution.OpenRequest{Scope: foreign}); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("foreign workspace error = %v, want ErrNotFound", err)
	}
}
