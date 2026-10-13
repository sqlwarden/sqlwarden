package identity_test

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
)

const (
	alice      int64 = 1
	bob        int64 = 2
	testOrg    int64 = 10
	missingOrg int64 = 11
	alicePass        = "alice-password"
	bobPass          = "bob-password"
)

func TestMain(m *testing.M) {
	restore := password.SetHashCostForTesting(4)
	code := m.Run()
	restore()
	os.Exit(code)
}

type fakeStore struct {
	now          func() time.Time
	adminExists  bool
	accounts     map[int64]database.Account
	sessions     map[string]database.AuthSession
	tokens       map[string]database.RefreshToken
	members      map[int64]map[int64]bool
	orgSessions  map[string]database.OrgAccessSession
	nextAccount  int64
	nextID       int
	insertErr    error
	familyRevoke []string
	admins       map[int64]bool
	orgs         map[string]database.Organization
}

func newFakeStore(t *testing.T, now func() time.Time) *fakeStore {
	t.Helper()
	s := &fakeStore{
		now:         now,
		adminExists: true,
		accounts:    map[int64]database.Account{},
		sessions:    map[string]database.AuthSession{},
		tokens:      map[string]database.RefreshToken{},
		members:     map[int64]map[int64]bool{testOrg: {alice: true, bob: true}},
		orgSessions: map[string]database.OrgAccessSession{},
		nextAccount: 100,
		admins:      map[int64]bool{alice: true},
		orgs:        map[string]database.Organization{},
	}
	for id, pw := range map[int64]string{alice: alicePass, bob: bobPass} {
		hashed, err := password.Hash(pw)
		if err != nil {
			t.Fatal(err)
		}
		email := map[int64]string{alice: "alice@example.com", bob: "bob@example.com"}[id]
		s.accounts[id] = database.Account{ID: id, Email: email, Name: email, Password: &hashed, IsActive: true}
	}
	return s
}

func (s *fakeStore) id(prefix string) string {
	s.nextID++
	return prefix + "-" + strconv.Itoa(s.nextID)
}

func (s *fakeStore) HasAnyInstanceAdmin(context.Context) (bool, error) { return s.adminExists, nil }

func (s *fakeStore) GetAccount(_ context.Context, id int64) (database.Account, bool, error) {
	a, ok := s.accounts[id]
	return a, ok, nil
}

func (s *fakeStore) GetAccountByEmail(_ context.Context, email string) (database.Account, bool, error) {
	for _, a := range s.accounts {
		if strings.EqualFold(a.Email, email) {
			return a, true, nil
		}
	}
	return database.Account{}, false, nil
}

func (s *fakeStore) InsertAccount(_ context.Context, email, name string, pw *string) (database.Account, error) {
	if s.insertErr != nil {
		return database.Account{}, s.insertErr
	}
	s.nextAccount++
	a := database.Account{ID: s.nextAccount, Email: email, Name: name, Password: pw, IsActive: true}
	s.accounts[a.ID] = a
	return a, nil
}

func (s *fakeStore) UpdateAccountName(_ context.Context, id int64, name string) (database.Account, error) {
	a := s.accounts[id]
	a.Name = name
	s.accounts[id] = a
	return a, nil
}

func (s *fakeStore) ChangeAccountPassword(_ context.Context, accountID int64, hashed, keep, reason string) error {
	a := s.accounts[accountID]
	a.Password = &hashed
	s.accounts[accountID] = a
	now := s.now()
	for id, sess := range s.sessions {
		if sess.AccountID == accountID && sess.RevokedAt == nil && id != keep {
			sess.RevokedAt, sess.RevocationReason = &now, reason
			s.sessions[id] = sess
		}
	}
	for hash, rt := range s.tokens {
		if rt.AccountID == accountID && rt.RevokedAt == nil && rt.AuthSessionID != keep {
			rt.RevokedAt = &now
			s.tokens[hash] = rt
		}
	}
	return nil
}

func (s *fakeStore) GetAccountOrgs(context.Context, int64) ([]database.Organization, error) {
	return nil, nil
}

func (s *fakeStore) ListAccountOrgsPage(_ context.Context, params database.ListAccountOrgsParams) (response.Paginated[database.AccountOrganizationListItem], error) {
	return response.Paginated[database.AccountOrganizationListItem]{Page: params.Page, PageSize: params.PageSize}, nil
}

