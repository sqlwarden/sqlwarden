// Package server is the multi-user server profile.
package server

import (
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/profile"
)

type serverProfile struct{}

func New() profile.Profile { return serverProfile{} }

func (serverProfile) Name() string                       { return config.ProfileServer }
func (serverProfile) Defaults(*config.Config)            {}
func (serverProfile) Validate(config.Config) error       { return nil }
func (serverProfile) Setup() identity.SetupStrategy      { return identity.FormSetup }
func (serverProfile) Invitations() orgs.InvitationPolicy { return orgs.InvitationsEnabled }

func (serverProfile) SignIn(accounts identity.AccountLookup) identity.SignInStrategy {
	return identity.PasswordSignIn(accounts)
}
