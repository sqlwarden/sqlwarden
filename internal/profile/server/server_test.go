package server

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
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

type fakeOrganizationStore struct {
	org   database.Organization
	found bool
	err   error
}

func (s fakeOrganizationStore) GetOrg(context.Context, int64) (database.Organization, bool, error) {
	return s.org, s.found, s.err
}

func TestServerRevealPolicyUsesOrganizationFlag(t *testing.T) {
	principal := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: 7}}
	for _, tt := range []struct {
		name  string
		store fakeOrganizationStore
		want  bool
	}{
		{name: "disabled", store: fakeOrganizationStore{org: database.Organization{}, found: true}},
		{name: "enabled", store: fakeOrganizationStore{org: database.Organization{AllowConnectionSecretReveal: true}, found: true}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := New(tt.store).RevealPolicy().Allowed(context.Background(), credentials.OrgRef{OrgID: "1"}, principal)
			if err != nil || allowed != tt.want {
				t.Fatalf("Allowed = %v, %v; want %v", allowed, err, tt.want)
			}
		})
	}
}

func TestServerRevealPolicyPropagatesStoreFailure(t *testing.T) {
	want := errors.New("down")
	_, err := New(fakeOrganizationStore{err: want}).RevealPolicy().Allowed(context.Background(), credentials.OrgRef{OrgID: "1"}, access.Principal{})
	if !errors.Is(err, want) {
		t.Fatalf("Allowed error = %v, want %v", err, want)
	}
}