func (s *fakeStore) CreateAuthSessionWithRefreshToken(_ context.Context, accountID int64, expiresAt time.Time, ua, ip, hash, family string, auth database.SessionAuth) (database.AuthSession, database.RefreshToken, error) {
	sess := database.AuthSession{ID: s.id("session"), AccountID: accountID, UserAgent: ua, IPAddress: ip, ExpiresAt: expiresAt, AuthMethod: auth.Method, Assurance: auth.Assurance}
	s.sessions[sess.ID] = sess
	rt := database.RefreshToken{ID: s.id("token"), AccountID: accountID, AuthSessionID: sess.ID, TokenHash: hash, Family: family, ExpiresAt: expiresAt}
	s.tokens[hash] = rt
	return sess, rt, nil
}

func (s *fakeStore) GetAuthSession(_ context.Context, id string, accountID int64) (database.AuthSession, bool, error) {
	sess, ok := s.sessions[id]
	if !ok || sess.AccountID != accountID {
		return database.AuthSession{}, false, nil
	}
	return sess, true, nil
}

func (s *fakeStore) RevokeAuthSession(_ context.Context, id string, revokedBy *int64, reason string) error {
	now := s.now()
	sess := s.sessions[id]
	sess.RevokedAt, sess.RevokedByAccountID, sess.RevocationReason = &now, revokedBy, reason
	s.sessions[id] = sess
	for hash, rt := range s.tokens {
		if rt.AuthSessionID == id && rt.RevokedAt == nil {
			rt.RevokedAt = &now
			s.tokens[hash] = rt
		}
	}
	return nil
}

func (s *fakeStore) RevokeAuthSessionsForAccount(ctx context.Context, accountID int64, revokedBy *int64, reason string) error {
	for id, sess := range s.sessions {
		if sess.AccountID == accountID && sess.RevokedAt == nil {
			_ = s.RevokeAuthSession(ctx, id, revokedBy, reason)
		}
	}
	return nil
}

func (s *fakeStore) ListAuthSessionsPage(_ context.Context, params database.ListAuthSessionsParams) (response.Paginated[database.AuthSession], error) {
	var items []database.AuthSession
	for _, sess := range s.sessions {
		if sess.AccountID == params.AccountID {
			items = append(items, sess)
		}
	}
	return response.Paginated[database.AuthSession]{Items: items, Page: params.Page, PageSize: params.PageSize, Total: len(items)}, nil
}

func (s *fakeStore) GetRefreshTokenByHash(_ context.Context, hash string) (database.RefreshToken, bool, error) {
	rt, ok := s.tokens[hash]
	return rt, ok, nil
}

func (s *fakeStore) RevokeRefreshToken(_ context.Context, id string) error {
	now := s.now()
	for hash, rt := range s.tokens {
		if rt.ID == id {
			rt.RevokedAt = &now
			s.tokens[hash] = rt
		}
	}
	return nil
}

func (s *fakeStore) RevokeFamilyTokens(_ context.Context, family string) error {
	s.familyRevoke = append(s.familyRevoke, family)
	now := s.now()
	for hash, rt := range s.tokens {
		if rt.Family == family && rt.RevokedAt == nil {
			rt.RevokedAt = &now
			s.tokens[hash] = rt
		}
	}
	return nil
}

func (s *fakeStore) RotateRefreshToken(ctx context.Context, oldID string, accountID int64, authSessionID, hash, family string, expiresAt time.Time, _, _ string) (database.RefreshToken, error) {
	_ = s.RevokeRefreshToken(ctx, oldID)
	rt := database.RefreshToken{ID: s.id("token"), AccountID: accountID, AuthSessionID: authSessionID, TokenHash: hash, Family: family, ExpiresAt: expiresAt}
	s.tokens[hash] = rt
	return rt, nil
}

func (s *fakeStore) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return s.members[orgID][accountID], nil
}

