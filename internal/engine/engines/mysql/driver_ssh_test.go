package mysql

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestApplySSHDialerRegistersNetAndDelegates(t *testing.T) {
	sentinel := errors.New("dialer invoked")
	d := &Driver{}
	dsn, err := d.applySSHDialer("u:p@tcp(db.internal:3306)/app", func(_ context.Context, network, addr string) (net.Conn, error) {
		_ = network
		_ = addr
		return nil, sentinel
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.releaseRegistrations)
	if d.netName == "" {
		t.Fatal("want netName recorded")
	}
	if !strings.Contains(dsn, "@"+d.netName+"(") {
		t.Fatalf("dsn %q does not reference registered net %q", dsn, d.netName)
	}
}

func TestApplySSHDialerNilIsPassthrough(t *testing.T) {
	d := &Driver{}
	dsn, err := d.applySSHDialer("u:p@tcp(127.0.0.1:3306)/app", nil)
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "u:p@tcp(127.0.0.1:3306)/app" {
		t.Fatalf("passthrough changed dsn: %q", dsn)
	}
	if d.netName != "" {
		t.Fatal("want no netName for nil dialer")
	}
}

func TestReleaseRegistrationsDeregistersNet(t *testing.T) {
	d := &Driver{}
	_, err := d.applySSHDialer("u:p@tcp(127.0.0.1:3306)/app", func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, errors.New("x")
	})
	if err != nil {
		t.Fatal(err)
	}
	d.releaseRegistrations()
	if d.netName != "" {
		t.Fatal("want netName cleared")
	}
}
