package execution

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTLSClientTransportCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	credentials := TLSClientTransportCredentials{Config: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}}
	if credentials.Scheme() != "https" {
		t.Fatalf("Scheme() = %q, want https", credentials.Scheme())
	}
	response, err := credentials.HTTPClient().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestTLSServerTransportCredentialsRequiresCertificate(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	credentials := TLSServerTransportCredentials{Config: &tls.Config{MinVersion: tls.VersionTLS12}}
	if err := credentials.Serve(&http.Server{}, listener); err == nil {
		t.Fatal("expected TLS server credentials without a certificate to fail")
	}
}