func (s *fakeStore) ListOrgAccessSessionsPage(_ context.Context, orgID, accountID int64, page, pageSize int) (response.Paginated[database.OrgAccessSession], error) {
	var items []database.OrgAccessSession
	for _, sess := range s.orgSessions {
		if sess.OrgID == orgID && sess.AccountID == accountID {
			items = append(items, sess)
		}
	}
	return response.Paginated[database.OrgAccessSession]{Items: items, Page: page, PageSize: pageSize, Total: len(items)}, nil
}

func (s *fakeStore) GetOrgAccessSessionByID(_ context.Context, id string, orgID, accountID int64) (database.OrgAccessSession, bool, error) {
	sess, ok := s.orgSessions[id]
	if !ok || sess.OrgID != orgID || sess.AccountID != accountID {
		return database.OrgAccessSession{}, false, nil
	}
	return sess, true, nil
}

func (s *fakeStore) RevokeOrgAccessSession(_ context.Context, id string, _ int64, revokedBy *int64, reason string) error {
	now := s.now()
	sess := s.orgSessions[id]
	sess.RevokedAt, sess.RevokedByAccountID, sess.RevocationReason = &now, revokedBy, reason
	s.orgSessions[id] = sess
	return nil
}

type fakeDirectory struct{ store *fakeStore }

func (d fakeDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == testOrg, nil
}

func (d fakeDirectory) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	// missingOrg reports membership so the Requirer reaches its org lookup.
	if orgID == missingOrg {
		return true, nil
	}
	return d.store.IsOrgMember(ctx, orgID, accountID)
}

func (fakeDirectory) IsEffectiveWorkspaceMember(context.Context, int64, int64, int64) (bool, error) {
	return false, nil
}

func (fakeDirectory) GetWorkspace(context.Context, int64) (access.Workspace, bool, error) {
	return access.Workspace{}, false, nil
}

func (fakeDirectory) GetConnection(context.Context, int64) (access.Connection, bool, error) {
	return access.Connection{}, false, nil
}

// fakePolicy grants each account exactly the organization permissions listed
// for it.
type fakePolicy struct{ grants map[int64]map[string]bool }

func (p *fakePolicy) Can(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return ownerType == "org" && resourceType == "org" && resourceID == orgID && p.grants[accountID][permission]
}

func (p *fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

func (p *fakePolicy) set(accountID int64, permissions ...string) {
	granted := map[string]bool{}
	for _, permission := range permissions {
		granted[permission] = true
	}
	p.grants[accountID] = granted
}

type fixedTTL time.Duration

func (f fixedTTL) AccessTokenTTL(context.Context) (time.Duration, error) {
	return time.Duration(f), nil
}

type harness struct {
	svc    *identity.Service
	store  *fakeStore
	policy *fakePolicy
	events []audit.Event
	now    time.Time
}

const secret = "k7mp29rf4qxhwn8vbtaj6pgucmve53y9"

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	clock := func() time.Time { return h.now }
	h.store = newFakeStore(t, clock)
	h.policy = &fakePolicy{grants: map[int64]map[string]bool{}}
	h.policy.set(alice, access.AllPermissions()...)
	h.svc = identity.New(identity.Deps{
		Store:    h.store,
		SignIn:   identity.PasswordSignIn(h.store),
		Requirer: access.NewRequirer(fakeDirectory{store: h.store}, h.policy),
		Tokens:   fixedTTL(15 * time.Minute),
		Secret:   secret,
		Audit: audit.WriterFunc(func(_ context.Context, e audit.Event) error {
			h.events = append(h.events, e)
			return nil
		}),
		Now: clock,
	})
	return h
}

var attrs = access.RequestAttributes{ClientIP: netip.MustParseAddr("192.0.2.7"), UserAgent: "test-agent"}

