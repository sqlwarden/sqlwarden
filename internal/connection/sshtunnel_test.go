package connection

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// --- test harness: an SSH server that honours direct-tcpip and an echo target ---

func newHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// echoListener accepts connections and echoes bytes until EOF.
func echoListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

type sshServerOpts struct {
	hostKey       ssh.Signer
	password      string
	authorizedKey ssh.PublicKey
}

func startSSHServer(t *testing.T, opts sshServerOpts) string {
	t.Helper()
	cfg := &ssh.ServerConfig{}
	if opts.password != "" {
		cfg.PasswordCallback = func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == opts.password {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("bad password")
		}
	}
	if opts.authorizedKey != nil {
		cfg.PublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) == string(opts.authorizedKey.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("unknown key")
		}
	}
	cfg.AddHostKey(opts.hostKey)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			nConn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSSHConn(nConn, cfg)
		}
	}()
	return ln.Addr().String()
}

func serveSSHConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "direct-tcpip" {
			_ = newCh.Reject(ssh.UnknownChannelType, "only direct-tcpip")
			continue
		}
		var payload struct {
			Host  string
			Port  uint32
			Orig  string
			OPort uint32
		}
		if err := ssh.Unmarshal(newCh.ExtraData(), &payload); err != nil {
			_ = newCh.Reject(ssh.ConnectionFailed, "bad payload")
			continue
		}
		target, err := net.Dial("tcp", net.JoinHostPort(payload.Host, strconv.Itoa(int(payload.Port))))
		if err != nil {
			_ = newCh.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			_ = target.Close()
			continue
		}
		go ssh.DiscardRequests(chReqs)
		go func() { _, _ = io.Copy(ch, target); _ = ch.Close() }()
		go func() { _, _ = io.Copy(target, ch); _ = target.Close() }()
	}
}

func hostKeyLine(addr string, key ssh.PublicKey) string {
	return fmt.Sprintf("%s %s", addr, string(ssh.MarshalAuthorizedKey(key)))
}

func hostOf(t *testing.T, addr string) string {
	t.Helper()
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	n, err := net.LookupPort("tcp", p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func encryptedPEM(t *testing.T, key *rsa.PrivateKey, passphrase string) []byte {
	t.Helper()
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

// --- tests ---

func TestOpenTunnelPasswordAuthDialsThrough(t *testing.T) {
	hk := newHostKey(t)
	sshAddr := startSSHServer(t, sshServerOpts{hostKey: hk, password: "s3cret"})
	echo := echoListener(t)

	tun, err := OpenTunnel(context.Background(), SSHConfig{
		Host: hostOf(t, sshAddr), Port: portOf(t, sshAddr), User: "bastion",
		AuthMethod: SSHAuthPassword, Password: "s3cret",
		KnownHostsEntry: hostKeyLine(sshAddr, hk.PublicKey()),
	})
	if err != nil {
		t.Fatalf("OpenTunnel: %v", err)
	}
	t.Cleanup(func() { _ = tun.Close() })

	conn, err := tun.DialContext(context.Background(), "tcp", echo.Addr().String())
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q", buf)
	}
	if !tun.Healthy() {
		t.Fatal("want Healthy() true right after open")
	}
}

func TestOpenTunnelPrivateKeyWithPassphrase(t *testing.T) {
	hk := newHostKey(t)
	clientRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	clientSigner, err := ssh.NewSignerFromKey(clientRSA)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := encryptedPEM(t, clientRSA, "hunter2")

	sshAddr := startSSHServer(t, sshServerOpts{hostKey: hk, authorizedKey: clientSigner.PublicKey()})
	echo := echoListener(t)

	tun, err := OpenTunnel(context.Background(), SSHConfig{
		Host: hostOf(t, sshAddr), Port: portOf(t, sshAddr), User: "bastion",
		AuthMethod: SSHAuthPrivateKey, PrivateKeyPEM: string(pemBytes), Passphrase: "hunter2",
		Fingerprint: ssh.FingerprintSHA256(hk.PublicKey()),
	})
	if err != nil {
		t.Fatalf("OpenTunnel: %v", err)
	}
	t.Cleanup(func() { _ = tun.Close() })
	if _, err := tun.DialContext(context.Background(), "tcp", echo.Addr().String()); err != nil {
		t.Fatalf("DialContext: %v", err)
	}
}

func TestOpenTunnelRejectsWrongHostKey(t *testing.T) {
	hk := newHostKey(t)
	other := newHostKey(t)
	sshAddr := startSSHServer(t, sshServerOpts{hostKey: hk, password: "pw"})

	_, err := OpenTunnel(context.Background(), SSHConfig{
		Host: hostOf(t, sshAddr), Port: portOf(t, sshAddr), User: "u",
		AuthMethod: SSHAuthPassword, Password: "pw",
		Fingerprint: ssh.FingerprintSHA256(other.PublicKey()),
	})
	if err == nil {
		t.Fatal("want host-key mismatch error")
	}
}

func TestOpenTunnelRequiresHostKeyMaterial(t *testing.T) {
	_, err := OpenTunnel(context.Background(), SSHConfig{
		Host: "127.0.0.1", Port: 22, User: "u",
		AuthMethod: SSHAuthPassword, Password: "pw",
	})
	if err == nil {
		t.Fatal("want error when no host-key material and InsecureSkipHostKey=false")
	}
}

func TestOpenTunnelInsecureSkipHostKey(t *testing.T) {
	hk := newHostKey(t)
	sshAddr := startSSHServer(t, sshServerOpts{hostKey: hk, password: "pw"})
	tun, err := OpenTunnel(context.Background(), SSHConfig{
		Host: hostOf(t, sshAddr), Port: portOf(t, sshAddr), User: "u",
		AuthMethod: SSHAuthPassword, Password: "pw", InsecureSkipHostKey: true,
	})
	if err != nil {
		t.Fatalf("OpenTunnel: %v", err)
	}
	_ = tun.Close()
}

func TestTunnelCloseStopsKeepalive(t *testing.T) {
	hk := newHostKey(t)
	sshAddr := startSSHServer(t, sshServerOpts{hostKey: hk, password: "pw"})
	tun, err := OpenTunnel(context.Background(), SSHConfig{
		Host: hostOf(t, sshAddr), Port: portOf(t, sshAddr), User: "u",
		AuthMethod: SSHAuthPassword, Password: "pw", InsecureSkipHostKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tun.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := tun.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if tun.Healthy() {
		t.Fatal("want Healthy() false after Close")
	}
	select {
	case <-tun.done:
	case <-time.After(time.Second):
		t.Fatal("keepalive goroutine did not stop")
	}
}

func TestDialContextHonoursCancelledContext(t *testing.T) {
	hk := newHostKey(t)
	sshAddr := startSSHServer(t, sshServerOpts{hostKey: hk, password: "pw"})
	tun, err := OpenTunnel(context.Background(), SSHConfig{
		Host: hostOf(t, sshAddr), Port: portOf(t, sshAddr), User: "u",
		AuthMethod: SSHAuthPassword, Password: "pw", InsecureSkipHostKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tun.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tun.DialContext(ctx, "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("want error for cancelled context")
	}
}
