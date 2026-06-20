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
	if cfg.Log.Level != LogLevelInfo || cfg.Log.Format != LogFormatJSON {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
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
	if !cfg.Sessions.RevocationEnabled {
		t.Fatal("expected session revocation to default to enabled")
	}
	if cfg.Desktop.ActiveBackend != "local" {
		t.Fatalf("desktop.active_backend = %q, want local", cfg.Desktop.ActiveBackend)
	}
	if cfg.Files.StorageMode != FilesStorageModeObject || cfg.Files.ActiveStorageBackend != "local" || !cfg.Files.Revisions.Enabled || cfg.Files.Revisions.KeepLatest != defaultFilesRevisionKeepLatest {
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
	if len(cfg.Drivers.SQLite.AllowedSources) != 0 {
		t.Fatalf("drivers.sqlite.allowed_sources = %v, want empty", cfg.Drivers.SQLite.AllowedSources)
	}
}

func TestLoadConfigDefaultsHaveNoPreviousEncryptionKeys(t *testing.T) {
	cfg, _, err := loadConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Encryption.PreviousKeys) != 0 {
		t.Fatalf("expected no previous keys by default, got %v", cfg.Encryption.PreviousKeys)
	}
}

func TestLoadConfigParsesPreviousEncryptionKeys(t *testing.T) {
	t.Setenv("ENCRYPTION_PREVIOUS_KEYS", " old-key-one , old-key-two ,, ")

	cfg, _, err := loadConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"old-key-one", "old-key-two"}
	if len(cfg.Encryption.PreviousKeys) != len(want) {
		t.Fatalf("previous keys = %v, want %v", cfg.Encryption.PreviousKeys, want)
	}
	for i, key := range want {
		if cfg.Encryption.PreviousKeys[i] != key {
			t.Errorf("previous key %d = %q, want %q", i, cfg.Encryption.PreviousKeys[i], key)
		}
	}
}

func TestLoadConfigFromExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
base_url: https://cfg.example.com
http_port: 7000
personal_spaces_enabled: false
jwt:
  access_token_ttl: 12h
sessions:
  revocation_enabled: false
