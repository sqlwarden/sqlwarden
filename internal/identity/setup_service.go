package identity

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
)

// ErrAlreadyConfigured rejects first-run setup once the instance has an
// administrator. Setup seals itself after its first success.
var ErrAlreadyConfigured = errors.New("identity: instance is already configured")

// FirstRun is everything first-run setup writes. PasswordHash is nil for an
// account without a password credential.
type FirstRun struct {
	Email            string
	Name             string
	PasswordHash     *string
	OrgSlug          string
	OrgName          string
	SessionExpiresAt time.Time
	UserAgent        string
	IPAddress        string
	Auth             database.SessionAuth
}

type FirstRunRecords struct {
	Account     database.Account
	Org         database.Organization
	AuthSession database.AuthSession
}

// FirstRunStore writes the first account, makes it an instance admin, creates
// the organization it owns with seeded roles, and starts its auth session, all
// in one transaction. It returns ErrAlreadyConfigured when an instance admin
// already exists inside that transaction, and it serializes concurrent calls so
// at most one of them configures the instance.
type FirstRunStore interface {
	CreateFirstRun(ctx context.Context, in FirstRun) (FirstRunRecords, error)
}

// PolicyCache drops an organization's cached policy after its roles change.
type PolicyCache interface {
	InvalidateOrgPolicy(orgID int64)
}

// SetupState reports whether the instance is configured and whether the
// profile's setup needs input from the user.
type SetupState struct {
	Configured    bool
	RequiresInput bool
}

// SetupStatus reports the instance setup state. It runs before anyone can
// sign in, so it takes no principal.
func (s *Service) SetupStatus(ctx context.Context) (SetupState, error) {
	configured, err := s.store.HasAnyInstanceAdmin(ctx)
	if err != nil {
		return SetupState{}, err
	}
	return SetupState{Configured: configured, RequiresInput: s.setup.RequiresInput()}, nil
}

// SetupResult is the outcome of first-run setup: the admin account, its
// organization, and a signed-in session.
type SetupResult struct {
	Account       database.Account
	Org           database.Organization
	AuthSessionID string
	AccessToken   string
	Method        string
}

// Setup bootstraps an unconfigured instance through the profile's setup
// strategy: it creates the first account, makes it the instance admin, and
// creates the organization it owns. It runs before any account exists, so it
// takes no principal, and it fails with ErrAlreadyConfigured once any
// instance admin exists.
func (s *Service) Setup(ctx context.Context, in SetupInput, attrs access.RequestAttributes) (SetupResult, error) {
	if s.firstRun == nil {
		return SetupResult{}, errors.New("identity: first-run store is not configured")
	}
	plan, fieldErrors := s.setup.Plan(in)
	if len(fieldErrors) > 0 {
		return SetupResult{}, &ValidationError{Fields: fieldErrors}
	}

	configured, err := s.store.HasAnyInstanceAdmin(ctx)
	if err != nil {
		return SetupResult{}, err
	}
	if configured {
		return SetupResult{}, ErrAlreadyConfigured
	}

	slug := plan.OrganizationSlug
	_, found, err := s.store.GetOrgBySlug(ctx, slug)
	if err != nil {
		return SetupResult{}, err
	}
	if found {
		if !plan.SlugFallback {
			return SetupResult{}, &ValidationError{Fields: map[string]string{"organization_slug": "An organization with this slug already exists."}}
		}
		slug = plan.OrganizationSlug + "-" + strings.ToLower(database.NewID())
	}

	var hashed *string
	if plan.Password != nil {
		h, err := password.Hash(*plan.Password)
		if err != nil {
			return SetupResult{}, err
		}
		hashed = &h
	}

	records, err := s.firstRun.CreateFirstRun(ctx, FirstRun{
		Email: plan.AccountEmail, Name: plan.AccountName, PasswordHash: hashed,
		OrgSlug: slug, OrgName: plan.OrganizationName,
		SessionExpiresAt: s.now().Add(RefreshTokenTTL),
		UserAgent:        attrs.UserAgent, IPAddress: clientIP(attrs),
		Auth: database.SessionAuth{Method: plan.Method, Assurance: string(access.AAL1)},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return SetupResult{}, &ValidationError{Fields: map[string]string{"email": "An account or organization with these details already exists."}}
		}
		return SetupResult{}, err
	}
	if s.cache != nil {
		s.cache.InvalidateOrgPolicy(records.Org.ID)
	}

	accessToken, err := s.accessToken(ctx, records.Account, records.AuthSession.ID)
	if err != nil {
		return SetupResult{}, err
	}

	principal := access.Principal{
		Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: records.Account.ID},
		Credential: access.CredentialInfo{
			Kind: access.CredentialSession, ID: records.AuthSession.ID,
			Method: plan.Method, Assurance: access.AAL1,
		},
	}
	orgID := records.Org.ID
	if err := audit.Emit(ctx, s.audit, audit.Event{
		OrgID: &orgID, Actor: audit.ActorFromPrincipal(principal),
		Action: audit.ActionSetupCompleted, Resource: "organization",
		ResourceID: strconv.FormatInt(orgID, 10), Outcome: audit.OutcomeSuccess,
		Metadata: map[string]string{"client_ip": clientIP(attrs)},
	}); err != nil {
		s.logger.WarnContext(ctx, "audit write failed", slog.String("action", audit.ActionSetupCompleted), slog.Any("error", err))
	}

	return SetupResult{
		Account: records.Account, Org: records.Org,
		AuthSessionID: records.AuthSession.ID, AccessToken: accessToken, Method: plan.Method,
	}, nil
}
