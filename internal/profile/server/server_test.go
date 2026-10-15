package server

import (
	"testing"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
)

func TestServerProfile(t *testing.T) {
	p := New()
	if p.Name() != config.ProfileServer {
		t.Fatalf("Name() = %q", p.Name())
	}
	if p.Setup() != identity.FormSetup {
		t.Fatal("server must use FormSetup")
	}
	if !p.SignIn(nil).Enabled() {
		t.Fatal("server must enable sign-in")
	}
	if p.Invitations() != orgs.InvitationsEnabled {
		t.Fatal("server must enable invitations")
	}
	cfg := config.Default()
	before := cfg
	p.Defaults(&cfg)
	if cfg.DB.DSN != before.DB.DSN {
		t.Fatal("server Defaults must not change config")
	}
	if err := p.Validate(cfg); err != nil {
		t.Fatal(err)
	}
}
