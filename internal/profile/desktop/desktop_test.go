package desktop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
)

func TestDesktopProfileStrategies(t *testing.T) {
	p := New()
	if p.Name() != config.ProfileDesktop {
		t.Fatalf("Name() = %q", p.Name())
	}
	if p.Setup() != identity.LocalSetup {
		t.Fatal("desktop must use LocalSetup")
	}
	if p.SignIn(nil).Enabled() {
		t.Fatal("desktop must not offer sign-in")
	}
	if p.Invitations() != orgs.InvitationsDisabled {
		t.Fatal("desktop must disable invitations")
	}
}

func TestDesktopRevealPolicyAlwaysAllows(t *testing.T) {
	allowed, err := New().RevealPolicy().Allowed(context.Background(), credentials.OrgRef{OrgID: "1"}, access.Principal{})
	if err != nil || !allowed {
		t.Fatalf("Allowed = %v, %v", allowed, err)
	}
}

func TestDesktopDefaultsDeriveDSNFromAppDir(t *testing.T) {
	cfg := config.Default()
	cfg.Desktop.AppDir = t.TempDir()
	New().Defaults(&cfg)
	if want := filepath.Join(cfg.Desktop.AppDir, "sqlwarden.db"); cfg.DB.DSN != want {
		t.Fatalf("DSN = %q, want %q", cfg.DB.DSN, want)
	}
}

func TestDesktopDefaultsKeepExplicitDSN(t *testing.T) {
	cfg := config.Default()
	cfg.Desktop.AppDir = t.TempDir()
	cfg.DB.DSN = "/explicit/app.db"
	New().Defaults(&cfg)
	if cfg.DB.DSN != "/explicit/app.db" {
		t.Fatalf("DSN = %q", cfg.DB.DSN)
	}
}

func TestDesktopValidate(t *testing.T) {
	p := New()
	ok := config.Default()
	if err := p.Validate(ok); err != nil {
		t.Fatalf("default config: %v", err)
	}
	cases := map[string]func(*config.Config){
		"postgres":   func(c *config.Config) { c.DB.Driver = "postgres" },
		"api only":   func(c *config.Config) { c.ProcessKinds = []string{config.ProcessKindAPI} },
		"no migrate": func(c *config.Config) { c.DB.Automigrate = false },
	}
	for name, mutate := range cases {
		cfg := config.Default()
		mutate(&cfg)
		if err := p.Validate(cfg); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
