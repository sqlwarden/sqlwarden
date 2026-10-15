package orgs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
)

// InvitationPolicy reports whether organization invitations are available.
type InvitationPolicy interface {
	Enabled() bool
}

type invitationPolicy bool

func (p invitationPolicy) Enabled() bool { return bool(p) }

var (
	InvitationsEnabled  InvitationPolicy = invitationPolicy(true)
	InvitationsDisabled InvitationPolicy = invitationPolicy(false)
)

// InvitationTTL is how long an invitation token stays acceptable after it is
// issued or resent.
const InvitationTTL = 7 * 24 * time.Hour

const (
	ActionInvitationCreated  = "invitation.created"
	ActionInvitationAccepted = "invitation.accepted"

	invitationTemplate = "organization-invitation.tmpl"
)

var (
	// ErrMailDisabled is returned by a Mailer when email delivery is turned off.
	ErrMailDisabled = errors.New("orgs: email delivery is disabled")
	// ErrInvitationExpired reports an invitation whose token is past its expiry.
	ErrInvitationExpired = errors.New("orgs: invitation expired")
	// ErrAuthenticationRequired reports that the invitee's account exists and
	// the caller must sign in as it to accept.
	ErrAuthenticationRequired = errors.New("orgs: authentication required")
	// ErrSignOutRequired reports a signed-in caller accepting an invitation
	// that would create a new account.
	ErrSignOutRequired = errors.New("orgs: sign out before creating the invited account")
	// ErrInvitationUnavailable reports an invitation accepted, revoked, or
	// expired while it was being accepted.
	ErrInvitationUnavailable = errors.New("orgs: invitation is no longer available")
	// ErrInvitationAccountExists reports that the invitee's account was
	// created by someone else while a new account was being created for it.
	ErrInvitationAccountExists = errors.New("orgs: invitee account already exists")
)

// InvitationStore is the invitation metadata the service reads and writes.
type InvitationStore interface {
	ListOrganizationInvitationsPage(ctx context.Context, params database.ListOrganizationInvitationsParams) (response.Paginated[database.OrganizationInvitationListItem], error)
	InsertOrganizationInvitation(ctx context.Context, orgID int64, email, tokenHash string, invitedBy int64, expiresAt time.Time) (database.OrganizationInvitation, error)
	RotateOrganizationInvitation(ctx context.Context, orgID int64, id, tokenHash string, expiresAt time.Time) (database.OrganizationInvitation, bool, error)
	RevokeOrganizationInvitation(ctx context.Context, orgID int64, id string) (bool, error)
	UpdateOrganizationInvitationDelivery(ctx context.Context, id, status string) error
	GetOrganizationInvitationByTokenHash(ctx context.Context, tokenHash string) (database.OrganizationInvitation, bool, error)
	GetAccount(ctx context.Context, id int64) (database.Account, bool, error)
	GetAccountByEmail(ctx context.Context, email string) (database.Account, bool, error)
	// AcceptOrganizationInvitation creates newAccount when it is non-nil, adds
	// the account to the organization, and marks the invitation accepted in
	// one transaction. It returns the created account, and returns
	// ErrInvitationUnavailable when the invitation can no longer be accepted.
	AcceptOrganizationInvitation(ctx context.Context, invitation database.OrganizationInvitation, accountID int64, newAccount *InvitedAccount) (database.Account, error)
}

// InvitedAccount is the account created when an invitee without one accepts.
type InvitedAccount struct {
	Email        string
	Name         string
	PasswordHash string
}

// Mailer sends one templated email without retrying. It returns an error
// matching ErrMailDisabled when delivery is turned off.
type Mailer interface {
	SendOnce(recipient string, data any, templates ...string) error
}

// BaseURLSource returns the instance's public base URL, used to build links.
type BaseURLSource interface {
	BaseURL(ctx context.Context) (string, error)
}

type Invitation = database.OrganizationInvitation

type InvitationListItem = database.OrganizationInvitationListItem

