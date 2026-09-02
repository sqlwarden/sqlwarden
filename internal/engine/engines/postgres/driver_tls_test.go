package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func TestPostgresTLSSpec(t *testing.T) {
	spec := (&postgresDriver{}).TLSSpec()
	if len(spec.Modes) != 4 || !spec.SupportsCABundle || !spec.SupportsClientCert || !spec.SupportsServerName {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestPostgresBuildPgxConfigAppliesStructuredTLS(t *testing.T) {
	cfg := engine.ConnectionConfig{
		DSN: "postgres://u:p@127.0.0.1:5432/db?sslmode=require",
		TLS: &engine.TLSConfig{Mode: engine.TLSModeVerifyFull, ServerName: "override.example"},
	}
	pgxCfg, err := buildPgxConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pgxCfg.TLSConfig == nil {
		t.Fatal("want TLSConfig set from structured material")
	}
	// Our structured config wins over the DSN's sslmode=require: verify-full
	// means stdlib verification stays on and our ServerName override is kept.
	if pgxCfg.TLSConfig.InsecureSkipVerify {
		t.Fatal("verify-full: want InsecureSkipVerify=false")
	}
	if pgxCfg.TLSConfig.ServerName != "override.example" {
		t.Fatalf("ServerName=%q, want override.example", pgxCfg.TLSConfig.ServerName)
	}
	if _, ok := pgxCfg.RuntimeParams["sslmode"]; ok {
		t.Fatal("want sslmode stripped from RuntimeParams")
	}
}

func TestPostgresBuildPgxConfigServerNameDefaultsToHost(t *testing.T) {
	pgxCfg, err := buildPgxConfig(engine.ConnectionConfig{
		DSN: "postgres://u:p@db.internal:5432/db",
		TLS: &engine.TLSConfig{Mode: engine.TLSModeRequire},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pgxCfg.TLSConfig.ServerName != "db.internal" {
		t.Fatalf("ServerName=%q, want db.internal", pgxCfg.TLSConfig.ServerName)
	}
}

func TestPostgresBuildPgxConfigNoTLS(t *testing.T) {
	if _, err := buildPgxConfig(engine.ConnectionConfig{DSN: "postgres://u:p@127.0.0.1:5432/db"}); err != nil {
		t.Fatal(err)
	}
}
