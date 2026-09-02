package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func genTestCA(t *testing.T) (caPEM, certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemStr := func(typ string, b []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}))
	}
	ca := pemStr("CERTIFICATE", der)
	return ca, ca, pemStr("PRIVATE KEY", keyDER)
}

func TestTLSConfigBuild(t *testing.T) {
	caPEM, _, _ := genTestCA(t)

	t.Run("nil and disable return nil config", func(t *testing.T) {
		var c *TLSConfig
		got, err := c.Build()
		if err != nil || got != nil {
			t.Fatalf("nil: got (%v,%v), want (nil,nil)", got, err)
		}
		got, err = (&TLSConfig{Mode: TLSModeDisable}).Build()
		if err != nil || got != nil {
			t.Fatalf("disable: got (%v,%v), want (nil,nil)", got, err)
		}
	})

	t.Run("require sets InsecureSkipVerify", func(t *testing.T) {
		got, err := (&TLSConfig{Mode: TLSModeRequire}).Build()
		if err != nil {
			t.Fatal(err)
		}
		if !got.InsecureSkipVerify {
			t.Fatal("require: want InsecureSkipVerify=true")
		}
		if got.VerifyPeerCertificate != nil {
			t.Fatal("require: want no VerifyPeerCertificate callback")
		}
	})

	t.Run("verify-ca skips stdlib verify but sets chain callback", func(t *testing.T) {
		got, err := (&TLSConfig{Mode: TLSModeVerifyCA, CAPEM: caPEM}).Build()
		if err != nil {
			t.Fatal(err)
		}
		if !got.InsecureSkipVerify || got.VerifyPeerCertificate == nil {
			t.Fatalf("verify-ca: want skip-verify + callback, got skip=%v cb=%v", got.InsecureSkipVerify, got.VerifyPeerCertificate != nil)
		}
	})

	t.Run("verify-full keeps stdlib verification and server name", func(t *testing.T) {
		got, err := (&TLSConfig{Mode: TLSModeVerifyFull, CAPEM: caPEM, ServerName: "db.internal"}).Build()
		if err != nil {
			t.Fatal(err)
		}
		if got.InsecureSkipVerify {
			t.Fatal("verify-full: want InsecureSkipVerify=false")
		}
		if got.ServerName != "db.internal" {
			t.Fatalf("verify-full: ServerName=%q", got.ServerName)
		}
		if got.RootCAs == nil {
			t.Fatal("verify-full: want RootCAs populated")
		}
	})

	t.Run("bad CA PEM errors", func(t *testing.T) {
		_, err := (&TLSConfig{Mode: TLSModeVerifyFull, CAPEM: "not a pem"}).Build()
		if err == nil || !strings.Contains(err.Error(), "ca") {
			t.Fatalf("want CA parse error, got %v", err)
		}
	})

	t.Run("client cert without key errors and vice versa", func(t *testing.T) {
		_, certPEM, keyPEM := genTestCA(t)
		if _, err := (&TLSConfig{Mode: TLSModeRequire, ClientCertPEM: certPEM}).Build(); err == nil {
			t.Fatal("cert-without-key: want error")
		}
		if _, err := (&TLSConfig{Mode: TLSModeRequire, ClientKeyPEM: keyPEM}).Build(); err == nil {
			t.Fatal("key-without-cert: want error")
		}
		got, err := (&TLSConfig{Mode: TLSModeRequire, ClientCertPEM: certPEM, ClientKeyPEM: keyPEM}).Build()
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Certificates) != 1 {
			t.Fatalf("want 1 client cert, got %d", len(got.Certificates))
		}
	})
}

