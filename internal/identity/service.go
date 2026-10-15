package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/platform/invalidate"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
	"github.com/uptrace/bun/driver/pgdriver"
)

// RefreshTokenTTL is the lifetime of an auth session and each refresh token
// issued for it.
const RefreshTokenTTL = 7 * 24 * time.Hour

var (
	ErrForbidden  = access.ErrForbidden
	ErrNotFound   = access.ErrNotFound
	ErrValidation = errors.New("identity: validation failed")
	// ErrSetupIncomplete rejects registration before the instance has an
	// administrator.
	ErrSetupIncomplete = errors.New("identity: instance setup is not complete")
	// ErrSignInUnavailable reports that the profile offers no sign-in.
	ErrSignInUnavailable = errors.New("identity: sign-in is unavailable")
	// ErrInvalidRefreshToken rejects a refresh or its auth session. Callers
	// see one error whatever the cause; RefreshRejection carries the cause.
	ErrInvalidRefreshToken = errors.New("identity: invalid refresh token")
)

// ValidationError carries field-safe validation messages. Values supplied by
// callers are never copied into these messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// RefreshRejection is an ErrInvalidRefreshToken with a cause for logs. Reason
// is empty when there is nothing worth logging, such as an unknown token.
type RefreshRejection struct {
	Reason        string
	AccountID     int64
	AuthSessionID string
}

func (e *RefreshRejection) Error() string { return ErrInvalidRefreshToken.Error() }
func (e *RefreshRejection) Unwrap() error { return ErrInvalidRefreshToken }

type Deps struct {
	Store Store
	// SignIn is the profile's sign-in strategy. It defaults to
	// SignInUnavailable.
	SignIn SignInStrategy
	// Setup is the profile's first-run setup strategy. It defaults to
	// FormSetup. FirstRun writes the setup records and Cache drops the new
	// organization's cached policy.
	Setup    SetupStrategy
	FirstRun FirstRunStore
	Cache    PolicyCache
	Requirer *access.Requirer
	Tokens   TokenPolicy
	// Secret signs access tokens.
	Secret string
	// Audit defaults to audit.Discard, Logger to a discarding logger and Now
	// to time.Now.
	Audit  audit.Writer
	Logger *slog.Logger
	Now    func() time.Time
	// Invalidator, when set, is told after each credential or session
	// revocation commits. Nil disables publishing.
	Invalidator invalidate.Invalidator
}

// Service owns accounts, sign-in, authentication sessions, instance
// administration, and first-run setup. Methods that take a principal authorize
// it themselves; Register, Login, Refresh, Logout, SignInEnabled,
// StartSession, Setup and SetupStatus are entry points that run before a
// caller is authenticated.
type Service struct {
	store    Store
	signIn   SignInStrategy
	setup    SetupStrategy
	firstRun FirstRunStore
	cache    PolicyCache
	requirer *access.Requirer
	tokens   TokenPolicy
	secret   string
	audit    audit.Writer
	logger   *slog.Logger
	now      func() time.Time
	inval    invalidate.Invalidator
}

