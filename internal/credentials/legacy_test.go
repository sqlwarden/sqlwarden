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
	"github.com/sqlwarden/internal/encrypt"
)

const (
	testDSN      = "postgres://app:s3cr3t-dsn-pw@db.internal:5432/app"
	testSSHPass  = "s3cr3t-ssh-pw"
	testOrgID    = int64(7)
	testWSID     = int64(11)
	testConnOK   = int64(21)
	testConnSSH  = int64(22)
	testConnTLS  = int64(24)
	testConnBad  = int64(23)
	testOtherWS  = int64(12)
	testOtherOrg = int64(8)
)

type fakeStore struct {
	conns map[int64]database.Connection
	wss   map[int64]database.Workspace
	err   error
}

func (s fakeStore) GetConnection(_ context.Context, id int64) (database.Connection, bool, error) {
	c, ok := s.conns[id]
	return c, ok, s.err
}

func (s fakeStore) GetWorkspace(_ context.Context, id int64) (database.Workspace, bool, error) {
	w, ok := s.wss[id]
	return w, ok, s.err
}

func newFixture(t *testing.T) (*credentials.LegacyDSNProvider, credentialstest.Fixture) {
	t.Helper()
	kr, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	seal := func(s string) string {
		out, err := kr.Encrypt(s)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	sshDoc, err := json.Marshal(map[string]any{
		"enabled": true, "host": "bastion", "port": 2222, "user": "ops",
		"auth_method": "password", "password": testSSHPass,
	})
	if err != nil {
		t.Fatal(err)
	}
	org := testOrgID
	store := fakeStore{
		wss: map[int64]database.Workspace{testWSID: {ID: testWSID, OrgID: &org}},
		conns: map[int64]database.Connection{
			testConnOK:  {ID: testConnOK, WorkspaceID: testWSID, Driver: "postgres", DSNEncrypted: seal(testDSN), DefaultScope: "public"},
			testConnSSH: {ID: testConnSSH, WorkspaceID: testWSID, Driver: "postgres", DSNEncrypted: seal(testDSN), SSHConfigEncrypted: seal(string(sshDoc))},
			testConnBad: {ID: testConnBad, WorkspaceID: testWSID, Driver: "postgres", DSNEncrypted: "not-a-valid-ciphertext"},
		},
	}
	tlsDoc, err := json.Marshal(map[string]any{"mode": "verify-full", "server_name": "db.internal", "client_key_pem": "KEY-PEM"})
	if err != nil {
		t.Fatal(err)
	}
	store.conns[testConnTLS] = database.Connection{ID: testConnTLS, WorkspaceID: testWSID, Driver: "postgres", DSNEncrypted: seal(testDSN), TLSConfigEncrypted: seal(string(tlsDoc))}
	ref := func(conn int64) credentials.ConnectionRef {
		return credentials.ConnectionRef{
			OrgID:        strconv.FormatInt(testOrgID, 10),
			WorkspaceID:  strconv.FormatInt(testWSID, 10),
			ConnectionID: strconv.FormatInt(conn, 10),
		}
	}
	return credentials.NewLegacyDSNProvider(store, kr), credentialstest.Fixture{
		Ref:            ref(testConnOK),
		WantDSN:        testDSN,
		WithSSH:        ref(testConnSSH),
		Missing:        ref(999),
		WrongWorkspace: credentials.ConnectionRef{OrgID: ref(testConnOK).OrgID, WorkspaceID: strconv.FormatInt(testOtherWS, 10), ConnectionID: ref(testConnOK).ConnectionID},
		WrongOrg:       credentials.ConnectionRef{OrgID: strconv.FormatInt(testOtherOrg, 10), WorkspaceID: ref(testConnOK).WorkspaceID, ConnectionID: ref(testConnOK).ConnectionID},
		Undecryptable:  ref(testConnBad),
		Secrets:        []string{"s3cr3t-dsn-pw", testSSHPass},
	}
}

func TestLegacyDSNProviderContract(t *testing.T) {
	p, fx := newFixture(t)
	credentialstest.RunProviderContract(t, p, fx)
}

func TestLegacyDSNProviderResolvesFields(t *testing.T) {
	p, fx := newFixture(t)
	got, err := p.Resolve(context.Background(), fx.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Driver != "postgres" || got.DefaultScope != "public" || got.SSH != nil {
		t.Fatalf("unexpected credentials: driver=%q scope=%q ssh=%v", got.Driver, got.DefaultScope, got.SSH != nil)
	}
}

func TestLegacyDSNProviderDecodesSSH(t *testing.T) {
	p, fx := newFixture(t)
	ref := fx.Ref
	ref.ConnectionID = strconv.FormatInt(testConnSSH, 10)
	got, err := p.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSH == nil || got.SSH.Host != "bastion" || got.SSH.Port != 2222 ||
		got.SSH.AuthMethod != credentials.SSHAuthPassword || got.SSH.Password != testSSHPass {
		t.Fatalf("unexpected ssh config")
	}
}

func TestLegacyDSNProviderOwnershipMismatch(t *testing.T) {
	p, fx := newFixture(t)
	cases := map[string]credentials.ConnectionRef{
		"wrong workspace": {OrgID: fx.Ref.OrgID, WorkspaceID: strconv.FormatInt(testOtherWS, 10), ConnectionID: fx.Ref.ConnectionID},
		"wrong org":       {OrgID: strconv.FormatInt(testOtherOrg, 10), WorkspaceID: fx.Ref.WorkspaceID, ConnectionID: fx.Ref.ConnectionID},
		"invalid conn id": {OrgID: fx.Ref.OrgID, WorkspaceID: fx.Ref.WorkspaceID, ConnectionID: "abc"},
		"invalid ws id":   {OrgID: fx.Ref.OrgID, WorkspaceID: "", ConnectionID: fx.Ref.ConnectionID},
		"invalid org id":  {OrgID: "x", WorkspaceID: fx.Ref.WorkspaceID, ConnectionID: fx.Ref.ConnectionID},
	}
	for name, ref := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := p.Resolve(context.Background(), ref); !errors.Is(err, credentials.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestLegacyDSNProviderStoreError(t *testing.T) {
	kr, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	p := credentials.NewLegacyDSNProvider(fakeStore{err: errors.New("boom")}, kr)
	_, err = p.Resolve(context.Background(), credentials.ConnectionRef{OrgID: "1", WorkspaceID: "1", ConnectionID: "1"})
	if err == nil || errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("err = %v, want non-NotFound error", err)
	}
}

func TestSSHDocumentRoundTrip(t *testing.T) {
	kr, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	in := credentials.SSHDocument{
		Enabled: true, Host: "bastion", Port: 2222, User: "ops",
		AuthMethod: "private_key", PrivateKeyPEM: "pem", Passphrase: "pp",
		KnownHostsEntry: "kh", Fingerprint: "SHA256:abc", InsecureSkipHostKey: true,
		ClearPassword: true,
	}
	sealed, err := credentials.EncodeSSHDocument(kr, in)
	if err != nil {
		t.Fatal(err)
	}
	out, has, err := credentials.DecodeSSHDocument(kr, sealed)
	if err != nil || !has {
		t.Fatalf("decode: has=%v err=%v", has, err)
	}
	if out != in {
		t.Fatalf("round trip mismatch")
	}
	cfg, err := credentials.DecodeSSHConfig(kr, sealed)
	if err != nil || cfg == nil || cfg.Host != "bastion" || cfg.Passphrase != "pp" || cfg.AuthMethod != credentials.SSHAuthPrivateKey {
		t.Fatalf("config decode failed: %v", err)
	}
	empty, err := credentials.EncodeSSHDocument(kr, credentials.SSHDocument{})
	if err != nil || empty != "" {
		t.Fatalf("empty document sealed to %q, %v", empty, err)
	}
}

func TestLegacyDSNProviderDecodesTLS(t *testing.T) {
	p, fx := newFixture(t)
	ref := fx.Ref
	ref.ConnectionID = strconv.FormatInt(testConnTLS, 10)
	got, err := p.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.TLS == nil || got.TLS.Mode != "verify-full" || got.TLS.ServerName != "db.internal" || got.TLS.ClientKeyPEM != "KEY-PEM" {
		t.Fatalf("unexpected tls config: %v", got.TLS != nil)
	}
	plain, err := p.Resolve(context.Background(), fx.Ref)
	if err != nil || plain.TLS != nil {
		t.Fatalf("connection without tls must resolve nil TLS: %v", err)
	}
	if s := fmt.Sprintf("%+v", got); strings.Contains(s, "KEY-PEM") {
		t.Fatal("TLS key material leaked through formatting")
	}
}
