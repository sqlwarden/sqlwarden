package oracle

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func connConfigWithSSH(dial func(context.Context, string, string) (net.Conn, error)) engine.ConnectionConfig {
	return engine.ConnectionConfig{
		DSN:       "oracle://u:p@db.internal:1521/XEPDB1",
		SSHDialer: dial,
	}
}

func TestSSHDialerAdapterDelegates(t *testing.T) {
	var gotNetwork, gotAddr string
	sentinel := errors.New("invoked")
	a := sshDialerAdapter{dial: func(_ context.Context, network, address string) (net.Conn, error) {
		gotNetwork, gotAddr = network, address
		return nil, sentinel
	}}
	if _, err := a.DialContext(context.Background(), "tcp", "db.internal:1521"); !errors.Is(err, sentinel) {
		t.Fatalf("adapter did not delegate: %v", err)
	}
	if gotNetwork != "tcp" || gotAddr != "db.internal:1521" {
		t.Fatalf("delegate got (%q,%q)", gotNetwork, gotAddr)
	}
}

func TestBuildOracleConnectorWithSSHDialer(t *testing.T) {
	sentinel := errors.New("invoked")
	c, err := buildOracleConnector(connConfigWithSSH(func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, sentinel
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil connector")
	}
	if _, err := buildOracleConnector(connConfigWithSSH(nil)); err != nil {
		t.Fatalf("no-ssh build: %v", err)
	}
}