// ListInvitationsParams filters and pages an invitation list.
type ListInvitationsParams = database.ListOrganizationInvitationsParams

// IssuedInvitation is an invitation with its freshly issued link. InviteURL
// carries the plaintext token and is returned only to the inviter.
type IssuedInvitation struct {
	Invitation     Invitation
	InviteURL      string
	DeliveryStatus string
}

// InvitationView is what the holder of an invitation token may see.
type InvitationView struct {
	Organization           Org
	Email                  string
	ExpiresAt              time.Time
	Status                 string
	AccountExists          bool
	AuthenticatedAsInvitee bool
}

// AcceptInvitationInput carries the new account's details. It is ignored
// when the invitee already has an account.
type AcceptInvitationInput struct {
	Name     string
	Password string
}

// AcceptedInvitation reports the joined organization and the accepting
// account. Created is set when the account was created by the acceptance.
type AcceptedInvitation struct {
	Organization Org
	Account      database.Account
	Created      bool
}

// ListInvitations returns a page of the organization's pending invitations.
func (s *Service) ListInvitations(ctx context.Context, p access.Principal, orgID int64, params ListInvitationsParams) (response.Paginated[InvitationListItem], error) {
	if err := s.requireInvitations(ctx, p, orgID); err != nil {
		return response.Paginated[InvitationListItem]{}, err
	}
	params.OrgID = orgID
	return s.invitations.ListOrganizationInvitationsPage(ctx, params)
}

// CreateInvitation invites email to the organization and emails the link.
// Delivery failures do not fail the invitation; the returned link can be
// shared by hand.
func (s *Service) CreateInvitation(ctx context.Context, p access.Principal, orgID int64, email string) (IssuedInvitation, error) {
	if err := s.requireInvitations(ctx, p, orgID); err != nil {
		return IssuedInvitation{}, err
	}
	email = strings.TrimSpace(email)
	fields := map[string]string{}
	if email == "" {
		addField(fields, "email", "Email is required.")
	}
	if !validator.IsEmail(email) {
		addField(fields, "email", "Enter a valid email address.")
	}
	if len(fields) != 0 {
		return IssuedInvitation{}, &ValidationError{Fields: fields}
	}

	org, err := s.loadOrg(ctx, orgID)
	if err != nil {
		return IssuedInvitation{}, err
	}
	if account, found, err := s.invitations.GetAccountByEmail(ctx, email); err != nil {
		return IssuedInvitation{}, err
	} else if found {
		member, err := s.store.IsOrgMember(ctx, orgID, account.ID)
		if err != nil {
			return IssuedInvitation{}, err
		}
		if member {
			return IssuedInvitation{}, &ValidationError{Fields: map[string]string{"email": "This account is already a member of the organization."}}
		}
	}

	plain, hash, err := token.Generate()
	if err != nil {
		return IssuedInvitation{}, err
	}
	inviteURL, err := s.invitationURL(ctx, plain)
	if err != nil {
		return IssuedInvitation{}, err
	}
	invitation, err := s.invitations.InsertOrganizationInvitation(ctx, orgID, email, hash, p.Subject.ID, time.Now().Add(InvitationTTL))
	if err != nil {
		if isUniqueViolation(err) {
			return IssuedInvitation{}, &ValidationError{Fields: map[string]string{"email": "A pending invitation already exists for this email."}}
		}
		return IssuedInvitation{}, err
	}
	issued := s.deliverInvitation(ctx, p, invitation, org, inviteURL)
	s.emitID(ctx, p, orgID, ActionInvitationCreated, "invitation", invitation.ID, nil)
	return issued, nil
}

