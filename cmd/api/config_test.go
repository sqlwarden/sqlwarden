package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, showVersion, err := loadConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	if showVersion {
		t.Fatal("expected showVersion to be false")
	}

	if cfg.baseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", cfg.baseURL, defaultBaseURL)
	}
	if cfg.httpPort != defaultHTTPPort {
		t.Fatalf("httpPort = %d, want %d", cfg.httpPort, defaultHTTPPort)
	}
	if cfg.db.driver != defaultDBDriver {
		t.Fatalf("db.driver = %q, want %q", cfg.db.driver, defaultDBDriver)
	}
	if cfg.db.dsn != defaultDBDSN {
		t.Fatalf("db.dsn = %q, want %q", cfg.db.dsn, defaultDBDSN)
	}
	if !cfg.db.automigrate {
		t.Fatal("expected db.automigrate to default to true")
	}
	if !cfg.personalSpacesEnabled {
		t.Fatal("expected personal spaces to default to true")
	}
}

func TestLoadConfigFromExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
base_url: https://cfg.example.com
http_port: 7000
desktop_mode: true
personal_spaces_enabled: false
db:
  driver: postgres
  dsn: cfg-dsn
  automigrate: false
smtp:
  host: smtp.cfg.local
  port: 2525
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, showVersion, err := loadConfig([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}

	if showVersion {
		t.Fatal("expected showVersion to be false")
	}
	if cfg.baseURL != "https://cfg.example.com" {
		t.Fatalf("baseURL = %q", cfg.baseURL)
	}
	if cfg.httpPort != 7000 {
		t.Fatalf("httpPort = %d", cfg.httpPort)
	}
	if !cfg.desktopMode {
		t.Fatal("expected desktopMode from file")
	}
	if cfg.personalSpacesEnabled {
		t.Fatal("expected personal spaces disabled from file")
	}
	if cfg.db.driver != "postgres" || cfg.db.dsn != "cfg-dsn" || cfg.db.automigrate {
		t.Fatalf("unexpected db config: %+v", cfg.db)
	}
	if cfg.smtp.host != "smtp.cfg.local" || cfg.smtp.port != 2525 {
		t.Fatalf("unexpected smtp config: %+v", cfg.smtp)
	}
}

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("HTTP_PORT", "8123")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
http_port: 7000
db:
  driver: postgres
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := loadConfig([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.httpPort != 8123 {
		t.Fatalf("httpPort = %d, want 8123", cfg.httpPort)
	}
	if cfg.db.driver != "sqlite" {
		t.Fatalf("db.driver = %q, want sqlite", cfg.db.driver)
	}
}

func TestLoadConfigFlagsOverrideEnvAndFile(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("HTTP_PORT", "8123")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
http_port: 7000
db:
  driver: sqlite
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := loadConfig([]string{
		"--config", path,
		"--http-port", "9200",
		"--db-driver", "sqlite",
		"--base-url", "https://flags.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.httpPort != 9200 {
		t.Fatalf("httpPort = %d, want 9200", cfg.httpPort)
	}
	if cfg.db.driver != "sqlite" {
		t.Fatalf("db.driver = %q, want sqlite", cfg.db.driver)
	}
	if cfg.baseURL != "https://flags.example.com" {
		t.Fatalf("baseURL = %q", cfg.baseURL)
	}
}

func TestLoadConfigVersionFlag(t *testing.T) {
	cfg, showVersion, err := loadConfig([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}

	if !showVersion {
		t.Fatal("expected showVersion to be true")
	}
	if cfg.baseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", cfg.baseURL, defaultBaseURL)
	}
}

func TestLoadConfigConventionalFileLookup(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	content := []byte(`
base_url: https://discovered.example.com
db:
  dsn: discovered.db
`)
	if err := os.WriteFile(filepath.Join(dir, "sqlwarden.yaml"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := loadConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.baseURL != "https://discovered.example.com" {
		t.Fatalf("baseURL = %q", cfg.baseURL)
	}
	if cfg.db.dsn != "discovered.db" {
		t.Fatalf("db.dsn = %q", cfg.db.dsn)
	}
}
