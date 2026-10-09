package app

import (
	"fmt"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/profile"
	"github.com/sqlwarden/internal/profile/desktop"
	"github.com/sqlwarden/internal/profile/server"
)

var profiles = map[string]func(server.OrganizationStore) profile.Profile{
	config.ProfileServer:  func(organizations server.OrganizationStore) profile.Profile { return server.New(organizations) },
	config.ProfileDesktop: func(server.OrganizationStore) profile.Profile { return desktop.New() },
}

// selectProfile returns the named profile. organizations backs the server
// profile's reveal policy and may be nil when only configuration hooks run.
func selectProfile(name string, organizations server.OrganizationStore) (profile.Profile, error) {
	newProfile, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", name)
	}
	return newProfile(organizations), nil
}
