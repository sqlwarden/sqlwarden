package identity

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/token"
)

// SessionStore loads accounts and authentication sessions.
type SessionStore interface {
	GetAccount(ctx context.Context, id int64) (database.Account, bool, error)
	GetAuthSession(ctx context.Context, id string, accountID int64) (database.AuthSession, bool, error)
	TouchAuthSession(ctx context.Context, id string) error
}

// RevocationSetting reports whether auth sessions are checked on each request.
type RevocationSetting func(ctx context.Context) bool

type sessionAuthenticator struct {
	store      SessionStore
	secret     string
	revocation RevocationSetting
	now        func() time.Time
}

// NewSessionAuthenticator creates an authenticator for bearer session tokens.
func NewSessionAuthenticator(store SessionStore, secret string, revocation RevocationSetting, now func() time.Time) Authenticator {
	return sessionAuthenticator{store: store, secret: secret, revocation: revocation, now: now}
}

func (a sessionAuthenticator) Authenticate(ctx context.Context, p Presented) (Authenticated, bool, error) {
	raw, ok := strings.CutPrefix(p.Authorization, "Bearer ")
	if !ok || raw == "" || strings.Contains(raw, " ") {
		return Authenticated{}, false, nil
	}
	claims, err := token.Verify(raw, a.secret)
	if err != nil {
		return Authenticated{}, true, &CredentialError{Reason: "token_invalid"}
	}
	reject := func(reason string, accountID int64) error {
		return &CredentialError{Reason: reason, AccountID: accountID, CredentialID: claims.AuthSessionID}
	}
	revocation := a.revocation(ctx)
	if revocation && claims.AuthSessionID == "" {
		return Authenticated{}, true, reject("session_binding_missing", 0)
	}
	accountID, err := strconv.ParseInt(claims.AccountID, 10, 64)
	if err != nil {
		return Authenticated{}, true, reject("token_invalid", 0)
	}
	account, found, err := a.store.GetAccount(ctx, accountID)
	if err != nil {
		return Authenticated{}, true, err
	}
	if !found {
		return Authenticated{}, true, reject("account_not_found", accountID)
	}
	if !account.IsActive {
		return Authenticated{}, true, reject("account_inactive", account.ID)
	}

	result := Authenticated{
		Account: account,
		Principal: access.Principal{
			Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: account.ID},
			Credential: access.CredentialInfo{
				Kind: access.CredentialSession, ID: claims.AuthSessionID,
				Method: MethodPassword, Assurance: access.AAL1,
			},
		},
	}
	if !revocation {
		return result, true, nil
	}
	session, found, err := a.store.GetAuthSession(ctx, claims.AuthSessionID, account.ID)
	if err != nil {
		return Authenticated{}, true, err
	}
	switch {
	case !found:
		return Authenticated{}, true, reject("auth_session_not_found", account.ID)
	case session.RevokedAt != nil:
		return Authenticated{}, true, reject("auth_session_revoked", account.ID)
	case a.now().After(session.ExpiresAt):
		return Authenticated{}, true, reject("auth_session_expired", account.ID)
	}
	if err := a.store.TouchAuthSession(ctx, session.ID); err != nil {
		return Authenticated{}, true, err
	}
	result.AuthSession = &session
	result.Principal.Credential.AuthTime = session.CreatedAt
	return result, true, nil
}
