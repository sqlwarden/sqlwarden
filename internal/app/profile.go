package app

import (
	"fmt"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/profile"
	"github.com/sqlwarden/internal/profile/desktop"
	"github.com/sqlwarden/internal/profile/server"
)

var profiles = map[string]func() profile.Profile{
	config.ProfileServer:  server.New,
	config.ProfileDesktop: desktop.New,
}

func selectProfile(name string) (profile.Profile, error) {
	newProfile, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", name)
	}
	return newProfile(), nil
}
