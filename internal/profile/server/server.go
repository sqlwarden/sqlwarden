// Package server is the multi-user server profile.
package server

import (
	"context"
	"errors"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/profile"
)

type OrganizationStore interface {
	GetOrg(ctx context.Context, id int64) (database.Organization, bool, error)
}

type serverProfile struct {
	organizations OrganizationStore
}

func New(stores ...OrganizationStore) profile.Profile {
	var store OrganizationStore
	if len(stores) != 0 {
		store = stores[0]
	}
	return serverProfile{organizations: store}
}

func (serverProfile) Name() string                       { return config.ProfileServer }
func (serverProfile) Defaults(*config.Config)            {}
func (serverProfile) Validate(config.Config) error       { return nil }
func (serverProfile) Setup() identity.SetupStrategy      { return identity.FormSetup }
func (serverProfile) Invitations() orgs.InvitationPolicy { return orgs.InvitationsEnabled }
func (p serverProfile) RevealPolicy() credentials.RevealPolicy {
	return serverRevealPolicy{organizations: p.organizations}
}

func (serverProfile) SignIn(accounts identity.AccountLookup) identity.SignInStrategy {
	return identity.PasswordSignIn(accounts)
}

type serverRevealPolicy struct {
	organizations OrganizationStore
}

func (p serverRevealPolicy) Allowed(ctx context.Context, ref credentials.OrgRef, _ access.Principal) (bool, error) {
	orgID, err := strconv.ParseInt(ref.OrgID, 10, 64)
	if err != nil || orgID <= 0 {
		return false, nil
	}
	if p.organizations == nil {
		return false, errors.New("server reveal policy: organization store is not configured")
	}
	org, found, err := p.organizations.GetOrg(ctx, orgID)
	if err != nil || !found {
		return false, err
	}
	return org.AllowConnectionSecretReveal, nil
}
