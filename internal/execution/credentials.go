package execution

import (
	"context"
	"errors"
	"log/slog"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

var (
	// ErrCredentialsNotFound means the requested connection has no credential
	// record in the provider.
	ErrCredentialsNotFound = errors.New("connection credentials not found")
	// ErrCredentialDecryption means stored credential material could not be
	// decrypted. Provider errors deliberately omit ciphertext and plaintext.
	ErrCredentialDecryption = errors.New("connection credentials could not be decrypted")
	// ErrCredentialsInvalid means decrypted structured credential material is
	// malformed.
	ErrCredentialsInvalid = errors.New("connection credentials are invalid")
	// ErrCredentialSerialization prevents plaintext material from crossing the
	// execution transport by accident.
	ErrCredentialSerialization = errors.New("connection credentials cannot be serialized")
	// ErrTargetRejected means connector-side target policy refused the resolved
	// driver and DSN.
	ErrTargetRejected = errors.New("target database is blocked by policy")
	// ErrTargetConnection hides driver errors that may embed credential-bearing
	// connection strings.
	ErrTargetConnection = errors.New("could not connect to target database")
	// ErrProtocolMismatch prevents routing requests to an incompatible runtime
	// contract during rolling deployments.
	ErrProtocolMismatch = errors.New("execution runtime protocol mismatch")
	// ErrSQLiteTargetDisabled and ErrSQLiteInMemoryTargetDisabled cross the
	// connector boundary without exposing the rejected DSN.
	ErrSQLiteTargetDisabled         = errors.New("sqlite file target connections are disabled for this instance")
	ErrSQLiteInMemoryTargetDisabled = errors.New("sqlite in-memory target connections are disabled for this instance")
)

// Credentials is the connector-local material needed to open a target
// database. It must never be serialized into an execution request or response.
type Credentials struct {
	Driver       string
	DSN          string
	DefaultScope metadata.ScopePath
	TLS          *engine.TLSConfig
	SSH          *SSHConfig
}

// MarshalJSON makes accidental placement of Credentials in a transport DTO a
// hard failure instead of emitting plaintext.
func (Credentials) MarshalJSON() ([]byte, error) {
	return nil, ErrCredentialSerialization
}

// String redacts credential material from fmt verbs such as %v and %+v.
func (c Credentials) String() string {
	return "execution.Credentials{driver:" + c.Driver + " [redacted]}"
}

// GoString redacts credential material from the %#v verb.
func (c Credentials) GoString() string { return c.String() }

// LogValue redacts credential material from structured logs.
func (c Credentials) LogValue() slog.Value { return slog.StringValue(c.String()) }

// CredentialProvider resolves connector-local target credentials by catalog
// connection ID. Implementations must not include secret material in errors or
// logs.
type CredentialProvider interface {
	Resolve(ctx context.Context, connectionID string) (Credentials, error)
}

// TargetValidator applies deployment policy to resolved target coordinates.
// It runs beside credential resolution so API-only processes never need the
// plaintext DSN. Policy refusals must match ErrTargetRejected,
// ErrSQLiteTargetDisabled, or ErrSQLiteInMemoryTargetDisabled; any other error
// is an internal failure.
type TargetValidator interface {
	Validate(ctx context.Context, driver, dsn string) error
}
