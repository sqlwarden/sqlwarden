package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/engine"
	_ "github.com/sqlwarden/internal/engine/engines/cockroachdb"
	_ "github.com/sqlwarden/internal/engine/engines/mariadb"
	_ "github.com/sqlwarden/internal/engine/engines/mysql"
	_ "github.com/sqlwarden/internal/engine/engines/neon"
	_ "github.com/sqlwarden/internal/engine/engines/oracle"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
	_ "github.com/sqlwarden/internal/engine/engines/sqlserver"
	_ "github.com/sqlwarden/internal/engine/engines/supabase"
	_ "github.com/sqlwarden/internal/engine/engines/tidb"
	_ "github.com/sqlwarden/internal/engine/engines/yugabyte"
)

type fakeLegacyRow struct {
	driver              string
	dsn, tlsDoc, sshDoc string
	params, tls, ssh    json.RawMessage
	structured          bool
}

type fakeSecret struct{ source, ciphertext, keyID string }

type fakeRotationStore struct {
	rows      map[int64]*fakeLegacyRow
	secrets   map[int64]map[string]fakeSecret
	skipSplit bool
}

func newFakeRotationStore() *fakeRotationStore {
	return &fakeRotationStore{rows: map[int64]*fakeLegacyRow{}, secrets: map[int64]map[string]fakeSecret{}}
}

func (s *fakeRotationStore) isLegacy(r *fakeLegacyRow) bool {
	return r.dsn != "" || r.tlsDoc != "" || r.sshDoc != ""
}

