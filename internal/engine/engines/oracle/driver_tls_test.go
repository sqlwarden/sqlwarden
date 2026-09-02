package oracle

import (
	"strings"
	"testing"

	go_ora "github.com/sijms/go-ora/v2"

	"github.com/sqlwarden/internal/engine"
)

func TestOracleTLSSpec(t *testing.T) {
	spec := (&oracleDriver{}).TLSSpec()
	if len(spec.Modes) != 4 || !spec.SupportsCABundle || !spec.SupportsClientCert {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if spec.SupportsServerName {
		t.Fatal("oracle cannot honor a server-name override; must not advertise it")
	}
}

func TestEnsureOracleSSL(t *testing.T) {
	got := ensureOracleSSL("oracle://u:p@h:1521/ORCLPDB1")
	if !strings.Contains(got, "SSL=true") {
		t.Fatalf("want SSL=true added, got %q", got)
	}
	// idempotent
	if got2 := ensureOracleSSL(got); got2 != got {
		t.Fatalf("not idempotent: %q -> %q", got, got2)
	}
}

func TestOracleBuildConnectorAttachesTLS(t *testing.T) {
	conn, err := buildOracleConnector(engine.ConnectionConfig{
		DSN: "oracle://u:p@127.0.0.1:1521/ORCLPDB1",
		TLS: &engine.TLSConfig{Mode: engine.TLSModeVerifyFull},
	})
	if err != nil {
		t.Fatal(err)
	}
	sc, ok := conn.(schemaConnector)
	if !ok {
		t.Fatalf("want schemaConnector, got %T", conn)
	}
	if _, ok := sc.inner.(*go_ora.OracleConnector); !ok {
		t.Fatalf("want *go_ora.OracleConnector inner, got %T", sc.inner)
	}
}

func TestOracleBuildConnectorNoTLS(t *testing.T) {
	conn, err := buildOracleConnector(engine.ConnectionConfig{DSN: "oracle://u:p@127.0.0.1:1521/ORCLPDB1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := conn.(schemaConnector); !ok {
		t.Fatalf("want schemaConnector, got %T", conn)
	}
}
