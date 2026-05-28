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
	if cfg.DeploymentMode != DeploymentModeServer {
		t.Fatalf("deploymentMode = %q, want %q", cfg.DeploymentMode, DeploymentModeServer)
	}
	if cfg.AccessMode != AccessModeMultiUser {
		t.Fatalf("accessMode = %q, want %q", cfg.AccessMode, AccessModeMultiUser)
	}
	if cfg.DB.Driver != defaultDBDriver {
		t.Fatalf("db.driver = %q, want %q", cfg.DB.Driver, defaultDBDriver)
	}
	defaultDBDSN, err := expandHomePath(defaultDBDSN)
	if err != nil {
		t.Fatal(err)
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
	if cfg.Desktop.ActiveBackend != "local" {
		t.Fatalf("desktop.active_backend = %q, want local", cfg.Desktop.ActiveBackend)
	}
	if cfg.Files.StorageMode != FilesStorageModeObject || cfg.Files.ActiveStorageBackend != "local" || cfg.Files.Revisions.DefaultPolicy != FilesRevisionPolicyVersioned {
		t.Fatalf("unexpected default file config: %+v", cfg.Files)
	}
	defaultFilesRoot, err := expandHomePath(defaultFilesRootDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Files.StorageBackends["local"].Type != FilesStorageBackendFilesystem || cfg.Files.StorageBackends["local"].RootDir != defaultFilesRoot {
		t.Fatalf("unexpected default storage backends: %+v", cfg.Files.StorageBackends)
	}
	if len(cfg.Desktop.Backends) != 1 || cfg.Desktop.Backends[0].ID != "local" || cfg.Desktop.Backends[0].Kind != DesktopBackendKindLocal {
		t.Fatalf("unexpected default desktop backends: %+v", cfg.Desktop.Backends)
	}
}

func TestLoadConfigFromExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
base_url: https://cfg.example.com
http_port: 7000
deployment_mode: desktop
access_mode: single_user
personal_spaces_enabled: false
jwt:
  access_token_ttl: 12h
tls:
  enabled: true
  cert_file: /etc/sqlwarden/tls.crt
  key_file: /etc/sqlwarden/tls.key
db:
  driver: postgres
  dsn: cfg-dsn
  automigrate: false
smtp:
  host: smtp.cfg.local
  port: 2525
files:
  storage_mode: object
  active_storage_backend: alternate
  storage_backends:
    local:
      type: filesystem
      root_dir: /tmp/sqlwarden-local-files
    alternate:
      type: filesystem
      root_dir: /tmp/sqlwarden-alternate-files
desktop:
  app_dir: /tmp/sqlwarden-desktop
  active_backend: acme-prod
  allow_user_backends: false
  backends:
    - id: local
      name: Local
      kind: local
      access_mode: single_user
    - id: acme-prod
      name: Acme Production
      kind: remote
      url: https://sqlwarden-prod.acme.test
      environment: prod
      access_mode: multi_user
      locked: true
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
	if cfg.DeploymentMode != DeploymentModeDesktop {
		t.Fatalf("deploymentMode = %q", cfg.DeploymentMode)
	}
	if cfg.AccessMode != AccessModeSingleUser {
		t.Fatalf("accessMode = %q", cfg.AccessMode)
	}
	if cfg.PersonalSpacesEnabled {
		t.Fatal("expected personal spaces disabled from file")
	}
	if cfg.JWT.AccessTokenTTL != 12*time.Hour {
		t.Fatalf("jwt.accessTokenTTL = %s, want 12h", cfg.JWT.AccessTokenTTL)
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/etc/sqlwarden/tls.crt" || cfg.TLS.KeyFile != "/etc/sqlwarden/tls.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
	}
	if cfg.DB.Driver != "postgres" || cfg.DB.DSN != "cfg-dsn" || cfg.DB.Automigrate {
		t.Fatalf("unexpected db config: %+v", cfg.DB)
	}
	if cfg.SMTP.Host != "smtp.cfg.local" || cfg.SMTP.Port != 2525 {
		t.Fatalf("unexpected smtp config: %+v", cfg.SMTP)
	}
	if cfg.Files.StorageMode != FilesStorageModeObject || cfg.Files.ActiveStorageBackend != "alternate" {
		t.Fatalf("unexpected file storage config: %+v", cfg.Files)
	}
	if cfg.Files.StorageBackends["alternate"].RootDir != "/tmp/sqlwarden-alternate-files" {
		t.Fatalf("unexpected alternate storage backend: %+v", cfg.Files.StorageBackends["alternate"])
	}
	if cfg.Desktop.AppDir != "/tmp/sqlwarden-desktop" || cfg.Desktop.ActiveBackend != "acme-prod" || cfg.Desktop.AllowUserBackends {
		t.Fatalf("unexpected desktop config: %+v", cfg.Desktop)
	}
	if len(cfg.Desktop.Backends) != 2 {
		t.Fatalf("desktop.backends length = %d, want 2", len(cfg.Desktop.Backends))
	}
	if cfg.Desktop.Backends[1].ID != "acme-prod" || cfg.Desktop.Backends[1].Kind != DesktopBackendKindRemote || cfg.Desktop.Backends[1].URL == "" || !cfg.Desktop.Backends[1].Locked {
		t.Fatalf("unexpected remote backend: %+v", cfg.Desktop.Backends[1])
	}
}

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("HTTP_PORT", "8123")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "6h")
	t.Setenv("ACCESS_MODE", AccessModeSingleUser)
	t.Setenv("TLS_ENABLED", "true")
	t.Setenv("TLS_CERT_FILE", "/env/tls.crt")
	t.Setenv("TLS_KEY_FILE", "/env/tls.key")

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
	if cfg.AccessMode != AccessModeSingleUser {
		t.Fatalf("accessMode = %q, want %q", cfg.AccessMode, AccessModeSingleUser)
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/env/tls.crt" || cfg.TLS.KeyFile != "/env/tls.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
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
		"--deployment-mode", DeploymentModeDesktop,
		"--access-mode", AccessModeSingleUser,
		"--tls-enabled",
		"--tls-cert-file", "/flag/tls.crt",
		"--tls-key-file", "/flag/tls.key",
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
	if cfg.DeploymentMode != DeploymentModeDesktop {
		t.Fatalf("deploymentMode = %q", cfg.DeploymentMode)
	}
	if cfg.AccessMode != AccessModeSingleUser {
		t.Fatalf("accessMode = %q", cfg.AccessMode)
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/flag/tls.crt" || cfg.TLS.KeyFile != "/flag/tls.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
	}
}

