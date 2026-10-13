package identity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
)

func orgSessionKey(authSessionID string, orgID, accountID int64) string {
	return fmt.Sprintf("%s/%d/%d", authSessionID, orgID, accountID)
}

func (s *fakeStore) EnsureOrgAccessSession(_ context.Context, authSessionID string, orgID, accountID int64, expiresAt time.Time) error {
	key := orgSessionKey(authSessionID, orgID, accountID)
	if _, ok := s.orgSessions[key]; ok {
		return nil
	}
	s.orgSessions[key] = database.OrgAccessSession{ID: key, AuthSessionID: authSessionID, OrgID: orgID, AccountID: accountID, CreatedAt: s.now(), LastSeenAt: s.now(), ExpiresAt: expiresAt}
	return nil
}

func (s *fakeStore) GetOrgAccessSession(_ context.Context, authSessionID string, orgID, accountID int64) (database.OrgAccessSession, bool, error) {
	sess, ok := s.orgSessions[orgSessionKey(authSessionID, orgID, accountID)]
	return sess, ok, nil
}

func newOrgHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.store.orgs["acme"] = database.Organization{ID: testOrg, Slug: "acme", Name: "Acme"}
	h.store.orgs["other"] = database.Organization{ID: missingOrg, Slug: "other", Name: "Other"}
	return h
}

func TestEnterOrgResolvesMemberWithoutSessionWhenRevocationOff(t *testing.T) {
	h := newOrgHarness(t)
	entry, err := h.svc.EnterOrg(context.Background(), account(alice), identity.EnterOrgInput{Slug: "acme"})
	if err != nil || entry.ID != testOrg || entry.Slug != "acme" {
		t.Fatalf("entry = %+v, err = %v", entry, err)
	}
	if len(h.store.orgSessions) != 0 {
		t.Fatalf("org access sessions created with revocation off: %d", len(h.store.orgSessions))
	}
}

func TestEnterOrgDenials(t *testing.T) {
	h := newOrgHarness(t)
	ctx := context.Background()
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.EnterOrg(ctx, account(alice), identity.EnterOrgInput{Slug: "missing"})), "unknown slug")
	accesstest.AssertForbidden(t, accesstest.Err(h.svc.EnterOrg(ctx, account(alice), identity.EnterOrgInput{Slug: "other"})), "non-member")
	accesstest.AssertForbidden(t, accesstest.Err(h.svc.EnterOrg(ctx, account(0), identity.EnterOrgInput{Slug: "acme"})), "anonymous")
	if _, err := h.svc.EnterOrg(ctx, account(alice), identity.EnterOrgInput{Slug: "acme", RevocationEnabled: true}); !errors.Is(err, identity.ErrAuthSessionRequired) {
		t.Fatalf("missing auth session err = %v", err)
	}
}

func TestEnterOrgBindsAndChecksAccessSession(t *testing.T) {
	h := newOrgHarness(t)
	ctx := context.Background()
	in := identity.EnterOrgInput{Slug: "acme", RevocationEnabled: true, AuthSessionID: "sess-1", AuthSessionExpiresAt: h.now.Add(time.Hour)}
	if _, err := h.svc.EnterOrg(ctx, account(alice), in); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if len(h.store.orgSessions) != 1 {
		t.Fatalf("org access sessions = %d", len(h.store.orgSessions))
	}

	key := orgSessionKey("sess-1", testOrg, alice)
	sess := h.store.orgSessions[key]
	revokedAt := h.now
	sess.RevokedAt = &revokedAt
	h.store.orgSessions[key] = sess
	_, err := h.svc.EnterOrg(ctx, account(alice), in)
	var rejection *identity.OrgAccessRejection
	if !errors.As(err, &rejection) || rejection.Reason != "org_access_session_revoked" || !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("revoked err = %v", err)
	}

	sess.RevokedAt = nil
	sess.ExpiresAt = h.now.Add(-time.Minute)
	h.store.orgSessions[key] = sess
	_, err = h.svc.EnterOrg(ctx, account(alice), in)
	if !errors.As(err, &rejection) || rejection.Reason != "org_access_session_expired" || rejection.OrgID != testOrg || rejection.AccountID != alice {
		t.Fatalf("expired err = %v", err)
	}
}
