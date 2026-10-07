package identity

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

// Presented is the raw credential material and request attributes an
// authenticator inspects.
type Presented struct {
	Authorization string
	Request       access.RequestAttributes
}

// Authenticated is a verified principal plus the records loaded to verify it.
type Authenticated struct {
	Principal   access.Principal
	Account     database.Account
	AuthSession *database.AuthSession
}

// Authenticator returns ok=false when the credential is not in its format,
// and a non-nil error when the credential is its format but not valid.
type Authenticator interface {
	Authenticate(ctx context.Context, p Presented) (Authenticated, bool, error)
}

// PostureProvider collects request posture signals after authentication.
type PostureProvider interface {
	Collect(ctx context.Context, p Presented, s *access.Signals) error
}

// RequestPolicy evaluates an authenticated request principal.
type RequestPolicy interface {
	Evaluate(ctx context.Context, p access.Principal) access.Decision
}

// CredentialError rejects a credential. Reason is a stable code safe to
// return to clients and to write to audit.
type CredentialError struct {
	Reason string
}

func (e *CredentialError) Error() string { return "credential rejected: " + e.Reason }

// ErrNoAuthenticator indicates that no authenticator recognized a credential.
var ErrNoAuthenticator = &CredentialError{Reason: "credential_unrecognized"}

// Chain authenticates with the first matching authenticator, then collects
// posture and evaluates request policies.
type Chain struct {
	Authenticators []Authenticator
	Posture        []PostureProvider
	Policies       []RequestPolicy
}

// Authenticate runs the authentication chain for presented credentials.
func (c Chain) Authenticate(ctx context.Context, p Presented) (Authenticated, error) {
	for _, a := range c.Authenticators {
		result, ok, err := a.Authenticate(ctx, p)
		if !ok {
			continue
		}
		if err != nil {
			return Authenticated{}, err
		}
		result.Principal.Request = p.Request
		for _, provider := range c.Posture {
			if err := provider.Collect(ctx, p, &result.Principal.Request.Signals); err != nil {
				return Authenticated{}, err
			}
		}
		for _, policy := range c.Policies {
			if d := policy.Evaluate(ctx, result.Principal); d.Effect != access.EffectAllow {
				reason := d.Reason
				if reason == "" {
					reason = "request_policy_denied"
				}
				return Authenticated{}, &CredentialError{Reason: reason}
			}
		}
		return result, nil
	}
	return Authenticated{}, ErrNoAuthenticator
}
