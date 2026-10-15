package credentials

import (
	"context"
	"errors"
	"log/slog"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine"
)

// ErrNotFound reports that no connection matches the ref, including refs whose
// workspace or organization does not own the connection.
var ErrNotFound = errors.New("credentials: connection not found")

// ErrSerialization is returned when Credentials is marshaled; secrets must
// never enter transport payloads.
var ErrSerialization = errors.New("credentials: refusing to serialize credentials")

// SSHConfig is the decoded SSH tunnel material for a connection.
type SSHConfig = connection.SSHConfig

// SSHAuthMethod selects how the tunnel authenticates to the bastion.
type SSHAuthMethod = connection.SSHAuthMethod

const (
	SSHAuthPassword   = connection.SSHAuthPassword
	SSHAuthPrivateKey = connection.SSHAuthPrivateKey
)

// ConnectionRef identifies a connection by its owning hierarchy. IDs are
// strings so providers stay independent of the metadata database's keys.
type ConnectionRef struct {
	OrgID        string
	WorkspaceID  string
	ConnectionID string
}

// Credentials is everything needed to open a target session.
type Credentials struct {
	Driver       string
	DefaultScope string
	// DSN is the finished legacy connection string, ready to hand to the
	// driver, with any password already embedded.
	DSN string
	SSH *SSHConfig
	// TLS holds client key material; Credentials redacts it like every other
	// secret field.
	TLS *engine.TLSConfig
}

const redacted = "credentials.Credentials{[redacted]}"

// MarshalJSON always fails so decoded secrets cannot be serialized.
func (Credentials) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }

// String redacts secrets from fmt verbs such as %v and %+v.
func (Credentials) String() string { return redacted }

// GoString redacts secrets from the %#v verb.
func (c Credentials) GoString() string { return c.String() }

// LogValue redacts secrets from structured logs.
func (c Credentials) LogValue() slog.Value { return slog.StringValue(c.String()) }

// Provider resolves the credentials for a connection.
type Provider interface {
	Resolve(ctx context.Context, ref ConnectionRef) (Credentials, error)
}