func New(deps Deps) *Service {
	if deps.SignIn == nil {
		deps.SignIn = SignInUnavailable
	}
	if deps.Setup == nil {
		deps.Setup = FormSetup
	}
	if deps.Audit == nil {
		deps.Audit = audit.Discard
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{
		store:    deps.Store,
		signIn:   deps.SignIn,
		setup:    deps.Setup,
		firstRun: deps.FirstRun,
		cache:    deps.Cache,
		requirer: deps.Requirer,
		tokens:   deps.Tokens,
		secret:   deps.Secret,
		audit:    deps.Audit,
		logger:   deps.Logger,
		now:      deps.Now,
		inval:    deps.Invalidator,
	}
}

// Issued is a newly started session: the access token, and the raw refresh
// token the caller hands to the client as a cookie for RefreshTTL.
type Issued struct {
	AccountID     int64
	AuthSessionID string
	AccessToken   string
	RefreshToken  string
	RefreshTTL    time.Duration
}

type RegisterInput struct {
	Email    string
	Name     string
	Password string
}

// Register creates a password account. Self-registration opens only after
// instance setup has created an administrator.
func (s *Service) Register(ctx context.Context, in RegisterInput) (database.Account, error) {
	var v validator.Validator
	v.CheckField(in.Email != "", "email", "Email is required.")
	v.CheckField(validator.IsEmail(in.Email), "email", "Enter a valid email address.")
	v.CheckField(in.Name != "", "name", "Name is required.")
	v.CheckField(len(in.Password) >= 8, "password", "Password must be at least 8 characters.")
	if v.HasErrors() {
		return database.Account{}, &ValidationError{Fields: v.FieldErrors}
	}

	configured, err := s.store.HasAnyInstanceAdmin(ctx)
	if err != nil {
		return database.Account{}, err
	}
	if !configured {
		return database.Account{}, ErrSetupIncomplete
	}

	emailTaken := &ValidationError{Fields: map[string]string{"email": "An account with this email already exists."}}
	_, exists, err := s.store.GetAccountByEmail(ctx, in.Email)
	if err != nil {
		return database.Account{}, err
	}
	if exists {
		return database.Account{}, emailTaken
	}
	hashed, err := password.Hash(in.Password)
	if err != nil {
		return database.Account{}, err
	}
	account, err := s.store.InsertAccount(ctx, in.Email, in.Name, &hashed)
	if err != nil {
		if isUniqueViolation(err) {
			return database.Account{}, emailTaken
		}
		return database.Account{}, err
	}
	return account, nil
}

// SignInEnabled reports whether the profile offers sign-in at all.
func (s *Service) SignInEnabled() bool {
	return s.signIn.Enabled()
}

type LoginInput struct {
	Email    string
	Password string
}

// Login verifies credentials through the sign-in strategy and starts a
// session. A rejected sign-in returns a *CredentialError whose reason does not
// reveal whether the account exists. Every attempt past input validation
// records exactly one sign-in audit event.
func (s *Service) Login(ctx context.Context, in LoginInput, attrs access.RequestAttributes) (Issued, error) {
	if !s.signIn.Enabled() {
		return Issued{}, ErrSignInUnavailable
	}
	var v validator.Validator
	v.CheckField(in.Email != "", "email", "Email is required.")
	v.CheckField(in.Password != "", "password", "Password is required.")
	if v.HasErrors() {
		return Issued{}, &ValidationError{Fields: v.FieldErrors}
	}

	method := s.signIn.Method()
	reject := func(reason string) error {
		s.emitSignIn(ctx, attrs, audit.Event{Actor: audit.AnonymousActor(method.Name()), Outcome: audit.OutcomeFailure, DecisionReason: reason})
		return &CredentialError{Reason: reason}
	}

	subject, err := method.Begin(ctx, Credentials(in))
	var credErr *CredentialError
	if errors.As(err, &credErr) {
		return Issued{}, reject(credErr.Reason)
	}
	if err != nil {
		return Issued{}, err
	}
	if factors := s.signIn.Factors().Required(ctx, subject, attrs); len(factors) > 0 {
		return Issued{}, reject("factor_required")
	}
	decision, err := s.signIn.Policy().Evaluate(ctx, subject, attrs)
	if err != nil {
		return Issued{}, err
	}
	if decision.Effect != access.EffectAllow {
		reason := decision.Reason
		if reason == "" {
			reason = "sign_in_policy_denied"
		}
		return Issued{}, reject(reason)
	}
	credential, err := method.Complete(ctx, subject)
	if err != nil {
		return Issued{}, err
	}

	issued, err := s.issue(ctx, subject.Account, database.SessionAuth{Method: credential.Method, Assurance: string(credential.Assurance)}, attrs)
	if err != nil {
		return Issued{}, err
	}
	credential.ID = issued.AuthSessionID
	principal := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: subject.Account.ID}, Credential: credential}
	s.emitSignIn(ctx, attrs, audit.Event{Actor: audit.ActorFromPrincipal(principal), Outcome: audit.OutcomeSuccess})
	return issued, nil
}

