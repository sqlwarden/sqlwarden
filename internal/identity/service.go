package identity

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/validator"
)

// Store is the account persistence contract the identity service reads and
// writes through. *database.DB satisfies it.
type Store interface {
	HasAnyInstanceAdmin(ctx context.Context) (bool, error)
	AccountByEmail(ctx context.Context, email string) (database.Account, bool, error)
	Account(ctx context.Context, accountID int64) (database.Account, bool, error)
	InsertAccount(ctx context.Context, email, name string, hashedPassword *string) (database.Account, error)
	UpdateAccountName(ctx context.Context, accountID int64, name string) (database.Account, error)
	UpdateAccountPassword(ctx context.Context, accountID int64, hashedPassword string) error
}

// Service is the application service for account identity use cases:
// registration, authentication, profile, and credential changes. It owns the
// rules that used to live in the HTTP transport, which keeps request decoding,
// session issuance, and status-code mapping.
//
// Authentication is delegated to a [Provider] so an edition can extend the
// supported methods without the service changing. The service always resolves
// the returned subject back to a live account, so a provider cannot admit an
// identity that has no active SQLWarden account.
// Audited actions emitted by identity use cases.
const (
	ActionRegister       = "identity.account.registered"
	ActionAuthenticate   = "identity.authentication"
	ActionUpdateName     = "identity.account.name_updated"
	ActionChangePassword = "identity.account.password_changed"

	auditResourceAccount = "account"
)

type Service struct {
	store    Store
	provider Provider
	audit    audit.Writer
}

// NewService returns the identity service. The provider is the
// edition-decorated authentication path, and the writer is the
// edition-composed audit sink every identity use case emits through.
func NewService(store Store, provider Provider, writer audit.Writer) *Service {
	if writer == nil {
		writer = audit.Discard
	}
	return &Service{store: store, provider: provider, audit: writer}
}

// RegisterInput is a self-service registration request. The password is never
// logged or returned.
type RegisterInput struct {
	Email    string
	Name     string
	Password string
}

// Register creates a password account. Registration is refused until the
// instance has an administrator, so a fresh instance cannot be claimed by a
// drive-by signup before setup completes.
func (s *Service) Register(ctx context.Context, input RegisterInput) (database.Account, error) {
	configured, err := s.store.HasAnyInstanceAdmin(ctx)
	if err != nil {
		return database.Account{}, err
	}
	if !configured {
		return database.Account{}, s.auditFailure(ctx, ActionRegister, nil, map[string]string{"reason": "setup_incomplete"}, ErrSetupIncomplete)
	}

	email := strings.TrimSpace(input.Email)
	name := strings.TrimSpace(input.Name)
	if !validator.IsEmail(email) {
		return database.Account{}, ErrInvalidEmail
	}
	if name == "" {
		return database.Account{}, ErrInvalidName
	}
	if len(input.Password) < 8 {
		return database.Account{}, ErrWeakPassword
	}
	_, exists, err := s.store.AccountByEmail(ctx, email)
	if err != nil {
		return database.Account{}, err
	}
	if exists {
		return database.Account{}, s.auditFailure(ctx, ActionRegister, nil, map[string]string{"reason": "email_taken"}, ErrEmailTaken)
	}

	hashed, err := password.Hash(input.Password)
	if err != nil {
		return database.Account{}, err
	}

	account, err := s.store.InsertAccount(ctx, email, name, &hashed)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return database.Account{}, s.auditFailure(ctx, ActionRegister, nil, map[string]string{"reason": "email_taken"}, ErrEmailTaken)
		}
		return database.Account{}, err
	}
	if err := s.auditSuccess(ctx, ActionRegister, &account.ID, nil); err != nil {
		return database.Account{}, err
	}
	return account, nil
}

