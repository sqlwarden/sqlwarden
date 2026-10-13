package identity

import (
	"context"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// PasswordChangedReason is recorded on the sessions a password change ends.
const PasswordChangedReason = "password_changed"

// Page selects one page of a list.
type Page struct {
	Page     int
	PageSize int
}

// GetAccount returns the calling account.
func (s *Service) GetAccount(ctx context.Context, p access.Principal) (database.Account, error) {
	if err := requireAccount(p); err != nil {
		return database.Account{}, err
	}
	return s.loadAccount(ctx, p.Subject.ID)
}

type UpdateAccountInput struct {
	Name string
}

// UpdateAccount changes the calling account's profile.
func (s *Service) UpdateAccount(ctx context.Context, p access.Principal, in UpdateAccountInput) (database.Account, error) {
	if err := requireAccount(p); err != nil {
		return database.Account{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return database.Account{}, &ValidationError{Fields: map[string]string{"name": "Name is required."}}
	}
	return s.store.UpdateAccountName(ctx, p.Subject.ID, name)
}

type UpdatePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

// UpdatePassword changes the calling account's password after verifying the
// current one, and revokes every other session of the account. The session
// the request authenticated with stays valid.
func (s *Service) UpdatePassword(ctx context.Context, p access.Principal, in UpdatePasswordInput) error {
	if err := requireAccount(p); err != nil {
		return err
	}
	var v validator.Validator
	v.CheckField(in.CurrentPassword != "", "current_password", "Current password is required.")
	v.CheckField(len(in.NewPassword) >= 8, "new_password", "New password must be at least 8 characters.")
	if v.HasErrors() {
		return &ValidationError{Fields: v.FieldErrors}
	}

	account, err := s.loadAccount(ctx, p.Subject.ID)
	if err != nil {
		return err
	}
	if account.Password == nil {
		return &ValidationError{Fields: map[string]string{"current_password": "Password changes are not available for this account."}}
	}
	match, err := password.Matches(in.CurrentPassword, *account.Password)
	if err != nil {
		return err
	}
	if !match {
		return &ValidationError{Fields: map[string]string{"current_password": "Current password is incorrect."}}
	}
	hashed, err := password.Hash(in.NewPassword)
	if err != nil {
		return err
	}
	keep := ""
	if p.Credential.Kind == access.CredentialSession {
		keep = p.Credential.ID
	}
	return s.store.ChangeAccountPassword(ctx, account.ID, hashed, keep, PasswordChangedReason)
}

type ListOrgsInput struct {
	Search   string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// Orgs lists the organizations the calling account belongs to.
func (s *Service) Orgs(ctx context.Context, p access.Principal, in ListOrgsInput) (response.Paginated[database.AccountOrganizationListItem], error) {
	if err := requireAccount(p); err != nil {
		return response.Paginated[database.AccountOrganizationListItem]{}, err
	}
	return s.store.ListAccountOrgsPage(ctx, database.ListAccountOrgsParams{
		AccountID: p.Subject.ID,
		Search:    in.Search,
		Sort:      in.Sort,
		Order:     in.Order,
		Page:      in.Page,
		PageSize:  in.PageSize,
	})
}

// CurrentSession is the bootstrap view of the calling account.
type CurrentSession struct {
	Account         database.Account
	Organizations   []database.Organization
	IsInstanceAdmin bool
}

// CurrentSession returns the calling account, its organizations, and whether
// it administers the instance.
func (s *Service) CurrentSession(ctx context.Context, p access.Principal) (CurrentSession, error) {
	if err := requireAccount(p); err != nil {
		return CurrentSession{}, err
	}
	account, err := s.loadAccount(ctx, p.Subject.ID)
	if err != nil {
		return CurrentSession{}, err
	}
	orgs, err := s.store.GetAccountOrgs(ctx, account.ID)
	if err != nil {
		return CurrentSession{}, err
	}
	if orgs == nil {
		orgs = []database.Organization{}
	}
	return CurrentSession{Account: account, Organizations: orgs, IsInstanceAdmin: p.InstanceAdmin}, nil
}

// ListOwnSessions lists the calling account's auth sessions.
func (s *Service) ListOwnSessions(ctx context.Context, p access.Principal, page Page) (response.Paginated[database.AuthSession], error) {
	if err := requireAccount(p); err != nil {
		return response.Paginated[database.AuthSession]{}, err
	}
	return s.listSessions(ctx, p.Subject.ID, page)
}

// RevokeOwnSession revokes one of the calling account's auth sessions. A
// session of another account is reported as not found.
func (s *Service) RevokeOwnSession(ctx context.Context, p access.Principal, sessionID string) error {
	if err := requireAccount(p); err != nil {
		return err
	}
	return s.revokeSession(ctx, p.Subject.ID, sessionID, p.Subject.ID, "user_revoked")
}

// RevokeOwnSessions revokes every auth session of the calling account,
// including the current one.
func (s *Service) RevokeOwnSessions(ctx context.Context, p access.Principal) error {
	if err := requireAccount(p); err != nil {
		return err
	}
	revokedBy := p.Subject.ID
	return s.store.RevokeAuthSessionsForAccount(ctx, p.Subject.ID, &revokedBy, "user_revoked_all")
}

func (s *Service) loadAccount(ctx context.Context, id int64) (database.Account, error) {
	account, found, err := s.store.GetAccount(ctx, id)
	if err != nil {
		return database.Account{}, err
	}
	if !found {
		return database.Account{}, ErrNotFound
	}
	return account, nil
}

func (s *Service) listSessions(ctx context.Context, accountID int64, page Page) (response.Paginated[database.AuthSession], error) {
	return s.store.ListAuthSessionsPage(ctx, database.ListAuthSessionsParams{
		AccountID: accountID,
		Page:      page.Page,
		PageSize:  page.PageSize,
	})
}

// revokeSession revokes sessionID only when it belongs to accountID.
func (s *Service) revokeSession(ctx context.Context, accountID int64, sessionID string, revokedBy int64, reason string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrNotFound
	}
	session, found, err := s.store.GetAuthSession(ctx, sessionID, accountID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return s.store.RevokeAuthSession(ctx, session.ID, &revokedBy, reason)
}
