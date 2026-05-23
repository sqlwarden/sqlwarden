package web

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, showVersion, err := loadConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	if showVersion {
		t.Fatal("expected showVersion to be false")
	}

	if cfg.BaseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", cfg.BaseURL, defaultBaseURL)
	}
	if cfg.HTTPPort != defaultHTTPPort {
		t.Fatalf("httpPort = %d, want %d", cfg.HTTPPort, defaultHTTPPort)
	}
	if cfg.DB.Driver != defaultDBDriver {
		t.Fatalf("db.driver = %q, want %q", cfg.DB.Driver, defaultDBDriver)
	}
	if cfg.DB.DSN != defaultDBDSN {
		t.Fatalf("db.dsn = %q, want %q", cfg.DB.DSN, defaultDBDSN)
	}
	if !cfg.DB.Automigrate {
		t.Fatal("expected db.automigrate to default to true")
	}
	if !cfg.PersonalSpacesEnabled {
		t.Fatal("expected personal spaces to default to true")
	}
	if cfg.JWT.AccessTokenTTL != defaultJWTAccessTokenTTL {
		t.Fatalf("jwt.accessTokenTTL = %s, want %s", cfg.JWT.AccessTokenTTL, defaultJWTAccessTokenTTL)
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
jwt:
  access_token_ttl: 12h
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
	if cfg.BaseURL != "https://cfg.example.com" {
		t.Fatalf("baseURL = %q", cfg.BaseURL)
	}
	if cfg.HTTPPort != 7000 {
		t.Fatalf("httpPort = %d", cfg.HTTPPort)
	}
	if !cfg.DesktopMode {
		t.Fatal("expected desktopMode from file")
	}
	if cfg.PersonalSpacesEnabled {
		t.Fatal("expected personal spaces disabled from file")
	}
	if cfg.JWT.AccessTokenTTL != 12*time.Hour {
		t.Fatalf("jwt.accessTokenTTL = %s, want 12h", cfg.JWT.AccessTokenTTL)
	}
	if cfg.DB.Driver != "postgres" || cfg.DB.DSN != "cfg-dsn" || cfg.DB.Automigrate {
		t.Fatalf("unexpected db config: %+v", cfg.DB)
	}
	if cfg.SMTP.Host != "smtp.cfg.local" || cfg.SMTP.Port != 2525 {
		t.Fatalf("unexpected smtp config: %+v", cfg.SMTP)
	}
}

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("HTTP_PORT", "8123")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "6h")

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

	if cfg.HTTPPort != 8123 {
		t.Fatalf("httpPort = %d, want 8123", cfg.HTTPPort)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("db.driver = %q, want sqlite", cfg.DB.Driver)
	}
	if cfg.JWT.AccessTokenTTL != 6*time.Hour {
		t.Fatalf("jwt.accessTokenTTL = %s, want 6h", cfg.JWT.AccessTokenTTL)
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
		"--jwt-access-token-ttl", "2h",
	})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTTPPort != 9200 {
		t.Fatalf("httpPort = %d, want 9200", cfg.HTTPPort)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("db.driver = %q, want sqlite", cfg.DB.Driver)
	}
	if cfg.BaseURL != "https://flags.example.com" {
		t.Fatalf("baseURL = %q", cfg.BaseURL)
	}
	if cfg.JWT.AccessTokenTTL != 2*time.Hour {
		t.Fatalf("jwt.accessTokenTTL = %s, want 2h", cfg.JWT.AccessTokenTTL)
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
	if cfg.BaseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", cfg.BaseURL, defaultBaseURL)
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

	if cfg.BaseURL != "https://discovered.example.com" {
		t.Fatalf("baseURL = %q", cfg.BaseURL)
	}
	if cfg.DB.DSN != "discovered.db" {
		t.Fatalf("db.dsn = %q", cfg.DB.DSN)
	}
}
