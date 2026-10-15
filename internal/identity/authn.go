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

// Account is a stored user account.
type Account = database.Account

// AuthSession is a stored sign-in session.
type AuthSession = database.AuthSession

// SessionAuth records how a session was authenticated.
type SessionAuth = database.SessionAuth

// Organization is an organization an account belongs to.
type Organization = database.Organization

// Authenticated is a verified principal plus the records loaded to verify it.
type Authenticated struct {
	Principal   access.Principal
	Account     Account
	AuthSession *AuthSession
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

	// AccountID and CredentialID are for logs only and are never returned to clients.
	AccountID    int64
	CredentialID string
}

func (e *CredentialError) Error() string { return "credential rejected: " + e.Reason }

// ErrNoAuthenticator indicates that no authenticator recognized a credential.
var ErrNoAuthenticator = &CredentialError{Reason: "credential_unrecognized"}

// InstanceAdminLookup reports whether an account administers the instance.
type InstanceAdminLookup interface {
	IsInstanceAdmin(ctx context.Context, accountID int64) (bool, error)
}

// Chain authenticates with the first matching authenticator, then resolves
// the instance admin flag, collects posture and evaluates request policies.
type Chain struct {
	Authenticators []Authenticator
	// InstanceAdmins is the only source of Principal.InstanceAdmin. A nil
	// lookup leaves every principal without instance admin rights.
	InstanceAdmins InstanceAdminLookup
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
		// Authenticators never decide instance admin rights; the flag is
		// looked up per request so a grant or removal applies immediately.
		result.Principal.InstanceAdmin = false
		if c.InstanceAdmins != nil && result.Principal.Subject.Kind == access.SubjectAccount && result.Principal.Subject.ID != 0 {
			admin, err := c.InstanceAdmins.IsInstanceAdmin(ctx, result.Principal.Subject.ID)
			if err != nil {
				return Authenticated{}, err
			}
			result.Principal.InstanceAdmin = admin
		}
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