func account(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

func sessionPrincipal(id int64, sessionID string) access.Principal {
	p := account(id)
	p.Credential = access.CredentialInfo{Kind: access.CredentialSession, ID: sessionID}
	return p
}

func admin(id int64) access.Principal {
	p := account(id)
	p.InstanceAdmin = true
	return p
}

func (h *harness) login(t *testing.T, email, pw string) identity.Issued {
	t.Helper()
	issued, err := h.svc.Login(context.Background(), identity.LoginInput{Email: email, Password: pw}, attrs)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return issued
}

func TestLoginSameErrorForUnknownAccountAndBadPassword(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, unknownErr := h.svc.Login(ctx, identity.LoginInput{Email: "nobody@example.com", Password: "whatever-pass"}, attrs)
	_, badErr := h.svc.Login(ctx, identity.LoginInput{Email: "alice@example.com", Password: "wrong-password"}, attrs)

	var unknownCred, badCred *identity.CredentialError
	if !errors.As(unknownErr, &unknownCred) || !errors.As(badErr, &badCred) {
		t.Fatalf("errors = %v, %v", unknownErr, badErr)
	}
	if unknownCred.Reason != "invalid_credentials" || *unknownCred != *badCred || unknownErr.Error() != badErr.Error() {
		t.Fatalf("unknown=%+v bad=%+v", unknownCred, badCred)
	}
	if len(h.events) != 2 {
		t.Fatalf("events = %d", len(h.events))
	}
	for _, e := range h.events {
		if e.Action != audit.ActionSignIn || e.Outcome != audit.OutcomeFailure || e.DecisionReason != "invalid_credentials" || e.Actor.SubjectID != nil {
			t.Fatalf("event = %+v", e)
		}
		if e.Metadata["client_ip"] != "192.0.2.7" {
			t.Fatalf("metadata = %v", e.Metadata)
		}
		for _, v := range e.Metadata {
			if strings.Contains(v, "@") {
				t.Fatalf("metadata leaks email: %v", e.Metadata)
			}
		}
	}
	if len(h.store.sessions) != 0 {
		t.Fatalf("failed logins created sessions")
	}
}

func TestLoginIssuesSessionAndAuditsSuccess(t *testing.T) {
	h := newHarness(t)
	issued := h.login(t, "alice@example.com", alicePass)
	if issued.AccountID != alice || issued.AccessToken == "" || issued.RefreshToken == "" || issued.RefreshTTL != identity.RefreshTokenTTL {
		t.Fatalf("issued = %+v", issued)
	}
	sess := h.store.sessions[issued.AuthSessionID]
	if sess.AccountID != alice || sess.UserAgent != "test-agent" || sess.IPAddress != "192.0.2.7" || sess.AuthMethod != identity.MethodPassword {
		t.Fatalf("session = %+v", sess)
	}
	if _, ok := h.store.tokens[token.Hash(issued.RefreshToken)]; !ok {
		t.Fatal("refresh token not stored by hash")
	}
	claims, err := token.Verify(issued.AccessToken, secret)
	if err != nil || claims.AuthSessionID != issued.AuthSessionID {
		t.Fatalf("claims = %+v, %v", claims, err)
	}
	if len(h.events) != 1 {
		t.Fatalf("events = %d", len(h.events))
	}
	e := h.events[0]
	if e.Outcome != audit.OutcomeSuccess || e.Actor.SubjectID == nil || *e.Actor.SubjectID != alice || e.Actor.CredentialID != issued.AuthSessionID || e.Actor.AuthMethod != identity.MethodPassword {
		t.Fatalf("event = %+v", e)
	}
}

func TestLoginValidationAndUnavailable(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.Login(context.Background(), identity.LoginInput{}, attrs)
	var ve *identity.ValidationError
	if !errors.As(err, &ve) || ve.Fields["email"] != "Email is required." || ve.Fields["password"] != "Password is required." {
		t.Fatalf("err = %v", err)
	}
	if len(h.events) != 0 {
		t.Fatalf("validation failures must not audit")
	}

	off := identity.New(identity.Deps{Store: h.store, SignIn: identity.SignInUnavailable})
	if off.SignInEnabled() {
		t.Fatal("sign-in should be disabled")
	}
	if _, err := off.Login(context.Background(), identity.LoginInput{Email: "a@example.com", Password: "x"}, attrs); !errors.Is(err, identity.ErrSignInUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshRotatesToken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	first := h.login(t, "alice@example.com", alicePass)
	old := h.store.tokens[token.Hash(first.RefreshToken)]

	second, err := h.svc.Refresh(ctx, identity.RefreshInput{RefreshToken: first.RefreshToken}, attrs)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken || second.AuthSessionID != first.AuthSessionID || second.AccessToken == "" {
		t.Fatalf("second = %+v", second)
	}
	if h.store.tokens[token.Hash(first.RefreshToken)].RevokedAt == nil {
		t.Fatal("old refresh token not revoked")
	}
	rotated := h.store.tokens[token.Hash(second.RefreshToken)]
	if rotated.Family != old.Family || rotated.RevokedAt != nil {
		t.Fatalf("rotated = %+v", rotated)
	}

	_, err = h.svc.Refresh(ctx, identity.RefreshInput{RefreshToken: first.RefreshToken}, attrs)
	var rej *identity.RefreshRejection
	if !errors.Is(err, identity.ErrInvalidRefreshToken) || !errors.As(err, &rej) || rej.Reason != "refresh_token_revoked" || rej.AccountID != alice {
		t.Fatalf("reuse err = %v", err)
	}
}

func TestRefreshRejectsExpiredTokenAndRevokesFamily(t *testing.T) {
	h := newHarness(t)
	issued := h.login(t, "alice@example.com", alicePass)
	family := h.store.tokens[token.Hash(issued.RefreshToken)].Family
	h.now = h.now.Add(identity.RefreshTokenTTL + time.Minute)

	_, err := h.svc.Refresh(context.Background(), identity.RefreshInput{RefreshToken: issued.RefreshToken}, attrs)
	var rej *identity.RefreshRejection
	if !errors.As(err, &rej) || rej.Reason != "refresh_token_expired" {
		t.Fatalf("err = %v", err)
	}
	if len(h.store.familyRevoke) != 1 || h.store.familyRevoke[0] != family {
		t.Fatalf("family revocations = %v", h.store.familyRevoke)
	}
}

func TestRefreshRejectsRevokedSessionAndUnknownToken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	issued := h.login(t, "alice@example.com", alicePass)
	sess := h.store.sessions[issued.AuthSessionID]
	now := h.now
	sess.RevokedAt = &now
	h.store.sessions[sess.ID] = sess

	_, err := h.svc.Refresh(ctx, identity.RefreshInput{RefreshToken: issued.RefreshToken}, attrs)
	var rej *identity.RefreshRejection
	if !errors.As(err, &rej) || rej.Reason != "auth_session_revoked" || rej.AuthSessionID != issued.AuthSessionID {
		t.Fatalf("revoked session err = %v", err)
	}
	for _, raw := range []string{"", "not-a-token"} {
		if _, err := h.svc.Refresh(ctx, identity.RefreshInput{RefreshToken: raw}, attrs); !errors.Is(err, identity.ErrInvalidRefreshToken) {
			t.Fatalf("token %q err = %v", raw, err)
		}
	}

	inactive := h.login(t, "bob@example.com", bobPass)
	acc := h.store.accounts[bob]
	acc.IsActive = false
	h.store.accounts[bob] = acc
	if _, err := h.svc.Refresh(ctx, identity.RefreshInput{RefreshToken: inactive.RefreshToken}, attrs); !errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("inactive account err = %v", err)
	}
}