// ResendInvitation issues a new token for a pending invitation, which
// invalidates the previous link, and emails it.
func (s *Service) ResendInvitation(ctx context.Context, p access.Principal, orgID int64, invitationID string) (IssuedInvitation, error) {
	if err := s.requireInvitations(ctx, p, orgID); err != nil {
		return IssuedInvitation{}, err
	}
	org, err := s.loadOrg(ctx, orgID)
	if err != nil {
		return IssuedInvitation{}, err
	}
	plain, hash, err := token.Generate()
	if err != nil {
		return IssuedInvitation{}, err
	}
	inviteURL, err := s.invitationURL(ctx, plain)
	if err != nil {
		return IssuedInvitation{}, err
	}
	invitation, found, err := s.invitations.RotateOrganizationInvitation(ctx, orgID, invitationID, hash, time.Now().Add(InvitationTTL))
	if err != nil {
		return IssuedInvitation{}, err
	}
	if !found {
		return IssuedInvitation{}, ErrNotFound
	}
	return s.deliverInvitation(ctx, p, invitation, org, inviteURL), nil
}

// RevokeInvitation revokes a pending invitation.
func (s *Service) RevokeInvitation(ctx context.Context, p access.Principal, orgID int64, invitationID string) error {
	if err := s.requireInvitations(ctx, p, orgID); err != nil {
		return err
	}
	revoked, err := s.invitations.RevokeOrganizationInvitation(ctx, orgID, invitationID)
	if err != nil {
		return err
	}
	if !revoked {
		return ErrNotFound
	}
	return nil
}

// GetInvitation describes the invitation plainToken identifies. The token is
// the credential, so no principal is required. viewerAccountID is the
// signed-in account, or 0 for an anonymous caller.
func (s *Service) GetInvitation(ctx context.Context, plainToken string, viewerAccountID int64) (InvitationView, error) {
	invitation, org, err := s.resolveInvitation(ctx, plainToken)
	if err != nil {
		return InvitationView{}, err
	}
	status := "pending"
	if !time.Now().Before(invitation.ExpiresAt) {
		status = "expired"
	}
	account, exists, err := s.invitations.GetAccountByEmail(ctx, invitation.Email)
	if err != nil {
		return InvitationView{}, err
	}
	return InvitationView{
		Organization:           org,
		Email:                  invitation.Email,
		ExpiresAt:              invitation.ExpiresAt,
		Status:                 status,
		AccountExists:          exists,
		AuthenticatedAsInvitee: exists && viewerAccountID == account.ID,
	}, nil
}

// AcceptInvitation joins the invitee to the organization. The token is the
// credential; viewerAccountID is the signed-in account, or 0 for an anonymous
// caller. An invitee with an account must be signed in as it. An invitee
// without one must be anonymous, and the account is created from the input.
// readInput is called only after the token is known to be acceptable, and its
// error is returned unchanged.
func (s *Service) AcceptInvitation(ctx context.Context, plainToken string, viewerAccountID int64, readInput func() (AcceptInvitationInput, error)) (AcceptedInvitation, error) {
	invitation, org, err := s.resolveInvitation(ctx, plainToken)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	if !time.Now().Before(invitation.ExpiresAt) {
		return AcceptedInvitation{}, ErrInvitationExpired
	}
	in, err := readInput()
	if err != nil {
		return AcceptedInvitation{}, err
	}

	account, exists, err := s.invitations.GetAccountByEmail(ctx, invitation.Email)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	var newAccount *InvitedAccount
	if exists {
		if viewerAccountID == 0 {
			return AcceptedInvitation{}, ErrAuthenticationRequired
		}
		if viewerAccountID != account.ID || !account.IsActive {
			return AcceptedInvitation{}, ErrForbidden
		}
	} else {
		if viewerAccountID != 0 {
			return AcceptedInvitation{}, ErrSignOutRequired
		}
		name := strings.TrimSpace(in.Name)
		fields := map[string]string{}
		if name == "" {
			addField(fields, "name", "Name is required.")
		}
		if len(in.Password) < 8 {
			addField(fields, "password", "Password must be at least 8 characters.")
		}
		if len(fields) != 0 {
			return AcceptedInvitation{}, &ValidationError{Fields: fields}
		}
		hashed, err := password.Hash(in.Password)
		if err != nil {
			return AcceptedInvitation{}, err
		}
		newAccount = &InvitedAccount{Email: invitation.Email, Name: name, PasswordHash: hashed}
	}

	joined, err := s.invitations.AcceptOrganizationInvitation(ctx, invitation, account.ID, newAccount)
	if err != nil {
		if newAccount != nil && isUniqueViolation(err) {
			return AcceptedInvitation{}, ErrInvitationAccountExists
		}
		return AcceptedInvitation{}, err
	}
	if newAccount != nil {
		account = joined
	}
	s.invalidatePrincipals(org.ID, account.ID)
	accepter := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: account.ID}}
	s.emitID(ctx, accepter, org.ID, ActionInvitationAccepted, "invitation", invitation.ID, nil)
	return AcceptedInvitation{Organization: org, Account: account, Created: newAccount != nil}, nil
}

