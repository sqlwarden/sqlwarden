package execution

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
)

// ClientTransportCredentials configure the HTTP client used by WorkerRuntime.
// Implementations may provide plaintext transport for development or TLS and
// workload credentials for deployment without changing the runtime protocol.
type ClientTransportCredentials interface {
	Scheme() string
	HTTPClient() *http.Client
}

// ServerTransportCredentials configure how the connector serves internal HTTP.
type ServerTransportCredentials interface {
	Serve(*http.Server, net.Listener) error
}

// InsecureTransportCredentials use plaintext HTTP. They are intended for
// trusted development networks and the initial single-connector deployment.
type InsecureTransportCredentials struct {
	Client *http.Client
}

// Scheme returns the plaintext HTTP scheme.
func (InsecureTransportCredentials) Scheme() string { return "http" }

// HTTPClient returns the configured client or a default transport client.
func (c InsecureTransportCredentials) HTTPClient() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{}
}

// Serve starts a plaintext HTTP server.
func (InsecureTransportCredentials) Serve(server *http.Server, listener net.Listener) error {
	return server.Serve(listener)
}

// TLSClientTransportCredentials use a caller-owned TLS configuration. The
// configuration may include roots, server name, and a workload certificate.
type TLSClientTransportCredentials struct {
	Config *tls.Config
	Client *http.Client
}

// Scheme returns the HTTPS scheme.
func (TLSClientTransportCredentials) Scheme() string { return "https" }

// HTTPClient returns a client whose transport uses a clone of the TLS config.
func (c TLSClientTransportCredentials) HTTPClient() *http.Client {
	client := c.Client
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if existing, ok := client.Transport.(*http.Transport); ok {
		transport = existing.Clone()
	}
	if c.Config != nil {
		transport.TLSClientConfig = c.Config.Clone()
	}
	clone.Transport = transport
	return &clone
}

// TLSServerTransportCredentials serve internal HTTP over TLS. ClientAuth and
// ClientCAs on Config enable mTLS without changing RuntimeServer.
type TLSServerTransportCredentials struct {
	Config *tls.Config
}

// Serve starts a TLS-protected HTTP server.
func (c TLSServerTransportCredentials) Serve(server *http.Server, listener net.Listener) error {
	if c.Config == nil || len(c.Config.Certificates) == 0 {
		return errors.New("execution TLS server credentials require a certificate")
	}
	return server.Serve(tls.NewListener(listener, c.Config.Clone()))
}

var (
	_ ClientTransportCredentials = InsecureTransportCredentials{}
	_ ClientTransportCredentials = TLSClientTransportCredentials{}
	_ ServerTransportCredentials = InsecureTransportCredentials{}
	_ ServerTransportCredentials = TLSServerTransportCredentials{}
)