// StartSession starts a session for an account another service has already
// verified, such as the account that just accepted an invitation.
func (s *Service) StartSession(ctx context.Context, account database.Account, auth database.SessionAuth, attrs access.RequestAttributes) (Issued, error) {
	return s.issue(ctx, account, auth, attrs)
}

func (s *Service) issue(ctx context.Context, account database.Account, auth database.SessionAuth, attrs access.RequestAttributes) (Issued, error) {
	raw := database.NewID()
	authSession, _, err := s.store.CreateAuthSessionWithRefreshToken(
		ctx, account.ID, s.now().Add(RefreshTokenTTL), attrs.UserAgent,
		clientIP(attrs), token.Hash(raw), raw, auth,
	)
	if err != nil {
		return Issued{}, err
	}
	accessToken, err := s.accessToken(ctx, account, authSession.ID)
	if err != nil {
		return Issued{}, err
	}
	return Issued{
		AccountID: account.ID, AuthSessionID: authSession.ID,
		AccessToken: accessToken, RefreshToken: raw, RefreshTTL: RefreshTokenTTL,
	}, nil
}

func (s *Service) accessToken(ctx context.Context, account database.Account, authSessionID string) (string, error) {
	ttl, err := s.tokens.AccessTokenTTL(ctx)
	if err != nil {
		return "", err
	}
	accessToken, _, err := token.IssueWithSessionTTL(strconv.FormatInt(account.ID, 10), authSessionID, account.Email, account.Name, s.secret, ttl)
	return accessToken, err
}

type RefreshInput struct {
	RefreshToken string
}

// Refresh rotates a refresh token and issues a new access token for its auth
// session. Presenting an expired token revokes its whole family. Every
// rejection unwraps to ErrInvalidRefreshToken.
func (s *Service) Refresh(ctx context.Context, in RefreshInput, attrs access.RequestAttributes) (Issued, error) {
	if in.RefreshToken == "" {
		return Issued{}, &RefreshRejection{}
	}
	now := s.now()
	rt, found, err := s.store.GetRefreshTokenByHash(ctx, token.Hash(in.RefreshToken))
	if err != nil {
		return Issued{}, err
	}
	if !found {
		return Issued{}, &RefreshRejection{}
	}
	if rt.RevokedAt != nil || now.After(rt.ExpiresAt) {
		reason := "refresh_token_revoked"
		if rt.RevokedAt == nil {
			reason = "refresh_token_expired"
			if err := s.store.RevokeFamilyTokens(ctx, rt.Family); err != nil {
				s.logger.WarnContext(ctx, "refresh token family revocation failed", slog.Int64("account_id", rt.AccountID), slog.Any("error", err))
			}
		}
		return Issued{}, &RefreshRejection{Reason: reason, AccountID: rt.AccountID, AuthSessionID: rt.AuthSessionID}
	}
	if rt.AuthSessionID == "" {
		return Issued{}, &RefreshRejection{}
	}

	account, found, err := s.store.GetAccount(ctx, rt.AccountID)
	if err != nil {
		return Issued{}, err
	}
	if !found || !account.IsActive {
		return Issued{}, &RefreshRejection{}
	}
	authSession, found, err := s.store.GetAuthSession(ctx, rt.AuthSessionID, account.ID)
	if err != nil {
		return Issued{}, err
	}
	if !found || authSession.RevokedAt != nil || now.After(authSession.ExpiresAt) {
		reason := "auth_session_not_found"
		if found && authSession.RevokedAt != nil {
			reason = "auth_session_revoked"
		} else if found {
			reason = "auth_session_expired"
		}
		return Issued{}, &RefreshRejection{Reason: reason, AccountID: account.ID, AuthSessionID: rt.AuthSessionID}
	}

	accessToken, err := s.accessToken(ctx, account, authSession.ID)
	if err != nil {
		return Issued{}, err
	}
	// The rotated token keeps the family of the token it replaces, so reuse
	// detection still revokes the whole chain.
	raw := database.NewID()
	if _, err := s.store.RotateRefreshToken(ctx, rt.ID, rt.AccountID, authSession.ID, token.Hash(raw), rt.Family,
		now.Add(RefreshTokenTTL), attrs.UserAgent, clientIP(attrs)); err != nil {
		return Issued{}, err
	}
	return Issued{
		AccountID: account.ID, AuthSessionID: authSession.ID,
		AccessToken: accessToken, RefreshToken: raw, RefreshTTL: RefreshTokenTTL,
	}, nil
}

