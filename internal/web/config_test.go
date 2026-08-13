package web

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

	if cfg.BootstrapBaseURL != defaultBaseURL {
		t.Fatalf("bootstrapBaseURL = %q, want %q", cfg.BootstrapBaseURL, defaultBaseURL)
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
	if cfg.Log.Format != LogFormatJSON {
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
	if cfg.Desktop.ActiveBackend != "local" {
		t.Fatalf("desktop.active_backend = %q, want local", cfg.Desktop.ActiveBackend)
	}
	if cfg.Files.StorageMode != FilesStorageModeObject || cfg.Files.ActiveStorageBackend != "local" {
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
log:
  format: text
tls:
  enabled: true
  cert_file: /etc/sqlwarden/tls.crt
  key_file: /etc/sqlwarden/tls.key
db:
  driver: postgres
  dsn: cfg-dsn
  automigrate: false
files:
  root_dir: /tmp/sqlwarden-files
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
	if cfg.BootstrapBaseURL != "https://cfg.example.com" {
		t.Fatalf("bootstrapBaseURL = %q", cfg.BootstrapBaseURL)
	}
	if cfg.HTTPPort != 7000 {
		t.Fatalf("httpPort = %d", cfg.HTTPPort)
	}
	if cfg.Log.Format != LogFormatText {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/etc/sqlwarden/tls.crt" || cfg.TLS.KeyFile != "/etc/sqlwarden/tls.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
	}
	if cfg.DB.Driver != "postgres" || cfg.DB.DSN != "cfg-dsn" || cfg.DB.Automigrate {
		t.Fatalf("unexpected db config: %+v", cfg.DB)
	}
	if cfg.Files.StorageMode != FilesStorageModeObject || cfg.Files.ActiveStorageBackend != "local" {
		t.Fatalf("unexpected file storage config: %+v", cfg.Files)
	}
	if cfg.Files.StorageBackends["local"].RootDir != "/tmp/sqlwarden-files" {
		t.Fatalf("unexpected local storage backend: %+v", cfg.Files.StorageBackends["local"])
	}
}

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("HTTP_PORT", "8123")
	t.Setenv("FILES_ROOT_DIR", "/env/sqlwarden-files")
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
	if cfg.Log.Format != LogFormatText {
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
		"--log-format", "json",
		"--tls-enabled",
		"--tls-cert-file", "/flag/tls.crt",
		"--tls-key-file", "/flag/tls.key",
		"--files-root-dir", "/flag/sqlwarden-files",
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
	if cfg.BootstrapBaseURL != "https://flags.example.com" {
		t.Fatalf("bootstrapBaseURL = %q", cfg.BootstrapBaseURL)
	}
	if cfg.Log.Format != LogFormatJSON {
		t.Fatalf("unexpected log config: %+v", cfg.Log)
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/flag/tls.crt" || cfg.TLS.KeyFile != "/flag/tls.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
	}
	if cfg.Files.StorageBackends["local"].RootDir != "/flag/sqlwarden-files" {
		t.Fatalf("files.root_dir = %q, want /flag/sqlwarden-files", cfg.Files.StorageBackends["local"].RootDir)
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
		{"--log-level", "debug"},
		{"--db-log-queries"},
		{"--jobs-worker-count", "2"},
		{"--jobs-poll-interval", "2s"},
		{"--jobs-claim-lease", "1m"},
		{"--jobs-completed-retention", "24h"},
		{"--smtp-enabled"},
		{"--smtp-host", "smtp.example.com"},
	} {
		if _, _, err := loadConfig(args); err == nil {
			t.Fatalf("expected internal runtime flag %v to fail", args)
		}
	}
}

func TestLoadConfigRejectsUnsupportedFileConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"--personal-spaces-enabled=false"},
		{"--jwt-access-token-ttl", "2h"},
		{"--sessions-revocation-enabled=false"},
		{"--query-max-result-rows", "100"},
		{"--query-max-result-bytes", "1000"},
		{"--exports-sync-max-bytes", "1000"},
		{"--schema-snapshot-freshness", "1h"},
		{"--files-revisions-enabled=false"},
		{"--files-revisions-keep-latest", "10"},
		{"--notifications-email", "errors@example.com"},
	} {
		if _, _, err := loadConfig(args); err == nil {
			t.Fatalf("expected removed runtime flag %v to fail", args)
		}
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
	if cfg.BootstrapBaseURL != defaultBaseURL {
		t.Fatalf("bootstrapBaseURL = %q, want %q", cfg.BootstrapBaseURL, defaultBaseURL)
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

	if cfg.BootstrapBaseURL != "https://discovered.example.com" {
		t.Fatalf("bootstrapBaseURL = %q", cfg.BootstrapBaseURL)
	}
	if cfg.DB.DSN != "discovered.db" {
		t.Fatalf("db.dsn = %q", cfg.DB.DSN)
	}
}
