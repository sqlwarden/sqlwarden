package identity

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

const (
	ActionInstanceAdminAdded   = "instance_admin.added"
	ActionInstanceAdminRemoved = "instance_admin.removed"
	ActionInstanceAccountAdded = "instance_account.created"
)

// ErrLastInstanceAdmin rejects removing the only remaining instance admin,
// which would leave the instance without anyone able to administer it.
var ErrLastInstanceAdmin = errors.New("identity: cannot remove the last instance admin")

// ListQuery is a validated paginated list request. Sort names a column the
// store accepts.
type ListQuery struct {
	Search   string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// RequireInstanceAdmin admits only principals the authentication chain marked
// as instance admins. Instance routes that have no service of their own use it
// to authorize.
func (s *Service) RequireInstanceAdmin(_ context.Context, p access.Principal) error {
	return requireInstanceAdmin(p)
}

// ListInstanceAdmins lists the instance admins to an instance admin.
func (s *Service) ListInstanceAdmins(ctx context.Context, p access.Principal, q ListQuery) (response.Paginated[database.InstanceAdmin], error) {
	if err := requireInstanceAdmin(p); err != nil {
		return response.Paginated[database.InstanceAdmin]{}, err
	}
	return s.store.ListInstanceAdminsPage(ctx, database.ListInstanceAdminsParams{
		Search: q.Search, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
}

// ListOrganizations lists every organization on the instance to an instance
// admin. A non-empty slug restricts the list to that slug.
func (s *Service) ListOrganizations(ctx context.Context, p access.Principal, q ListQuery, slug string) (response.Paginated[database.OrganizationListItem], error) {
	if err := requireInstanceAdmin(p); err != nil {
		return response.Paginated[database.OrganizationListItem]{}, err
	}
	return s.store.ListOrganizationsPage(ctx, database.ListOrganizationsParams{
		Search: q.Search, Slug: slug, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
}

// ListInstanceAccounts lists every account on the instance to an instance
// admin.
func (s *Service) ListInstanceAccounts(ctx context.Context, p access.Principal, q ListQuery) (response.Paginated[database.Account], error) {
	if err := requireInstanceAdmin(p); err != nil {
		return response.Paginated[database.Account]{}, err
	}
	return s.store.ListAccountsPage(ctx, database.ListAccountsParams{
		Search: q.Search, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
}

type CreateAccountInput struct {
	Name     string
	Email    string
	Password string
}

// CreateInstanceAccount lets an instance admin create a password account
// directly, without registration or an invitation.
func (s *Service) CreateInstanceAccount(ctx context.Context, p access.Principal, in CreateAccountInput) (database.Account, error) {
	if err := requireInstanceAdmin(p); err != nil {
		return database.Account{}, err
	}
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	var v validator.Validator
	v.CheckField(name != "", "name", "Name is required.")
	v.CheckField(email != "", "email", "Email is required.")
	v.CheckField(email == "" || validator.IsEmail(email), "email", "Enter a valid email address.")
	v.CheckField(len(in.Password) >= minPasswordLength, "password", "Password must be at least 8 characters.")
	if v.HasErrors() {
		return database.Account{}, &ValidationError{Fields: v.FieldErrors}
	}

	emailTaken := &ValidationError{Fields: map[string]string{"email": "An account with this email already exists."}}
	_, exists, err := s.store.GetAccountByEmail(ctx, email)
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
	account, err := s.store.InsertAccount(ctx, email, name, &hashed)
	if err != nil {
		if isUniqueViolation(err) {
			return database.Account{}, emailTaken
		}
		return database.Account{}, err
	}
	s.emitInstance(ctx, p, ActionInstanceAccountAdded, account.ID)
	return account, nil
}

// AddInstanceAdmin makes the account with email an instance admin. Adding an
// existing admin again succeeds without change.
func (s *Service) AddInstanceAdmin(ctx context.Context, p access.Principal, email string) (database.Account, error) {
	if err := requireInstanceAdmin(p); err != nil {
		return database.Account{}, err
	}
	if email == "" {
		return database.Account{}, &ValidationError{Fields: map[string]string{"email": "Email is required."}}
	}
	account, found, err := s.store.GetAccountByEmail(ctx, email)
	if err != nil {
		return database.Account{}, err
	}
	if !found {
		return database.Account{}, ErrNotFound
	}
	if err := s.store.InsertInstanceAdmin(ctx, account.ID); err != nil {
		return database.Account{}, err
	}
	s.emitInstance(ctx, p, ActionInstanceAdminAdded, account.ID)
	return account, nil
}

// RemoveInstanceAdmin revokes instance admin status from an account. The last
// remaining admin cannot be removed.
func (s *Service) RemoveInstanceAdmin(ctx context.Context, p access.Principal, accountID int64) error {
	if err := requireInstanceAdmin(p); err != nil {
		return err
	}
	if err := s.store.RemoveInstanceAdmin(ctx, accountID); err != nil {
		if errors.Is(err, database.ErrLastInstanceAdmin) {
			return ErrLastInstanceAdmin
		}
		return err
	}
	s.emitInstance(ctx, p, ActionInstanceAdminRemoved, accountID)
	return nil
}

// emitInstance records an instance administration event on a best-effort
// basis; the change has already committed.
func (s *Service) emitInstance(ctx context.Context, p access.Principal, action string, accountID int64) {
	err := audit.Emit(ctx, s.audit, audit.Event{
		Actor:      audit.ActorFromPrincipal(p),
		Action:     action,
		Resource:   "account",
		ResourceID: strconv.FormatInt(accountID, 10),
		Outcome:    audit.OutcomeSuccess,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "audit write failed", slog.String("action", action), slog.Any("error", err))
	}
}