func TestLogoutRevokesTokenAndSession(t *testing.T) {
	h := newHarness(t)
	issued := h.login(t, "alice@example.com", alicePass)
	out, err := h.svc.Logout(context.Background(), identity.LogoutInput{RefreshToken: issued.RefreshToken})
	if err != nil || out.AuthSessionID != issued.AuthSessionID || out.AccountID != alice {
		t.Fatalf("out = %+v, %v", out, err)
	}
	if h.store.sessions[issued.AuthSessionID].RevocationReason != "logout" || h.store.tokens[token.Hash(issued.RefreshToken)].RevokedAt == nil {
		t.Fatal("logout did not revoke")
	}
	if out, err := h.svc.Logout(context.Background(), identity.LogoutInput{RefreshToken: "unknown"}); err != nil || out != (identity.LoggedOut{}) {
		t.Fatalf("unknown logout = %+v, %v", out, err)
	}
}

func TestUpdatePasswordRevokesOtherSessions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	current := h.login(t, "alice@example.com", alicePass)
	other := h.login(t, "alice@example.com", alicePass)
	bystander := h.login(t, "bob@example.com", bobPass)
	p := sessionPrincipal(alice, current.AuthSessionID)

	err := h.svc.UpdatePassword(ctx, p, identity.UpdatePasswordInput{CurrentPassword: "wrong-password", NewPassword: "new-password"})
	var ve *identity.ValidationError
	if !errors.As(err, &ve) || ve.Fields["current_password"] != "Current password is incorrect." {
		t.Fatalf("wrong current err = %v", err)
	}
	if h.store.sessions[other.AuthSessionID].RevokedAt != nil {
		t.Fatal("failed change revoked sessions")
	}

	if err := h.svc.UpdatePassword(ctx, p, identity.UpdatePasswordInput{CurrentPassword: alicePass, NewPassword: "new-password"}); err != nil {
		t.Fatal(err)
	}
	if h.store.sessions[current.AuthSessionID].RevokedAt != nil || h.store.tokens[token.Hash(current.RefreshToken)].RevokedAt != nil {
		t.Fatal("current session revoked")
	}
	if s := h.store.sessions[other.AuthSessionID]; s.RevokedAt == nil || s.RevocationReason != identity.PasswordChangedReason {
		t.Fatalf("other session = %+v", s)
	}
	if h.store.tokens[token.Hash(other.RefreshToken)].RevokedAt == nil {
		t.Fatal("other refresh token not revoked")
	}
	if h.store.sessions[bystander.AuthSessionID].RevokedAt != nil {
		t.Fatal("another account's session revoked")
	}
	if ok, _ := password.Matches("new-password", *h.store.accounts[alice].Password); !ok {
		t.Fatal("password not changed")
	}
}