// Authenticate verifies a credential and returns the active account behind it.
// Every failure that a caller could use to probe for account existence is
// reported as [ErrInvalidCredentials].
func (s *Service) Authenticate(ctx context.Context, request AuthenticationRequest) (database.Account, error) {
	metadata := map[string]string{"method": string(request.Method)}
	failed := func(reason string, cause error) (database.Account, error) {
		metadata["reason"] = reason
		return database.Account{}, s.auditFailure(ctx, ActionAuthenticate, nil, metadata, cause)
	}

	subject, err := s.provider.Authenticate(ctx, request)
	if err != nil {
		return failed("provider_rejected", err)
	}
	if subject.AccountID == 0 {
		return failed("no_subject", ErrInvalidCredentials)
	}

	account, found, err := s.store.Account(ctx, subject.AccountID)
	if err != nil {
		return database.Account{}, err
	}
	if !found || !account.IsActive {
		return failed("account_unavailable", ErrInvalidCredentials)
	}
	if err := s.auditSuccess(ctx, ActionAuthenticate, &account.ID, metadata); err != nil {
		return database.Account{}, err
	}
	return account, nil
}

// AuthenticateWithPassword is the password login use case.
func (s *Service) AuthenticateWithPassword(ctx context.Context, email, secret string) (database.Account, error) {
	return s.Authenticate(ctx, AuthenticationRequest{
		Method:     AuthenticationPassword,
		Identifier: email,
		Secret:     secret,
	})
}

// UpdateName changes an account's display name.
func (s *Service) UpdateName(ctx context.Context, accountID int64, name string) (database.Account, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return database.Account{}, ErrInvalidName
	}
	account, err := s.store.UpdateAccountName(ctx, accountID, name)
	if err != nil {
		return database.Account{}, err
	}
	if err := s.auditSuccess(ctx, ActionUpdateName, &accountID, nil); err != nil {
		return database.Account{}, err
	}
	return account, nil
}

// ChangePassword replaces an account password after verifying the current one.
// An account without a local password authenticates elsewhere and reports
// [ErrPasswordUnavailable].
func (s *Service) ChangePassword(ctx context.Context, accountID int64, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrWeakPassword
	}
	account, found, err := s.store.Account(ctx, accountID)
	if err != nil {
		return err
	}
	if !found {
		return ErrAccountNotFound
	}
	if account.Password == nil {
		return s.auditFailure(ctx, ActionChangePassword, &accountID, map[string]string{"reason": "password_unavailable"}, ErrPasswordUnavailable)
	}

	match, err := password.Matches(currentPassword, *account.Password)
	if err != nil {
		return err
	}
	if !match {
		return s.auditFailure(ctx, ActionChangePassword, &accountID, map[string]string{"reason": "invalid_current_password"}, ErrInvalidCredentials)
	}

	hashed, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := s.store.UpdateAccountPassword(ctx, accountID, hashed); err != nil {
		return err
	}
	return s.auditSuccess(ctx, ActionChangePassword, &accountID, nil)
}

// auditSuccess records a completed identity use case. Its error is returned to
// the caller: an identity change that could not be audited is not reported as
// having succeeded.
func (s *Service) auditSuccess(ctx context.Context, action string, accountID *int64, metadata map[string]string) error {
	return audit.Emit(ctx, s.audit, s.event(action, audit.OutcomeSuccess, accountID, metadata))
}

// auditFailure records a refused identity use case and returns cause
// unchanged, so an audit sink problem cannot mask the reason the use case was
// refused.
func (s *Service) auditFailure(ctx context.Context, action string, accountID *int64, metadata map[string]string, cause error) error {
	_ = audit.Emit(ctx, s.audit, s.event(action, audit.OutcomeFailure, accountID, metadata))
	return cause
}

func (s *Service) event(action, outcome string, accountID *int64, metadata map[string]string) audit.Event {
	event := audit.Event{
		AccountID: accountID,
		Action:    action,
		Resource:  auditResourceAccount,
		Outcome:   outcome,
		Metadata:  metadata,
	}
	if accountID != nil {
		event.ResourceID = strconv.FormatInt(*accountID, 10)
	}
	return event
}

// IsUnsupportedMethod reports whether err came from a provider that does not
// implement the requested authentication method.
func IsUnsupportedMethod(err error) bool { return errors.Is(err, ErrUnsupportedMethod) }
