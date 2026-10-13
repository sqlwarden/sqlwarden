package identity

import (
	"context"
	"errors"
	"time"

	"github.com/sqlwarden/internal/access"
)

// ErrAuthSessionRequired rejects organization entry while session revocation
// is on and the caller authenticated without an auth session, because there
// is nothing to bind an organization access session to.
var ErrAuthSessionRequired = errors.New("identity: organization access requires an auth session")

// EnterOrgInput identifies the organization a request addresses and the auth
// session the request authenticated with.
type EnterOrgInput struct {
	Slug string
	// RevocationEnabled binds the entry to an organization access session
	// that can be revoked independently of the auth session.
	RevocationEnabled    bool
	AuthSessionID        string
	AuthSessionExpiresAt time.Time
}

// OrgEntry is the organization a caller entered.
type OrgEntry struct {
	ID   int64
	Slug string
}

// OrgAccessRejection is an ErrForbidden caused by a missing, revoked or
// expired organization access session. Reason is safe to log.
type OrgAccessRejection struct {
	Reason        string
	AccountID     int64
	OrgID         int64
	AuthSessionID string
}

func (e *OrgAccessRejection) Error() string { return ErrForbidden.Error() }
func (e *OrgAccessRejection) Unwrap() error { return ErrForbidden }

// EnterOrg resolves an organization slug for a member. An unknown slug is
// ErrNotFound and a non-member is ErrForbidden, in that order, so the slug
// lookup itself reveals nothing a non-member could not learn from the 403.
// With revocation on, it ensures the organization access session for the
// auth session and rejects one that is revoked or expired.
func (s *Service) EnterOrg(ctx context.Context, p access.Principal, in EnterOrgInput) (OrgEntry, error) {
	if err := requireAccount(p); err != nil {
		return OrgEntry{}, err
	}
	org, found, err := s.store.GetOrgBySlug(ctx, in.Slug)
	if err != nil {
		return OrgEntry{}, err
	}
	if !found {
		return OrgEntry{}, ErrNotFound
	}
	accountID := p.Subject.ID
	member, err := s.store.IsOrgMember(ctx, org.ID, accountID)
	if err != nil {
		return OrgEntry{}, err
	}
	if !member {
		return OrgEntry{}, ErrForbidden
	}
	entry := OrgEntry{ID: org.ID, Slug: org.Slug}
	if !in.RevocationEnabled {
		return entry, nil
	}
	if in.AuthSessionID == "" {
		return OrgEntry{}, ErrAuthSessionRequired
	}
	if err := s.store.EnsureOrgAccessSession(ctx, in.AuthSessionID, org.ID, accountID, in.AuthSessionExpiresAt); err != nil {
		return OrgEntry{}, err
	}
	session, found, err := s.store.GetOrgAccessSession(ctx, in.AuthSessionID, org.ID, accountID)
	if err != nil {
		return OrgEntry{}, err
	}
	reason := ""
	switch {
	case !found:
		reason = "org_access_session_not_found"
	case session.RevokedAt != nil:
		reason = "org_access_session_revoked"
	case s.now().After(session.ExpiresAt):
		reason = "org_access_session_expired"
	}
	if reason != "" {
		return OrgEntry{}, &OrgAccessRejection{Reason: reason, AccountID: accountID, OrgID: org.ID, AuthSessionID: in.AuthSessionID}
	}
	return entry, nil
}