func TestUpdatePasswordValidation(t *testing.T) {
	h := newHarness(t)
	err := h.svc.UpdatePassword(context.Background(), account(alice), identity.UpdatePasswordInput{NewPassword: "short"})
	var ve *identity.ValidationError
	if !errors.As(err, &ve) || ve.Fields["current_password"] != "Current password is required." || ve.Fields["new_password"] != "New password must be at least 8 characters." {
		t.Fatalf("err = %v", err)
	}
	acc := h.store.accounts[bob]
	acc.Password = nil
	h.store.accounts[bob] = acc
	err = h.svc.UpdatePassword(context.Background(), account(bob), identity.UpdatePasswordInput{CurrentPassword: "anything", NewPassword: "long-enough"})
	if !errors.As(err, &ve) || ve.Fields["current_password"] != "Password changes are not available for this account." {
		t.Fatalf("passwordless err = %v", err)
	}
}

func TestOwnSessionsCannotTouchAnotherAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	mine := h.login(t, "alice@example.com", alicePass)
	theirs := h.login(t, "bob@example.com", bobPass)

	if err := h.svc.RevokeOwnSession(ctx, account(alice), theirs.AuthSessionID); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if h.store.sessions[theirs.AuthSessionID].RevokedAt != nil {
		t.Fatal("another account's session revoked")
	}
	page, err := h.svc.ListOwnSessions(ctx, account(alice), identity.Page{Page: 1, PageSize: 25})
	if err != nil || page.Total != 1 || page.Items[0].ID != mine.AuthSessionID {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if err := h.svc.RevokeOwnSessions(ctx, account(alice)); err != nil {
		t.Fatal(err)
	}
	if h.store.sessions[mine.AuthSessionID].RevocationReason != "user_revoked_all" || h.store.sessions[theirs.AuthSessionID].RevokedAt != nil {
		t.Fatal("revoke all crossed accounts")
	}
	if err := h.svc.RevokeOwnSession(ctx, account(alice), "  "); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("blank id err = %v", err)
	}
}