func TestLoadConfigDeprecatedDesktopModeAlias(t *testing.T) {
	cfg, _, err := loadConfig([]string{"--desktop-mode"})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DeploymentMode != DeploymentModeDesktop {
		t.Fatalf("deploymentMode = %q, want %q", cfg.DeploymentMode, DeploymentModeDesktop)
	}
	if cfg.AccessMode != AccessModeSingleUser {
		t.Fatalf("accessMode = %q, want %q", cfg.AccessMode, AccessModeSingleUser)
	}
}

func TestLoadConfigDesktopDefaultsToVisibleUnversionedFiles(t *testing.T) {
	cfg, _, err := loadConfig([]string{"--deployment-mode", DeploymentModeDesktop, "--access-mode", AccessModeSingleUser})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Files.StorageMode != FilesStorageModeFile || cfg.Files.Revisions.DefaultPolicy != FilesRevisionPolicyDisabled {
		t.Fatalf("unexpected desktop file config: %+v", cfg.Files)
	}
}

func TestLoadConfigRejectsUnsupportedFileConfiguration(t *testing.T) {
	_, _, err := loadConfig([]string{"--files-storage-backends-local-type", FilesStorageBackendS3})
	if err == nil {
		t.Fatal("expected unimplemented file storage backend to fail")
	}

	_, _, err = loadConfig([]string{"--files-revisions-default-policy", "sometimes"})
	if err == nil {
		t.Fatal("expected invalid revision policy to fail")
	}

	_, _, err = loadConfig([]string{"--files-storage-mode", FilesStorageModeFile})
	if err == nil {
		t.Fatal("expected file-mode versioning to fail until visible history is implemented")
	}

	_, _, err = loadConfig([]string{"--files-active-storage-backend", "missing"})
	if err == nil {
		t.Fatal("expected missing active storage backend to fail")
	}
}

func TestLoadConfigRejectsEnabledTLSWithoutCertOrKey(t *testing.T) {
	_, _, err := loadConfig([]string{"--tls-enabled"})
	if err == nil {
		t.Fatal("expected tls.enabled without cert/key to fail")
	}

	_, _, err = loadConfig([]string{"--tls-enabled", "--tls-cert-file", "/tmp/tls.crt"})
	if err == nil {
		t.Fatal("expected tls.enabled without key to fail")
	}

	_, _, err = loadConfig([]string{"--tls-enabled", "--tls-key-file", "/tmp/tls.key"})
	if err == nil {
		t.Fatal("expected tls.enabled without cert to fail")
	}
}

func TestLoadConfigRejectsInvalidDesktopBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
desktop:
  active_backend: prod
  backends:
    - id: prod
      name: Prod
      kind: remote
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := loadConfig([]string{"--config", path})
	if err == nil {
		t.Fatal("expected invalid remote backend without URL to fail")
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
