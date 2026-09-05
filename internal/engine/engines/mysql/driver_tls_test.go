package mysql

import (
	"crypto/tls"
	"testing"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/sqlwarden/internal/engine"
)

func TestMySQLTLSSpec(t *testing.T) {
	spec := (&Driver{}).TLSSpec()
	if len(spec.Modes) != 4 || !spec.SupportsClientCert || !spec.SupportsCABundle || !spec.SupportsServerName {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestMySQLApplyTLSRegistersUniqueNameAndRelease(t *testing.T) {
	d := &Driver{}
	dsn, err := d.applyTLS("u:p@tcp(127.0.0.1:3306)/db?parseTime=true", &engine.TLSConfig{Mode: engine.TLSModeRequire})
	if err != nil {
		t.Fatal(err)
	}
	if d.tlsName == "" {
		t.Fatal("want a registered TLS name recorded on the driver")
	}
	cfg, err := mysqlconfig.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSConfig != d.tlsName {
		t.Fatalf("DSN TLSConfig=%q, want %q", cfg.TLSConfig, d.tlsName)
	}
	name := d.tlsName
	d.releaseRegistrations()
	if d.tlsName != "" {
		t.Fatal("releaseRegistrations should clear the recorded name")
	}
	// Name is free again after release: re-registering it must not error.
	if err := mysqlconfig.RegisterTLSConfig(name, &tls.Config{InsecureSkipVerify: true}); err != nil { //nolint:gosec // test-only
		t.Fatalf("name still registered after releaseRegistrations: %v", err)
	}
	mysqlconfig.DeregisterTLSConfig(name)
}

func TestMySQLApplyTLSNoop(t *testing.T) {
	d := &Driver{}
	in := "u:p@tcp(127.0.0.1:3306)/db?parseTime=true"
	out, err := d.applyTLS(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != in || d.tlsName != "" {
		t.Fatalf("nil TLS should be a passthrough: out=%q name=%q", out, d.tlsName)
	}
	out, err = d.applyTLS(in, &engine.TLSConfig{Mode: engine.TLSModeDisable})
	if err != nil || out != in || d.tlsName != "" {
		t.Fatalf("disable should be a passthrough: out=%q name=%q err=%v", out, d.tlsName, err)
	}
}