func TestPrincipalMethodsRequireAccountSubject(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	team := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectTeam, ID: alice}, InstanceAdmin: true}
	calls := map[string]error{}
	_, calls["GetAccount"] = h.svc.GetAccount(ctx, team)
	_, calls["UpdateAccount"] = h.svc.UpdateAccount(ctx, team, identity.UpdateAccountInput{Name: "x"})
	calls["UpdatePassword"] = h.svc.UpdatePassword(ctx, team, identity.UpdatePasswordInput{})
	_, calls["Orgs"] = h.svc.Orgs(ctx, team, identity.ListOrgsInput{})
	_, calls["CurrentSession"] = h.svc.CurrentSession(ctx, team)
	_, calls["ListOwnSessions"] = h.svc.ListOwnSessions(ctx, team, identity.Page{})
	calls["RevokeOwnSession"] = h.svc.RevokeOwnSession(ctx, team, "x")
	calls["RevokeOwnSessions"] = h.svc.RevokeOwnSessions(ctx, team)
	_, calls["ListInstanceSessions"] = h.svc.ListInstanceSessions(ctx, team, alice, identity.Page{})
	calls["RevokeInstanceSession"] = h.svc.RevokeInstanceSession(ctx, team, alice, "x")
	calls["RevokeInstanceSessions"] = h.svc.RevokeInstanceSessions(ctx, team, alice)
	for name, err := range calls {
		if !errors.Is(err, identity.ErrForbidden) {
			t.Errorf("%s err = %v", name, err)
		}
	}
}

func TestCurrentSessionReportsPrincipalInstanceAdmin(t *testing.T) {
	h := newHarness(t)
	got, err := h.svc.CurrentSession(context.Background(), admin(alice))
	if err != nil || got.Account.ID != alice || !got.IsInstanceAdmin || got.Organizations == nil {
		t.Fatalf("got = %+v, %v", got, err)
	}
	got, err = h.svc.CurrentSession(context.Background(), account(bob))
	if err != nil || got.IsInstanceAdmin {
		t.Fatalf("got = %+v, %v", got, err)
	}
}

func TestInstanceSessionsRequireInstanceAdmin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	target := h.login(t, "bob@example.com", bobPass)

	deny := func(p access.Principal) map[string]error {
		out := map[string]error{}
		_, out["list"] = h.svc.ListInstanceSessions(ctx, p, bob, identity.Page{})
		out["revoke"] = h.svc.RevokeInstanceSession(ctx, p, bob, target.AuthSessionID)
		out["revoke_all"] = h.svc.RevokeInstanceSessions(ctx, p, bob)
		return out
	}
	for name, err := range deny(account(alice)) {
		if !errors.Is(err, identity.ErrForbidden) {
			t.Errorf("non-admin %s err = %v", name, err)
		}
	}
	if h.store.sessions[target.AuthSessionID].RevokedAt != nil {
		t.Fatal("denied call revoked a session")
	}

	if _, err := h.svc.ListInstanceSessions(ctx, admin(alice), 999, identity.Page{}); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("unknown account list err = %v", err)
	}
	if err := h.svc.RevokeInstanceSessions(ctx, admin(alice), 999); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("unknown account revoke err = %v", err)
	}
	if err := h.svc.RevokeInstanceSession(ctx, admin(alice), alice, target.AuthSessionID); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("mismatched account err = %v", err)
	}
	page, err := h.svc.ListInstanceSessions(ctx, admin(alice), bob, identity.Page{Page: 1, PageSize: 25})
	if err != nil || page.Total != 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if err := h.svc.RevokeInstanceSession(ctx, admin(alice), bob, target.AuthSessionID); err != nil {
		t.Fatal(err)
	}
	s := h.store.sessions[target.AuthSessionID]
	if s.RevocationReason != "instance_admin_revoked" || s.RevokedByAccountID == nil || *s.RevokedByAccountID != alice {
		t.Fatalf("session = %+v", s)
	}
	if err := h.svc.RevokeInstanceSessions(ctx, admin(alice), bob); err != nil {
		t.Fatal(err)
	}
}

