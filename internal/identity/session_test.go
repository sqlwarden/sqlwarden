package identity

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/token"
)

const sessionTestSecret = "session-test-secret"

type fakeSessionStore struct {
	account      database.Account
	accountFound bool
	accountErr   error
	session      database.AuthSession
	sessionFound bool
	sessionErr   error
	touchErr     error
	accountCalls int
	sessionCalls int
	touchCalls   int
}

func (s *fakeSessionStore) GetAccount(context.Context, int64) (database.Account, bool, error) {
	s.accountCalls++
	return s.account, s.accountFound, s.accountErr
}

func (s *fakeSessionStore) GetAuthSession(context.Context, string, int64) (database.AuthSession, bool, error) {
	s.sessionCalls++
	return s.session, s.sessionFound, s.sessionErr
}

func (s *fakeSessionStore) TouchAuthSession(context.Context, string) error {
	s.touchCalls++
	return s.touchErr
}

func issueSessionBearer(t *testing.T, accountID int64, sessionID string) string {
	t.Helper()
	raw, _, err := token.IssueWithSessionTTL(strconv.FormatInt(accountID, 10), sessionID, "test@example.com", "Test", sessionTestSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + raw
}

func newFakeSessionStore(now time.Time) *fakeSessionStore {
	return &fakeSessionStore{
		account:      database.Account{ID: 42, IsActive: true},
		accountFound: true,
		session: database.AuthSession{
			ID:         "session-1",
			AccountID:  42,
			CreatedAt:  now.Add(-time.Hour),
			ExpiresAt:  now.Add(time.Hour),
			AuthMethod: MethodLocal,
			Assurance:  string(access.AAL1),
		},
		sessionFound: true,
	}
}

func authenticateSession(store SessionStore, revocation bool, now time.Time, authorization string) (Authenticated, bool, error) {
	authenticator := NewSessionAuthenticator(
		store,
		sessionTestSecret,
		func(context.Context) bool { return revocation },
		func() time.Time { return now },
	)
	return authenticator.Authenticate(context.Background(), Presented{Authorization: authorization})
}

func requireCredentialReason(t *testing.T, err error, want string) {
	t.Helper()
	var credentialErr *CredentialError
	if !errors.As(err, &credentialErr) || credentialErr.Reason != want {
		t.Fatalf("err = %v, want credential reason %q", err, want)
	}
}

func TestSessionAuthenticatorEmptyHeaderIsUnclaimed(t *testing.T) {
	store := newFakeSessionStore(time.Now())
	_, ok, err := authenticateSession(store, true, time.Now(), "")
	if ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
}

func TestSessionAuthenticatorBasicHeaderIsUnclaimed(t *testing.T) {
	store := newFakeSessionStore(time.Now())
	_, ok, err := authenticateSession(store, true, time.Now(), "Basic abc")
	if ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
}

func TestSessionAuthenticatorInvalidBearerToken(t *testing.T) {
	store := newFakeSessionStore(time.Now())
	_, ok, err := authenticateSession(store, true, time.Now(), "Bearer garbage")
	if !ok {
		t.Fatal("invalid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "token_invalid")
}

func TestSessionAuthenticatorRequiresSessionBindingWhenRevocationEnabled(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, ""))
	if !ok {
		t.Fatal("valid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "session_binding_missing")
}

func TestSessionAuthenticatorAccountNotFound(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	store.accountFound = false
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok {
		t.Fatal("valid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "account_not_found")
}

func TestSessionAuthenticatorInactiveAccount(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	store.account.IsActive = false
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok {
		t.Fatal("valid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "account_inactive")
}

func TestSessionAuthenticatorSessionNotFound(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	store.sessionFound = false
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok {
		t.Fatal("valid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "auth_session_not_found")
}

func TestSessionAuthenticatorRevokedSession(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	store.session.RevokedAt = &now
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok {
		t.Fatal("valid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "auth_session_revoked")
	var credentialErr *CredentialError
	if !errors.As(err, &credentialErr) || credentialErr.AccountID != 42 || credentialErr.CredentialID != "session-1" {
		t.Fatalf("err = %#v, want account 42 and session-1", err)
	}
}

func TestSessionAuthenticatorExpiredSession(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	store.session.ExpiresAt = now.Add(-time.Second)
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok {
		t.Fatal("valid bearer token was not claimed")
	}
	requireCredentialReason(t, err, "auth_session_expired")
}

func TestSessionAuthenticatorValidSessionIsTouchedAndReturned(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	got, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if store.sessionCalls != 1 || store.touchCalls != 1 {
		t.Fatalf("session calls = %d, touch calls = %d", store.sessionCalls, store.touchCalls)
	}
	if got.AuthSession == nil || got.AuthSession.ID != "session-1" {
		t.Fatalf("auth session = %+v", got.AuthSession)
	}
}

func TestSessionAuthenticatorSkipsSessionWhenRevocationDisabled(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	got, ok, err := authenticateSession(store, false, now, issueSessionBearer(t, 42, "session-1"))
	if !ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if store.sessionCalls != 0 || store.touchCalls != 0 {
		t.Fatalf("session calls = %d, touch calls = %d", store.sessionCalls, store.touchCalls)
	}
	if got.AuthSession != nil {
		t.Fatalf("auth session = %+v", got.AuthSession)
	}
	if got.Principal.Credential.Method != MethodPassword || got.Principal.Credential.Assurance != access.AAL1 {
		t.Fatalf("credential = %+v", got.Principal.Credential)
	}
}

func TestSessionAuthenticatorBuildsPrincipal(t *testing.T) {
	now := time.Now()
	store := newFakeSessionStore(now)
	got, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if got.Principal.Subject != (access.SubjectRef{Kind: access.SubjectAccount, ID: 42}) {
		t.Fatalf("subject = %+v", got.Principal.Subject)
	}
	credential := got.Principal.Credential
	if credential.Kind != access.CredentialSession || credential.ID != "session-1" || credential.Method != MethodLocal || credential.Assurance != access.AAL1 {
		t.Fatalf("credential = %+v", credential)
	}
	if !credential.AuthTime.Equal(store.session.CreatedAt) {
		t.Fatalf("auth time = %v, want %v", credential.AuthTime, store.session.CreatedAt)
	}
	if got.Principal.InstanceAdmin {
		t.Fatal("instance admin was set")
	}
}

func TestSessionAuthenticatorReturnsStoreError(t *testing.T) {
	now := time.Now()
	want := errors.New("store failed")
	store := newFakeSessionStore(now)
	store.accountErr = want
	_, ok, err := authenticateSession(store, true, now, issueSessionBearer(t, 42, "session-1"))
	if !ok || !errors.Is(err, want) {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	var credentialErr *CredentialError
	if errors.As(err, &credentialErr) {
		t.Fatalf("store error wrapped as credential error: %v", err)
	}
}