type LogoutInput struct {
	RefreshToken string
}

// LoggedOut identifies the auth session a logout ended. AuthSessionID is
// empty when the refresh token named no session.
type LoggedOut struct {
	AccountID     int64
	AuthSessionID string
}

// Logout revokes the refresh token and its auth session. The refresh token is
// the credential, so an unknown or empty token ends nothing and is not an
// error.
func (s *Service) Logout(ctx context.Context, in LogoutInput) (LoggedOut, error) {
	if in.RefreshToken == "" {
		return LoggedOut{}, nil
	}
	rt, found, err := s.store.GetRefreshTokenByHash(ctx, token.Hash(in.RefreshToken))
	if err != nil || !found {
		return LoggedOut{}, err
	}
	if err := s.store.RevokeRefreshToken(ctx, rt.ID); err != nil {
		return LoggedOut{}, err
	}
	if rt.AuthSessionID == "" {
		return LoggedOut{}, nil
	}
	if err := s.store.RevokeAuthSession(ctx, rt.AuthSessionID, &rt.AccountID, "logout"); err != nil {
		return LoggedOut{}, err
	}
	s.publishSessionRevoked(ctx, rt.AuthSessionID)
	return LoggedOut{AccountID: rt.AccountID, AuthSessionID: rt.AuthSessionID}, nil
}

// publishCredentials announces a committed revocation. Delivery is
// best-effort: a failure is logged and never changes the operation's result.
func (s *Service) publishCredentials(ctx context.Context, key string) {
	if s.inval == nil {
		return
	}
	if err := s.inval.Publish(context.WithoutCancel(ctx), invalidate.TopicCredentials, key); err != nil {
		s.logger.WarnContext(ctx, "credential invalidation publish failed", slog.Any("error", err))
	}
}

func (s *Service) publishSessionRevoked(ctx context.Context, sessionID string) {
	s.publishCredentials(ctx, "session:"+sessionID)
}

func (s *Service) publishAccountRevoked(ctx context.Context, accountID int64) {
	s.publishCredentials(ctx, "account:"+strconv.FormatInt(accountID, 10))
}

// emitSignIn records a sign-in event on a best-effort basis; an audit outage
// must not change the outcome of a sign-in.
func (s *Service) emitSignIn(ctx context.Context, attrs access.RequestAttributes, event audit.Event) {
	event.Action = audit.ActionSignIn
	event.Metadata = map[string]string{"client_ip": clientIP(attrs)}
	if err := audit.Emit(ctx, s.audit, event); err != nil {
		s.logger.WarnContext(ctx, "audit write failed", slog.String("action", event.Action), slog.Any("error", err))
	}
}

func clientIP(attrs access.RequestAttributes) string {
	if !attrs.ClientIP.IsValid() {
		return ""
	}
	return attrs.ClientIP.String()
}

func requireAccount(p access.Principal) error {
	if p.Subject.Kind != access.SubjectAccount || p.Subject.ID == 0 {
		return ErrForbidden
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