func TestOrgMemberAccessSessionsDenyMatrix(t *testing.T) {
	const outsider int64 = 3
	type call func(h *harness, p access.Principal, orgID int64) error
	calls := map[string]call{
		"list": func(h *harness, p access.Principal, orgID int64) error {
			_, err := h.svc.ListOrgMemberAccessSessions(context.Background(), p, orgID, bob, identity.Page{})
			return err
		},
		"revoke": func(h *harness, p access.Principal, orgID int64) error {
			return h.svc.RevokeOrgMemberAccessSession(context.Background(), p, orgID, bob, "org-session")
		},
	}
	seed := func(h *harness) {
		h.store.orgSessions["org-session"] = database.OrgAccessSession{ID: "org-session", OrgID: testOrg, AccountID: bob}
	}
	allBut := func(excluded string) []string {
		var out []string
		for _, p := range access.AllPermissions() {
			if p != excluded {
				out = append(out, p)
			}
		}
		return out
	}

	for name, c := range calls {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			seed(h)
			if err := c(h, account(outsider), testOrg); !errors.Is(err, identity.ErrForbidden) {
				t.Errorf("non-member err = %v", err)
			}
			if err := c(h, account(alice), missingOrg); !errors.Is(err, identity.ErrNotFound) {
				t.Errorf("missing org err = %v", err)
			}
			h.policy.set(bob, allBut(access.PermOrgWrite)...)
			if err := c(h, account(bob), testOrg); !errors.Is(err, identity.ErrForbidden) {
				t.Errorf("without org:write err = %v", err)
			}
			if h.store.orgSessions["org-session"].RevokedAt != nil {
				t.Fatal("denied call revoked a session")
			}
			h.policy.set(bob, access.PermOrgWrite)
			if err := c(h, account(bob), testOrg); err != nil {
				t.Errorf("with only org:write err = %v", err)
			}
		})
	}

	h := newHarness(t)
	seed(h)
	if _, err := h.svc.ListOrgMemberAccessSessions(context.Background(), account(alice), testOrg, outsider, identity.Page{}); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("non-member target err = %v", err)
	}
	if err := h.svc.RevokeOrgMemberAccessSession(context.Background(), account(alice), testOrg, alice, "org-session"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("other account session err = %v", err)
	}
	if err := h.svc.RevokeOrgMemberAccessSession(context.Background(), account(alice), testOrg, bob, "org-session"); err != nil {
		t.Fatal(err)
	}
	s := h.store.orgSessions["org-session"]
	if s.RevocationReason != "org_admin_revoked" || s.RevokedByAccountID == nil || *s.RevokedByAccountID != alice {
		t.Fatalf("org session = %+v", s)
	}
}

func TestRegister(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, err := h.svc.Register(ctx, identity.RegisterInput{Email: "bad", Password: "short"})
	var ve *identity.ValidationError
	if !errors.As(err, &ve) || ve.Fields["email"] != "Enter a valid email address." || ve.Fields["name"] != "Name is required." || ve.Fields["password"] != "Password must be at least 8 characters." {
		t.Fatalf("validation err = %v", err)
	}

	_, err = h.svc.Register(ctx, identity.RegisterInput{Email: "alice@example.com", Name: "Alice", Password: "long-enough"})
	if !errors.As(err, &ve) || ve.Fields["email"] != "An account with this email already exists." {
		t.Fatalf("duplicate err = %v", err)
	}

	h.store.insertErr = errors.New("UNIQUE constraint failed: accounts.email")
	_, err = h.svc.Register(ctx, identity.RegisterInput{Email: "race@example.com", Name: "Race", Password: "long-enough"})
	if !errors.As(err, &ve) || ve.Fields["email"] != "An account with this email already exists." {
		t.Fatalf("unique violation err = %v", err)
	}
	h.store.insertErr = nil

	created, err := h.svc.Register(ctx, identity.RegisterInput{Email: "carol@example.com", Name: "Carol", Password: "long-enough"})
	if err != nil || created.Email != "carol@example.com" || created.Password == nil {
		t.Fatalf("created = %+v, %v", created, err)
	}

	h.store.adminExists = false
	if _, err := h.svc.Register(ctx, identity.RegisterInput{Email: "dave@example.com", Name: "Dave", Password: "long-enough"}); !errors.Is(err, identity.ErrSetupIncomplete) {
		t.Fatalf("setup err = %v", err)
	}
}

func TestAccountProfile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.UpdateAccount(ctx, account(alice), identity.UpdateAccountInput{Name: "   "}); err == nil {
		t.Fatal("blank name accepted")
	}
	updated, err := h.svc.UpdateAccount(ctx, account(alice), identity.UpdateAccountInput{Name: "  Alice A  "})
	if err != nil || updated.Name != "Alice A" {
		t.Fatalf("updated = %+v, %v", updated, err)
	}
	got, err := h.svc.GetAccount(ctx, account(alice))
	if err != nil || got.Name != "Alice A" {
		t.Fatalf("got = %+v, %v", got, err)
	}
	if _, err := h.svc.GetAccount(ctx, account(999)); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}
