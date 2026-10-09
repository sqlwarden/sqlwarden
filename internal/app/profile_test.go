package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/config"
)

func TestSelectProfile(t *testing.T) {
	for _, name := range []string{config.ProfileServer, config.ProfileDesktop} {
		p, err := selectProfile(name, nil)
		if err != nil || p.Name() != name {
			t.Fatalf("selectProfile(%q) = %v, %v", name, p, err)
		}
	}
	if _, err := selectProfile("single_user", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRejectsDesktopWithPostgres(t *testing.T) {
	cfg := testConfig(t)
	cfg.Profile = config.ProfileDesktop
	cfg.DB.Driver = "postgres"
	_, err := Build(t.Context(), Options{Config: cfg, Edition: testEdition()})
	if err == nil || !strings.Contains(err.Error(), "profile desktop") {
		t.Fatalf("expected profile desktop validation error, got %v", err)
	}
}

func TestBuildDesktopPlacesDatabaseInAppDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := testConfig(t)
	cfg.Profile = config.ProfileDesktop
	cfg.DB.DSN = config.Default().DB.DSN
	cfg.Desktop.AppDir = t.TempDir()
	built, err := Build(t.Context(), Options{Config: cfg, Command: CommandMigrate, Edition: testEdition()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = built.Close(context.Background()) })
	if _, err := os.Stat(filepath.Join(cfg.Desktop.AppDir, "sqlwarden.db")); err != nil {
		t.Fatalf("expected database in app dir: %v", err)
	}
}
