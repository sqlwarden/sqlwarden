// Package profile defines the product composition a process runs as. A
// profile selects strategies. It never contains handler logic, and handlers
// never inspect which profile is active.
package profile

import (
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
)

type Profile interface {
	Name() string
	// Defaults fills profile-derived values that the operator left at the
	// global default.
	Defaults(*config.Config)
	// Validate rejects configuration the profile cannot run with.
	Validate(config.Config) error
	Setup() identity.SetupStrategy
	Invitations() orgs.InvitationPolicy
	RevealPolicy() credentials.RevealPolicy
	// SignIn takes the account lookup port because the password method needs
	// storage and profiles hold no database.
	SignIn(accounts identity.AccountLookup) identity.SignInStrategy
}
