// Package desktop is the single-machine desktop profile.
package desktop

import (
	"errors"
	"path/filepath"
	"slices"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/profile"
)

const databaseFileName = "sqlwarden.db"

type desktopProfile struct{}

func New() profile.Profile { return desktopProfile{} }

func (desktopProfile) Name() string { return config.ProfileDesktop }

func (desktopProfile) Defaults(cfg *config.Config) {
	if cfg.Desktop.AppDir != "" && cfg.DB.DSN == config.Default().DB.DSN {
		cfg.DB.DSN = filepath.Join(cfg.Desktop.AppDir, databaseFileName)
	}
}

func (desktopProfile) Validate(cfg config.Config) error {
	if cfg.DB.Driver != "sqlite" {
		return errors.New("profile desktop requires db.driver sqlite")
	}
	if !slices.Equal(cfg.ProcessKinds, []string{config.ProcessKindAll}) {
		return errors.New("profile desktop requires process_kinds all")
	}
	if !cfg.DB.Automigrate {
		return errors.New("profile desktop requires db.automigrate true")
	}
	return nil
}

func (desktopProfile) Setup() identity.SetupStrategy      { return identity.LocalSetup }
func (desktopProfile) Invitations() orgs.InvitationPolicy { return orgs.InvitationsDisabled }

func (desktopProfile) SignIn(identity.AccountLookup) identity.SignInStrategy {
	return identity.SignInUnavailable
}