func (s *Service) requireInvitations(ctx context.Context, p access.Principal, orgID int64) error {
	if !s.invitationsEnabled() {
		return ErrNotFound
	}
	return s.requireOrg(ctx, p, orgID, access.PermOrgInvite)
}

func (s *Service) invitationsEnabled() bool {
	return s.invitationPolicy != nil && s.invitationPolicy.Enabled()
}

// resolveInvitation finds the pending or expired invitation a token
// identifies. Unknown, accepted, and revoked invitations are ErrNotFound.
func (s *Service) resolveInvitation(ctx context.Context, plainToken string) (Invitation, Org, error) {
	if !s.invitationsEnabled() {
		return Invitation{}, Org{}, ErrNotFound
	}
	invitation, found, err := s.invitations.GetOrganizationInvitationByTokenHash(ctx, token.Hash(plainToken))
	if err != nil {
		return Invitation{}, Org{}, err
	}
	if !found || invitation.AcceptedAt != nil || invitation.RevokedAt != nil {
		return Invitation{}, Org{}, ErrNotFound
	}
	org, err := s.loadOrg(ctx, invitation.OrgID)
	if err != nil {
		return Invitation{}, Org{}, err
	}
	return invitation, org, nil
}

func (s *Service) invitationURL(ctx context.Context, plainToken string) (string, error) {
	baseURL, err := s.baseURL.BaseURL(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(baseURL, "/") + "/invitations/" + plainToken, nil
}

// deliverInvitation emails the invitation once and records the outcome. The
// link is never logged because it carries the token.
func (s *Service) deliverInvitation(ctx context.Context, p access.Principal, invitation Invitation, org Org, inviteURL string) IssuedInvitation {
	inviterName := ""
	if inviter, found, err := s.invitations.GetAccount(ctx, p.Subject.ID); err != nil {
		s.logger.WarnContext(ctx, "organization invitation inviter lookup failed", slog.String("invitation_id", invitation.ID), slog.Any("error", err))
	} else if found {
		inviterName = inviter.Name
	}

	status := database.InvitationDeliveryDisabled
	data := map[string]any{"OrganizationName": org.Name, "InviterName": inviterName, "InviteURL": inviteURL, "ExpiresAt": invitation.ExpiresAt}
	var err error
	if s.mailer == nil {
		err = ErrMailDisabled
	} else {
		err = s.mailer.SendOnce(invitation.Email, data, invitationTemplate)
	}
	switch {
	case err == nil:
		status = database.InvitationDeliverySent
	case !errors.Is(err, ErrMailDisabled):
		status = database.InvitationDeliveryFailed
		s.logger.WarnContext(ctx, "organization invitation email delivery failed", slog.String("invitation_id", invitation.ID), slog.Any("error", err))
	}
	if err := s.invitations.UpdateOrganizationInvitationDelivery(ctx, invitation.ID, status); err != nil {
		s.logger.ErrorContext(ctx, "organization invitation delivery status update failed", slog.String("invitation_id", invitation.ID), slog.Any("error", err))
	}
	invitation.LastDeliveryStatus = status
	return IssuedInvitation{Invitation: invitation, InviteURL: inviteURL, DeliveryStatus: status}
}
