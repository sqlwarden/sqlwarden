// Package identity defines authentication contracts shared by core and
// edition-specific identity implementations.
package identity

import "context"

// AuthenticationMethod identifies the credential protocol used for a login.
type AuthenticationMethod string

// Authentication methods supported by core or edition identity providers.
const (
	AuthenticationPassword AuthenticationMethod = "password"
	AuthenticationOIDC     AuthenticationMethod = "oidc"
	AuthenticationSAML     AuthenticationMethod = "saml"
)

// AuthenticationRequest is the provider-neutral input to authentication.
// Secret values must never be included in errors, logs, or audit metadata.
type AuthenticationRequest struct {
	Method     AuthenticationMethod
	Identifier string
	Secret     string
	Attributes map[string]string
}

// Subject is an authenticated SQLWarden identity.
type Subject struct {
	AccountID  int64
	Email      string
	Name       string
	Attributes map[string]string
}

// Provider authenticates one request and returns its stable subject. Account
// provisioning and directory synchronization remain explicit application use
// cases rather than side effects hidden in this interface.
type Provider interface {
	Authenticate(ctx context.Context, request AuthenticationRequest) (Subject, error)
}

// ProviderFunc adapts a function to [Provider].
type ProviderFunc func(context.Context, AuthenticationRequest) (Subject, error)

// Authenticate implements [Provider].
func (f ProviderFunc) Authenticate(ctx context.Context, request AuthenticationRequest) (Subject, error) {
	return f(ctx, request)
}