func (s *fakeRotationStore) sortedIDs() []int64 {
	ids := make([]int64, 0, len(s.rows))
	for id := range s.rows {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *fakeRotationStore) ListLegacyConnectionIDs(context.Context) ([]int64, error) {
	var out []int64
	for _, id := range s.sortedIDs() {
		if s.isLegacy(s.rows[id]) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *fakeRotationStore) GetLegacyConnection(_ context.Context, id int64) (string, string, string, string, bool, bool, bool, bool, error) {
	r, ok := s.rows[id]
	if !ok {
		return "", "", "", "", false, false, false, false, nil
	}
	return r.driver, r.dsn, r.tlsDoc, r.sshDoc, r.dsn != "", r.tlsDoc != "", r.sshDoc != "", true, nil
}

func (s *fakeRotationStore) SplitLegacyConnection(
	_ context.Context,
	id int64,
	consumeDSN, consumeTLS, consumeSSH bool,
	params, tlsConfig, sshConfig json.RawMessage,
	names, ciphertexts, keyIDs []string,
) (bool, error) {
	r := s.rows[id]
	if !s.isLegacy(r) {
		return false, nil
	}
	if s.skipSplit {
		r.dsn, r.tlsDoc, r.sshDoc = "", "", ""
		return false, nil
	}
	if len(params) > 0 {
		r.params = params
	}
	if len(tlsConfig) > 0 {
		r.tls = tlsConfig
	}
	if len(sshConfig) > 0 {
		r.ssh = sshConfig
	}
	if consumeDSN {
		r.dsn = ""
	}
	if consumeTLS {
		r.tlsDoc = ""
	}
	if consumeSSH {
		r.sshDoc = ""
	}
	r.structured = true
	if s.secrets[id] == nil {
		s.secrets[id] = map[string]fakeSecret{}
	}
	for i, name := range names {
		s.secrets[id][name] = fakeSecret{"stored", ciphertexts[i], keyIDs[i]}
	}
	return true, nil
}

func (s *fakeRotationStore) ListConnectionIDsWithSecrets(context.Context) ([]int64, error) {
	var out []int64
	for _, id := range s.sortedIDs() {
		if len(s.secrets[id]) > 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *fakeRotationStore) ListConnectionSecretValues(_ context.Context, id int64) (map[string]string, map[string]string, error) {
	sources, ciphertexts := map[string]string{}, map[string]string{}
	for name, sec := range s.secrets[id] {
		sources[name], ciphertexts[name] = sec.source, sec.ciphertext
	}
	return sources, ciphertexts, nil
}

func (s *fakeRotationStore) ReencryptConnectionSecrets(_ context.Context, id int64, names, ciphertexts, keyIDs []string) error {
	for i, name := range names {
		secret := s.secrets[id][name]
		secret.ciphertext = ciphertexts[i]
		secret.keyID = keyIDs[i]
		s.secrets[id][name] = secret
	}
	return nil
}

func mustKeyring(t *testing.T, primary string, previous ...string) *encrypt.Keyring {
	t.Helper()
	kr, err := encrypt.NewKeyring(primary, previous...)
	if err != nil {
		t.Fatal(err)
	}
	return kr
}

func seal(t *testing.T, kr *encrypt.Keyring, v string) string {
	t.Helper()
	out, err := kr.Encrypt(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sealDoc(t *testing.T, kr *encrypt.Keyring, doc any) string {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return seal(t, kr, string(raw))
}

type engineFixture struct {
	driver, dsn string
	password    string
	nativeTLS   bool
	wantMode    string
}

var engineFixtures = []engineFixture{
	{"postgres", "postgresql://app:pw-pg@db.example.com:5432/app?sslmode=verify-full", "pw-pg", true, "verify-full"},
	{"mysql", "app:pw-my@tcp(db.example.com:3306)/app?tls=skip-verify", "pw-my", true, "require"},
	{"mariadb", "app:pw-ma@tcp(db.example.com:3306)/app?tls=skip-verify", "pw-ma", true, "require"},
	{"tidb", "root:pw-ti@tcp(gateway.example.com:4000)/app?tls=true", "pw-ti", true, "verify-full"},
	{"sqlserver", "sqlserver://sa:pw-ms@sql.example.com:1433?database=app&encrypt=true", "pw-ms", true, "verify-full"},
	{"oracle", "oracle://system:pw-or@oracle.example.com:1521/ORCLPDB1?SSL=enable", "pw-or", true, "require"},
	{"cockroachdb", "postgresql://root:pw-cr@cluster.example.com:26257/defaultdb?sslmode=verify-full", "pw-cr", true, "verify-full"},
	{"neon", "postgresql://owner:pw-ne@ep-example.neon.tech:5432/neondb?sslmode=require", "pw-ne", true, "require"},
	{"supabase", "postgresql://postgres:pw-su@db.project.supabase.co:5432/postgres?sslmode=require", "pw-su", true, "require"},
	{"yugabyte", "postgresql://yugabyte:pw-yb@yb.example.com:5433/yugabyte?sslmode=require", "pw-yb", true, "require"},
	{"sqlite", "file:/tmp/app.db", "", false, ""},
}

func TestRotatorSplitsLegacyRowsPerEngine(t *testing.T) {
	ctx := context.Background()
	old := mustKeyring(t, "old-key")
	active := mustKeyring(t, "active-key", "old-key")

	store := newFakeRotationStore()
	for i, fx := range engineFixtures {
		id := int64(i + 1)
		store.rows[id] = &fakeLegacyRow{
			driver: fx.driver,
			dsn:    seal(t, old, fx.dsn),
			tlsDoc: sealDoc(t, old, TLSDocument{Mode: "verify-ca", CAPEM: "CA", ClientCertPEM: "CERT", ClientKeyPEM: "TLSKEY"}),
			sshDoc: sealDoc(t, old, SSHDocument{
				Enabled: true, Host: "bastion", Port: 22, User: "tunnel", AuthMethod: "password",
				Password: "sshpw", PrivateKeyPEM: "SSHKEY", Passphrase: "phrase",
			}),
		}
	}

	rep, err := NewRotator(store, active, engine.ConnectionSpecFor).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Split != len(engineFixtures) {
		t.Fatalf("Split = %d, want %d", rep.Split, len(engineFixtures))
	}

	for i, fx := range engineFixtures {
		id := int64(i + 1)
		row := store.rows[id]
		if row.dsn != "" || row.tlsDoc != "" || row.sshDoc != "" || !row.structured {
			t.Fatalf("%s: legacy columns not cleared: %+v", fx.driver, row)
		}
		if strings.Contains(string(row.params), "tls_mode") {
			t.Errorf("%s: tls_mode left in params: %s", fx.driver, row.params)
		}
		if strings.Contains(string(row.params)+string(row.tls)+string(row.ssh), "pw-") {
			t.Errorf("%s: plaintext password in structured columns", fx.driver)
		}
		for _, secret := range []string{"TLSKEY", "sshpw", "SSHKEY", "phrase"} {
			if strings.Contains(string(row.tls)+string(row.ssh), secret) {
				t.Errorf("%s: secret %q in structured columns", fx.driver, secret)
			}
		}

		var tls TLSDocument
		if err := json.Unmarshal(row.tls, &tls); err != nil {
			t.Fatalf("%s: tls: %v", fx.driver, err)
		}
		if tls.Mode != "verify-ca" || tls.CAPEM != "CA" || tls.ClientCertPEM != "CERT" {
			t.Errorf("%s: tls document = %+v; stored TLS mode must win over the DSN", fx.driver, tls)
		}
		var ssh SSHDocument
		if err := json.Unmarshal(row.ssh, &ssh); err != nil {
			t.Fatalf("%s: ssh: %v", fx.driver, err)
		}
		if !ssh.Enabled || ssh.Host != "bastion" || ssh.User != "tunnel" {
			t.Errorf("%s: ssh document = %+v", fx.driver, ssh)
		}

		want := map[string]string{
			string(SecretTLSClientKey):  "TLSKEY",
			string(SecretSSHPassword):   "sshpw",
			string(SecretSSHPrivateKey): "SSHKEY",
			string(SecretSSHPassphrase): "phrase",
		}
		if fx.password != "" {
			want[string(SecretPassword)] = fx.password
		}
		if len(store.secrets[id]) != len(want) {
			t.Errorf("%s: %d secrets, want %d", fx.driver, len(store.secrets[id]), len(want))
		}
		for name, plain := range want {
			sec, ok := store.secrets[id][name]
			if !ok {
				t.Errorf("%s: secret %q missing", fx.driver, name)
				continue
			}
			if sec.source != string(SourceStored) || sec.keyID != active.PrimaryKeyID() || active.NeedsRotation(sec.ciphertext) {
				t.Errorf("%s: secret %q not sealed with the active key: %+v", fx.driver, name, sec)
			}
			got, err := active.Decrypt(sec.ciphertext)
			if err != nil || got != plain {
				t.Errorf("%s: secret %q round trip failed", fx.driver, name)
			}
		}
	}
}

func TestRotatorLiftsDSNTLSModeWhenNoTLSDocument(t *testing.T) {
	for _, fx := range engineFixtures {
		if !fx.nativeTLS {
			continue
		}
		kr := mustKeyring(t, "k")
		store := newFakeRotationStore()
		store.rows[1] = &fakeLegacyRow{driver: fx.driver, dsn: seal(t, kr, fx.dsn)}
		if _, err := NewRotator(store, kr, engine.ConnectionSpecFor).Run(context.Background()); err != nil {
			t.Fatalf("%s: %v", fx.driver, err)
		}
		var tls TLSDocument
		if err := json.Unmarshal(store.rows[1].tls, &tls); err != nil {
			t.Fatalf("%s: tls_config = %q: %v", fx.driver, store.rows[1].tls, err)
		}
		if tls.Mode != fx.wantMode {
			t.Errorf("%s: mode = %q, want %q", fx.driver, tls.Mode, fx.wantMode)
		}
		if strings.Contains(string(store.rows[1].params), "tls_mode") {
			t.Errorf("%s: tls_mode left in params", fx.driver)
		}
	}
}

func TestRotatorIsIdempotent(t *testing.T) {
	ctx := context.Background()
	old := mustKeyring(t, "old-key")
	active := mustKeyring(t, "active-key", "old-key")
	store := newFakeRotationStore()
	store.rows[1] = &fakeLegacyRow{driver: "postgres", dsn: seal(t, old, engineFixtures[0].dsn)}
	// Personal-space connections are ordinary rows and are split like any other.
	store.rows[2] = &fakeLegacyRow{driver: "mysql", dsn: seal(t, old, engineFixtures[1].dsn)}
	rotator := NewRotator(store, active, engine.ConnectionSpecFor)

	first, err := rotator.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Split != 2 || first.Reencrypted != 0 {
		t.Fatalf("first report = %+v", first)
	}
	snapshot := fmt.Sprint(store.secrets)

	second, err := rotator.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Split != 0 || second.Reencrypted != 0 || second.Skipped != 2 {
		t.Fatalf("second report = %+v", second)
	}
	if fmt.Sprint(store.secrets) != snapshot {
		t.Fatal("second run changed stored secrets")
	}
}

func TestRotatorCountsGuardedAlreadySplitRowAsSkipped(t *testing.T) {
	kr := mustKeyring(t, "active-key")
	store := newFakeRotationStore()
	store.rows[1] = &fakeLegacyRow{driver: "postgres", dsn: seal(t, kr, engineFixtures[0].dsn)}
	store.skipSplit = true

	report, err := NewRotator(store, kr, engine.ConnectionSpecFor).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Split != 0 || report.Skipped != 1 || report.Reencrypted != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRotatorReencryptsStructuredSecretsWithActiveKey(t *testing.T) {
	ctx := context.Background()
	old := mustKeyring(t, "old-key")
	active := mustKeyring(t, "active-key", "old-key")
	store := newFakeRotationStore()
	store.rows[1] = &fakeLegacyRow{structured: true}
	store.secrets[1] = map[string]fakeSecret{
		"password":       {"stored", seal(t, old, "pw"), old.PrimaryKeyID()},
		"ssh_password":   {"stored", seal(t, active, "ssh"), active.PrimaryKeyID()},
		"ssh_passphrase": {"reference", "vault:path", ""},
	}

	rep, err := NewRotator(store, active, engine.ConnectionSpecFor).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Reencrypted != 1 || rep.Skipped != 2 || rep.Split != 0 {
		t.Fatalf("report = %+v", rep)
	}
	pw := store.secrets[1]["password"]
	if active.NeedsRotation(pw.ciphertext) || pw.keyID != active.PrimaryKeyID() {
		t.Fatalf("password not sealed with the active key: %+v", pw)
	}
	if got, _ := active.Decrypt(pw.ciphertext); got != "pw" {
		t.Fatal("re-encrypted password changed value")
	}
	if ref := store.secrets[1]["ssh_passphrase"]; ref.ciphertext != "vault:path" || ref.source != "reference" {
		t.Fatalf("reference secret modified: %+v", ref)
	}
}

type failingSplitStore struct {
	*fakeRotationStore
	splits int
}

func (s *failingSplitStore) SplitLegacyConnection(ctx context.Context, id int64, d, l, h bool, p, t, sdoc json.RawMessage, n, c, k []string) (bool, error) {
	s.splits++
	return s.fakeRotationStore.SplitLegacyConnection(ctx, id, d, l, h, p, t, sdoc, n, c, k)
}

func TestRotatorParseFailureStopsAndNamesConnection(t *testing.T) {
	ctx := context.Background()
	kr := mustKeyring(t, "k")
	store := &failingSplitStore{fakeRotationStore: newFakeRotationStore()}
	good1 := seal(t, kr, engineFixtures[0].dsn)
	bad := seal(t, kr, "postgresql://user:SUPERSECRETPW@")
	good3 := seal(t, kr, engineFixtures[1].dsn)
	store.rows[1] = &fakeLegacyRow{driver: "postgres", dsn: good1}
	store.rows[2] = &fakeLegacyRow{driver: "postgres", dsn: bad}
	store.rows[3] = &fakeLegacyRow{driver: "mysql", dsn: good3}

	rep, err := NewRotator(store, kr, engine.ConnectionSpecFor).Run(ctx)
	if !errors.Is(err, ErrLegacyParse) {
		t.Fatalf("error = %v, want ErrLegacyParse", err)
	}
	if !strings.Contains(err.Error(), "connection 2") {
		t.Fatalf("error %q does not name connection 2", err)
	}
	if strings.Contains(err.Error(), "SUPERSECRETPW") {
		t.Fatalf("error leaks the DSN: %q", err)
	}
	if rep.Split != 1 || store.splits != 1 {
		t.Fatalf("report = %+v, splits = %d; only the earlier row should commit", rep, store.splits)
	}
	if !store.rows[1].structured || store.rows[1].dsn != "" {
		t.Fatal("earlier connection was not committed")
	}
	if store.rows[2].structured || store.rows[2].dsn != bad || len(store.secrets[2]) != 0 {
		t.Fatal("failing connection was modified")
	}
	if store.rows[3].structured || store.rows[3].dsn != good3 {
		t.Fatal("later connection was processed after the failure")
	}
}

func TestRotatorUnknownDriverAndUndecryptableRowsFailWithID(t *testing.T) {
	kr := mustKeyring(t, "k")
	other := mustKeyring(t, "unrelated")
	cases := map[string]*fakeLegacyRow{
		"unknown driver": {driver: "no-such-engine", dsn: seal(t, kr, "x://y")},
		"undecryptable":  {driver: "postgres", dsn: seal(t, other, engineFixtures[0].dsn)},
		"bad tls doc":    {driver: "postgres", dsn: seal(t, kr, engineFixtures[0].dsn), tlsDoc: seal(t, kr, "{not json")},
	}
	for name, row := range cases {
		store := newFakeRotationStore()
		store.rows[7] = row
		_, err := NewRotator(store, kr, engine.ConnectionSpecFor).Run(context.Background())
		if !errors.Is(err, ErrLegacyParse) || !strings.Contains(err.Error(), "connection 7") {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

type fakeStartupStore struct {
	present bool
	count   int
}

func (s fakeStartupStore) StructuredConnectionSchemaPresent(context.Context) (bool, error) {
	return s.present, nil
}
func (s fakeStartupStore) CountLegacyConnections(context.Context) (int, error) { return s.count, nil }

func TestVerifyStartup(t *testing.T) {
	ctx := context.Background()
	if err := VerifyStartup(ctx, fakeStartupStore{present: true}); err != nil {
		t.Fatalf("clean database: %v", err)
	}
	if err := VerifyStartup(ctx, fakeStartupStore{present: false, count: 3}); !errors.Is(err, ErrSchemaOutdated) {
		t.Fatalf("outdated schema: %v", err)
	}
	err := VerifyStartup(ctx, fakeStartupStore{present: true, count: 3})
	if err == nil || err.Error() != "3 connections use the legacy format. Run sqlwarden rotate-keys." {
		t.Fatalf("legacy rows: %v", err)
	}
}