// testPKI is a throwaway CA used to sign server and client leaf certificates
// for the live-handshake tests below.
type testPKI struct {
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  string
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "handshake-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testPKI{
		caCert: caCert,
		caKey:  key,
		caPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

func (p *testPKI) leaf(t *testing.T, serial int64, cn string, hosts []string, eku x509.ExtKeyUsage, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{eku},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.caCert, &key.PublicKey, p.caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

// startTLSServer runs an in-process TLS listener that completes the handshake
// and hangs up. When clientCAs is non-nil the server demands a client
// certificate that chains to it.
func startTLSServer(t *testing.T, certPEM, keyPEM string, clientCAs *x509.CertPool) string {
	t.Helper()
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	if clientCAs != nil {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = clientCAs
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				if tc, ok := conn.(*tls.Conn); ok {
					if tc.Handshake() == nil {
						// A byte the client can read back confirms the server
						// accepted the handshake; on a server-side rejection
						// (e.g. missing client cert) the client's Handshake
						// returns before the alert arrives, so the read is what
						// surfaces the failure.
						_, _ = tc.Write([]byte{1})
					}
				}
				_ = conn.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

func handshake(t *testing.T, addr string, tc *TLSConfig) error {
	t.Helper()
	cfg, err := tc.Build()
	if err != nil {
		return err
	}
	raw, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	client := tls.Client(raw, cfg)
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if err := client.Handshake(); err != nil {
		return err
	}
	_, err = client.Read(make([]byte, 1))
	return err
}

func TestTLSConfigHandshake(t *testing.T) {
	pki := newTestPKI(t)
	other := newTestPKI(t)
	future := time.Now().Add(time.Hour)

	goodCert, goodKey := pki.leaf(t, 2, "127.0.0.1", []string{"127.0.0.1", "localhost"}, x509.ExtKeyUsageServerAuth, future)
	goodAddr := startTLSServer(t, goodCert, goodKey, nil)

	t.Run("require connects without verifying the chain", func(t *testing.T) {
		// No CA supplied; the server leaf chains to a CA the client does not
		// know. require must still succeed.
		if err := handshake(t, goodAddr, &TLSConfig{Mode: TLSModeRequire}); err != nil {
			t.Fatalf("require handshake failed: %v", err)
		}
	})

	t.Run("verify-ca accepts a chain to the supplied CA and ignores the hostname", func(t *testing.T) {
		err := handshake(t, goodAddr, &TLSConfig{
			Mode: TLSModeVerifyCA, CAPEM: pki.caPEM, ServerName: "wrong.example",
		})
		if err != nil {
			t.Fatalf("verify-ca handshake failed: %v", err)
		}
	})

	t.Run("verify-ca rejects a chain to a different CA", func(t *testing.T) {
		err := handshake(t, goodAddr, &TLSConfig{
			Mode: TLSModeVerifyCA, CAPEM: other.caPEM, ServerName: "127.0.0.1",
		})
		if err == nil {
			t.Fatal("verify-ca: want handshake failure for an unknown CA")
		}
	})

	t.Run("verify-ca rejects an expired server certificate", func(t *testing.T) {
		expCert, expKey := pki.leaf(t, 3, "127.0.0.1", []string{"127.0.0.1"}, x509.ExtKeyUsageServerAuth, time.Now().Add(-time.Minute))
		expAddr := startTLSServer(t, expCert, expKey, nil)
		err := handshake(t, expAddr, &TLSConfig{
			Mode: TLSModeVerifyCA, CAPEM: pki.caPEM, ServerName: "127.0.0.1",
		})
		if err == nil {
			t.Fatal("verify-ca: want handshake failure for an expired certificate")
		}
	})

	t.Run("verify-full accepts a matching CA and hostname", func(t *testing.T) {
		err := handshake(t, goodAddr, &TLSConfig{
			Mode: TLSModeVerifyFull, CAPEM: pki.caPEM, ServerName: "127.0.0.1",
		})
		if err != nil {
			t.Fatalf("verify-full handshake failed: %v", err)
		}
	})

	t.Run("verify-full rejects a hostname mismatch", func(t *testing.T) {
		err := handshake(t, goodAddr, &TLSConfig{
			Mode: TLSModeVerifyFull, CAPEM: pki.caPEM, ServerName: "wrong.example",
		})
		if err == nil {
			t.Fatal("verify-full: want handshake failure for a hostname mismatch")
		}
	})

	t.Run("verify-full rejects an unknown CA", func(t *testing.T) {
		err := handshake(t, goodAddr, &TLSConfig{
			Mode: TLSModeVerifyFull, CAPEM: other.caPEM, ServerName: "127.0.0.1",
		})
		if err == nil {
			t.Fatal("verify-full: want handshake failure for an unknown CA")
		}
	})

	t.Run("client certificate satisfies a server that requires mTLS", func(t *testing.T) {
		clientPEM, clientKeyPEM := pki.leaf(t, 4, "handshake-test-client", nil, x509.ExtKeyUsageClientAuth, future)
		clientCAs := x509.NewCertPool()
		if !clientCAs.AppendCertsFromPEM([]byte(pki.caPEM)) {
			t.Fatal("failed to build client CA pool")
		}
		mtlsAddr := startTLSServer(t, goodCert, goodKey, clientCAs)

		if err := handshake(t, mtlsAddr, &TLSConfig{
			Mode: TLSModeVerifyFull, CAPEM: pki.caPEM, ServerName: "127.0.0.1",
			ClientCertPEM: clientPEM, ClientKeyPEM: clientKeyPEM,
		}); err != nil {
			t.Fatalf("mTLS handshake with a client certificate failed: %v", err)
		}

		if err := handshake(t, mtlsAddr, &TLSConfig{
			Mode: TLSModeVerifyFull, CAPEM: pki.caPEM, ServerName: "127.0.0.1",
		}); err == nil {
			t.Fatal("mTLS: want handshake failure when the client sends no certificate")
		}
	})
}

func TestValidTLSMode(t *testing.T) {
	for _, m := range []TLSMode{TLSModeDisable, TLSModeRequire, TLSModeVerifyCA, TLSModeVerifyFull} {
		if !ValidTLSMode(m) {
			t.Fatalf("want %q valid", m)
		}
	}
	if ValidTLSMode("bogus") {
		t.Fatal("want bogus invalid")
	}
}
