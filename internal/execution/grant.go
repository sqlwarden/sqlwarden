package execution

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const defaultGrantTTL = time.Minute

// GrantAuthority issues and validates short-lived execution grants shared by
// trusted API and connector processes. Revocation generations are signed into
// the wire shape now even though enforcement is deferred.
type GrantAuthority struct {
	key      []byte
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time
}

// NewGrantAuthority returns an HMAC-backed grant authority. The signing key,
// issuer, and audience must be identical on API and connector processes.
func NewGrantAuthority(key []byte, issuer, audience string, ttl time.Duration) (*GrantAuthority, error) {
	if len(key) < 32 {
		return nil, errors.New("execution grant signing key must be at least 32 bytes")
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("execution grant issuer and audience are required")
	}
	if ttl <= 0 {
		ttl = defaultGrantTTL
	}
	return &GrantAuthority{
		key: append([]byte(nil), key...), issuer: issuer, audience: audience,
		ttl: ttl, now: time.Now,
	}, nil
}

// Issue creates a signed grant for one resource scope and permission set.
func (a *GrantAuthority) Issue(scope Scope, permissions []string, revocationGeneration uint64) (Grant, error) {
	now := a.now().UTC()
	id, err := grantID()
	if err != nil {
		return Grant{}, err
	}
	grant := Grant{
		ID: id, Issuer: a.issuer, Audience: a.audience, Scope: scope,
		Permissions: append([]string(nil), permissions...), IssuedAt: now,
		NotBefore: now, ExpiresAt: now.Add(a.ttl),
		RevocationGeneration: revocationGeneration,
	}
	grant.Signature, err = a.signature(grant)
	if err != nil {
		return Grant{}, err
	}
	return grant, nil
}

// Validate authenticates a grant, checks its validity window, and requires
// its signed scope to exactly match the operation scope.
func (a *GrantAuthority) Validate(_ context.Context, grant Grant, expected Scope) error {
	now := a.now().UTC()
	if grant.ID == "" || grant.Issuer != a.issuer || grant.Audience != a.audience || grant.Scope != expected {
		return grantInvalid(nil)
	}
	if grant.IssuedAt.IsZero() || grant.NotBefore.IsZero() || grant.ExpiresAt.IsZero() ||
		now.Before(grant.NotBefore) || !now.Before(grant.ExpiresAt) || grant.ExpiresAt.Before(grant.IssuedAt) {
		return grantInvalid(nil)
	}
	want, err := a.signature(grant)
	if err != nil {
		return grantInvalid(err)
	}
	provided, err := base64.RawURLEncoding.DecodeString(grant.Signature)
	if err != nil || !hmac.Equal(provided, mustDecodeSignature(want)) {
		return grantInvalid(err)
	}
	return nil
}

// ValidatePermission additionally requires the signed grant to authorize the
// specific internal operation being dispatched.
func (a *GrantAuthority) ValidatePermission(ctx context.Context, grant Grant, expected Scope, permission string) error {
	if err := a.Validate(ctx, grant, expected); err != nil {
		return err
	}
	for _, granted := range grant.Permissions {
		if granted == permission {
			return nil
		}
	}
	return grantInvalid(nil)
}

func (a *GrantAuthority) signature(grant Grant) (string, error) {
	grant.Signature = ""
	payload, err := json.Marshal(grant)
	if err != nil {
		return "", fmt.Errorf("marshal execution grant: %w", err)
	}
	mac := hmac.New(sha256.New, a.key)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func mustDecodeSignature(signature string) []byte {
	decoded, _ := base64.RawURLEncoding.DecodeString(signature)
	return decoded
}

func grantID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate execution grant id: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func grantInvalid(err error) error {
	if err == nil {
		err = ErrGrantInvalid
	} else {
		err = errors.Join(ErrGrantInvalid, err)
	}
	return &Failure{Code: FailureGrantInvalid, Err: err}
}
