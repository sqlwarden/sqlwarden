package postgres

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func TestBuildPgxConfigSetsDialFuncFromSSHDialer(t *testing.T) {
	var gotNetwork, gotAddr string
	sentinel := errors.New("dialer invoked")
	cfg := engine.ConnectionConfig{
		DSN: "postgres://u:p@db.internal:5432/db",
		SSHDialer: func(_ context.Context, network, addr string) (net.Conn, error) {
			gotNetwork, gotAddr = network, addr
			return nil, sentinel
		},
	}
	pgxCfg, err := buildPgxConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pgxCfg.DialFunc == nil {
		t.Fatal("want DialFunc set from SSHDialer")
	}
	if _, err := pgxCfg.DialFunc(context.Background(), "tcp", "db.internal:5432"); !errors.Is(err, sentinel) {
		t.Fatalf("DialFunc did not delegate to SSHDialer: %v", err)
	}
	if gotNetwork != "tcp" || gotAddr != "db.internal:5432" {
		t.Fatalf("delegate got (%q,%q)", gotNetwork, gotAddr)
	}
}

func TestBuildPgxConfigNoSSHDialerDoesNotRouteThroughSentinel(t *testing.T) {
	var invoked bool
	// A build without an SSHDialer must not install our wrapper. Prove it by
	// building one config with a recording dialer and one without, and checking
	// only the former reaches the recorder.
	withCfg, err := buildPgxConfig(engine.ConnectionConfig{
		DSN: "postgres://u:p@127.0.0.1:5432/db",
		SSHDialer: func(context.Context, string, string) (net.Conn, error) {
			invoked = true
			return nil, errors.New("stop")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = withCfg.DialFunc(context.Background(), "tcp", "127.0.0.1:5432")
	if !invoked {
		t.Fatal("SSHDialer build should route through the supplied dialer")
	}

	invoked = false
	plainCfg, err := buildPgxConfig(engine.ConnectionConfig{DSN: "postgres://u:p@127.0.0.1:5432/db"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = plainCfg.DialFunc(context.Background(), "tcp", "127.0.0.1:5432")
	if invoked {
		t.Fatal("plain build must not route through the previous dialer")
	}
}
