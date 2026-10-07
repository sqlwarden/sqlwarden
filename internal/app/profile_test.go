package app

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/config"
)

func TestSelectProfile(t *testing.T) {
	for _, name := range []string{config.ProfileServer, config.ProfileDesktop} {
		p, err := selectProfile(name)
		if err != nil || p.Name() != name {
			t.Fatalf("selectProfile(%q) = %v, %v", name, p, err)
		}
	}
	if _, err := selectProfile("single_user"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRejectsDesktopWithPostgres(t *testing.T) {
	cfg := testConfig(t)
	cfg.Profile = config.ProfileDesktop
	cfg.DB.Driver = "postgres"
	_, err := Build(t.Context(), Options{Config: cfg})
	if err == nil || !strings.Contains(err.Error(), "profile desktop") {
		t.Fatalf("expected profile desktop validation error, got %v", err)
	}
}
