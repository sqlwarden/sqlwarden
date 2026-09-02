package connection

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHAuthMethod is how the tunnel authenticates to the bastion.
type SSHAuthMethod string

const (
	SSHAuthPassword   SSHAuthMethod = "password"
	SSHAuthPrivateKey SSHAuthMethod = "private_key"
)

const (
	defaultSSHPort       = 22
	sshKeepaliveInterval = 30 * time.Second
	sshHandshakeTimeout  = 15 * time.Second
)

// SSHConfig is the decoded, structured SSH tunnel material for a connection.
type SSHConfig struct {
	Host                string
	Port                int // 0 -> 22
	User                string
	AuthMethod          SSHAuthMethod
	Password            string
	PrivateKeyPEM       string
	Passphrase          string
	KnownHostsEntry     string // known_hosts-style line(s)
	Fingerprint         string // "SHA256:..." pin, alternative to KnownHostsEntry
	InsecureSkipHostKey bool   // explicit opt-out only
}

// Tunnel is an open SSH client to a bastion plus a keepalive goroutine. It
// serves DialContext so a driver can route its TCP transport through the
// bastion without binding a local socket.
type Tunnel struct {
	client  *ssh.Client
	done    chan struct{}
	once    sync.Once
	healthy atomic.Bool
}

// OpenTunnel dials the bastion, verifies its host key, authenticates, and
// starts keepalives. ctx bounds only the handshake and is not retained.
func OpenTunnel(ctx context.Context, cfg SSHConfig) (*Tunnel, error) {
	auth, err := sshAuthMethods(cfg)
	if err != nil {
		return nil, err
	}
	hostKeyCB, err := sshHostKeyCallback(cfg)
	if err != nil {
		return nil, err
	}
	port := cfg.Port
	if port == 0 {
		port = defaultSSHPort
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))

	dialCtx, cancel := context.WithTimeout(ctx, sshHandshakeTimeout)
	defer cancel()
	var d net.Dialer
	tcpConn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh tunnel: dial bastion: %w", err)
	}
	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCB,
		Timeout:         sshHandshakeTimeout,
	}
	c, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
	if err != nil {
		_ = tcpConn.Close()
		return nil, fmt.Errorf("ssh tunnel: handshake: %w", err)
	}
	client := ssh.NewClient(c, chans, reqs)

	t := &Tunnel{client: client, done: make(chan struct{})}
	t.healthy.Store(true)
	go t.keepalive()
	return t, nil
}

// DialContext dials addr through the bastion. network is "tcp".
func (t *Tunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.client.DialContext(ctx, network, addr)
}

// Healthy reports whether the last keepalive round-trip succeeded.
func (t *Tunnel) Healthy() bool { return t.healthy.Load() }

// Close stops keepalives and closes the SSH client. Safe to call repeatedly.
func (t *Tunnel) Close() error {
	var err error
	t.once.Do(func() {
		close(t.done)
		t.healthy.Store(false)
		err = t.client.Close()
	})
	return err
}

func (t *Tunnel) keepalive() {
	ticker := time.NewTicker(sshKeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			_, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				t.healthy.Store(false)
				_ = t.client.Close()
				return
			}
		}
	}
}

func sshAuthMethods(cfg SSHConfig) ([]ssh.AuthMethod, error) {
	switch cfg.AuthMethod {
	case SSHAuthPassword:
		if cfg.Password == "" {
			return nil, errors.New("ssh tunnel: password required for password auth")
		}
		return []ssh.AuthMethod{ssh.Password(cfg.Password)}, nil
	case SSHAuthPrivateKey:
		if cfg.PrivateKeyPEM == "" {
			return nil, errors.New("ssh tunnel: private key required for key auth")
		}
		var signer ssh.Signer
		var err error
		if cfg.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cfg.PrivateKeyPEM), []byte(cfg.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(cfg.PrivateKeyPEM))
		}
		if err != nil {
			return nil, fmt.Errorf("ssh tunnel: parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("ssh tunnel: unknown auth method %q", cfg.AuthMethod)
	}
}

func sshHostKeyCallback(cfg SSHConfig) (ssh.HostKeyCallback, error) {
	switch {
	case cfg.KnownHostsEntry != "":
		_, _, pub, _, _, err := ssh.ParseKnownHosts([]byte(cfg.KnownHostsEntry))
		if err != nil {
			return nil, fmt.Errorf("ssh tunnel: parse known_hosts entry: %w", err)
		}
		want := pub.Marshal()
		return func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if subtle.ConstantTimeCompare(key.Marshal(), want) == 1 {
				return nil
			}
			return errors.New("ssh tunnel: host key does not match pinned known_hosts entry")
		}, nil
	case cfg.Fingerprint != "":
		want := cfg.Fingerprint
		return func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if subtle.ConstantTimeCompare([]byte(ssh.FingerprintSHA256(key)), []byte(want)) == 1 {
				return nil
			}
			return errors.New("ssh tunnel: host key does not match pinned fingerprint")
		}, nil
	case cfg.InsecureSkipHostKey:
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit operator opt-out; never the default
	default:
		return nil, errors.New("ssh tunnel: host key verification material required (known_hosts entry or fingerprint), or explicit insecure opt-out")
	}
}