log:
  level: warn
  format: text
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
  root_dir: /tmp/sqlwarden-files
  revisions:
    enabled: false
    keep_latest: 7
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
	if cfg.PersonalSpacesEnabled {
		t.Fatal("expected personal spaces disabled from file")
	}
	if cfg.JWT.AccessTokenTTL != 12*time.Hour {
		t.Fatalf("jwt.accessTokenTTL = %s, want 12h", cfg.JWT.AccessTokenTTL)
	}
	if cfg.Sessions.RevocationEnabled {
		t.Fatal("expected session revocation disabled from file")
	}
	if cfg.Log.Level != LogLevelWarn || cfg.Log.Format != LogFormatText {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
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
	if cfg.Files.StorageMode != FilesStorageModeObject || cfg.Files.ActiveStorageBackend != "local" {
		t.Fatalf("unexpected file storage config: %+v", cfg.Files)
	}
	if cfg.Files.Revisions.Enabled {
		t.Fatal("expected file revisions disabled from file")
	}
	if cfg.Files.Revisions.KeepLatest != 7 {
		t.Fatalf("files.revisions.keep_latest = %d, want 7", cfg.Files.Revisions.KeepLatest)
	}
	if cfg.Files.StorageBackends["local"].RootDir != "/tmp/sqlwarden-files" {
		t.Fatalf("unexpected local storage backend: %+v", cfg.Files.StorageBackends["local"])
	}
}

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("HTTP_PORT", "8123")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "6h")
	t.Setenv("FILES_ROOT_DIR", "/env/sqlwarden-files")
	t.Setenv("FILES_REVISIONS_ENABLED", "false")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
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
	if cfg.Files.Revisions.Enabled {
		t.Fatal("expected file revisions disabled from env")
	}
	if cfg.Log.Level != LogLevelDebug || cfg.Log.Format != LogFormatText {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
	}
	if cfg.Files.StorageBackends["local"].RootDir != "/env/sqlwarden-files" {
		t.Fatalf("files.root_dir = %q, want /env/sqlwarden-files", cfg.Files.StorageBackends["local"].RootDir)
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
		"--log-level", "error",
		"--log-format", "json",
		"--jwt-access-token-ttl", "2h",
		"--sessions-revocation-enabled=false",
		"--tls-enabled",
		"--tls-cert-file", "/flag/tls.crt",
		"--tls-key-file", "/flag/tls.key",
		"--files-root-dir", "/flag/sqlwarden-files",
		"--files-revisions-enabled=false",
		"--files-revisions-keep-latest", "3",
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
	if cfg.Log.Level != LogLevelError || cfg.Log.Format != LogFormatJSON {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
	}
	if cfg.JWT.AccessTokenTTL != 2*time.Hour {
		t.Fatalf("jwt.accessTokenTTL = %s, want 2h", cfg.JWT.AccessTokenTTL)
	}
	if cfg.Sessions.RevocationEnabled {
		t.Fatal("expected session revocation disabled from flag")
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/flag/tls.crt" || cfg.TLS.KeyFile != "/flag/tls.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
	}
	if cfg.Files.Revisions.Enabled {
		t.Fatal("expected file revisions disabled from flag")
	}
	if cfg.Files.StorageBackends["local"].RootDir != "/flag/sqlwarden-files" {
		t.Fatalf("files.root_dir = %q, want /flag/sqlwarden-files", cfg.Files.StorageBackends["local"].RootDir)
	}
	if cfg.Files.Revisions.KeepLatest != 3 {
		t.Fatalf("files.revisions.keep_latest = %d, want 3", cfg.Files.Revisions.KeepLatest)
	}
}

func TestLoadConfigRejectsInternalRuntimeFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--deployment-mode", DeploymentModeDesktop},
		{"--access-mode", AccessModeSingleUser},
		{"--desktop-mode"},
		{"--desktop-active-backend", "local"},
		{"--files-storage-mode", FilesStorageModeFile},
		{"--files-active-storage-backend", "local"},
		{"--files-storage-backends-local-type", FilesStorageBackendFilesystem},
		{"--files-storage-backends-local-root-dir", "/tmp/sqlwarden-files"},
	} {
		if _, _, err := loadConfig(args); err == nil {
			t.Fatalf("expected internal runtime flag %v to fail", args)
		}
	}
}

func TestLoadConfigRejectsUnsupportedFileConfiguration(t *testing.T) {
	_, _, err := loadConfig([]string{"--files-revisions-enabled=definitely"})
	if err == nil {
		t.Fatal("expected invalid revision boolean to fail")
	}

	_, _, err = loadConfig([]string{"--files-revisions-keep-latest", "-1"})
	if err == nil {
		t.Fatal("expected negative revision retention to fail")
	}
}

func TestLoadConfigRejectsUnsupportedLogConfiguration(t *testing.T) {
	_, _, err := loadConfig([]string{"--log-level", "verbose"})
	if err == nil {
		t.Fatal("expected unsupported log level to fail")
	}

	_, _, err = loadConfig([]string{"--log-format", "xml"})
	if err == nil {
		t.Fatal("expected unsupported log format to fail")
	}
}

func TestLoadConfigRejectsUnsupportedSQLiteTargetConfiguration(t *testing.T) {
	_, _, err := loadConfig([]string{"--drivers-sqlite-allowed-sources", "workspace_file"})
	if err == nil {
		t.Fatal("expected unimplemented sqlite target source to fail")
	}

	_, _, err = loadConfig([]string{"--drivers-sqlite-allowed-sources", "local,local"})
	if err == nil {
		t.Fatal("expected duplicate sqlite target source to fail")
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
