package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	loaded, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded.Config

	if loaded.ShowVersion {
		t.Fatal("expected ShowVersion to be false")
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
	if len(cfg.ProcessKinds) != 1 || cfg.ProcessKinds[0] != ProcessKindAll {
		t.Fatalf("processKinds = %v, want [%s]", cfg.ProcessKinds, ProcessKindAll)
	}
	if cfg.SessionDirectory != SessionDirectoryStatic {
		t.Fatalf("sessionDirectory = %q, want %q", cfg.SessionDirectory, SessionDirectoryStatic)
	}
	if cfg.Connector.Replicas != defaultConnectorReplicas {
		t.Fatalf("connector.replicas = %d, want %d", cfg.Connector.Replicas, defaultConnectorReplicas)
	}
	if cfg.Edition.Name != EditionCommunity || cfg.Edition.LicenseFile != "" {
		t.Fatalf("unexpected edition config: %+v", cfg.Edition)
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
}

func TestLoadDefaultsHaveNoPreviousEncryptionKeys(t *testing.T) {
	loaded, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Config.Encryption.PreviousKeys) != 0 {
		t.Fatalf("expected no previous keys by default, got %v", loaded.Config.Encryption.PreviousKeys)
	}
}

func TestLoadParsesPreviousEncryptionKeys(t *testing.T) {
	t.Setenv("ENCRYPTION_PREVIOUS_KEYS", " old-key-one , old-key-two ,, ")

	loaded, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"old-key-one", "old-key-two"}
	got := loaded.Config.Encryption.PreviousKeys
	if len(got) != len(want) {
		t.Fatalf("previous keys = %v, want %v", got, want)
	}
	for i, key := range want {
		if got[i] != key {
			t.Errorf("previous key %d = %q, want %q", i, got[i], key)
		}
	}
}

func TestLoadFromExplicitFile(t *testing.T) {
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

	loaded, err := Load([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded.Config

	if loaded.ShowVersion {
		t.Fatal("expected ShowVersion to be false")
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

func TestLoadEnvOverridesFile(t *testing.T) {
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

	loaded, err := Load([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded.Config

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

func TestLoadFlagsOverrideEnvAndFile(t *testing.T) {
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

	loaded, err := Load([]string{
		"--config", path,
		"--http-port", "9200",
		"--db-driver", "sqlite",
		"--base-url", "https://flags.example.com",
		"--deployment-mode", DeploymentModeDesktop,
		"--access-mode", AccessModeSingleUser,
		"--log-format", "json",
		"--tls-enabled",
		"--tls-cert-file", "/flag/tls.crt",
		"--tls-key-file", "/flag/tls.key",
		"--files-root-dir", "/flag/sqlwarden-files",
		"--files-storage-mode", FilesStorageModeFile,
		"--files-active-storage-backend", "local",
		"--desktop-app-dir", "/flag/desktop",
		"--desktop-active-backend", "local",
		"--desktop-allow-user-backends=false",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded.Config

	if cfg.HTTPPort != 9200 {
		t.Fatalf("httpPort = %d, want 9200", cfg.HTTPPort)
	}
	if cfg.DB.Driver != "sqlite" {
		t.Fatalf("db.driver = %q, want sqlite", cfg.DB.Driver)
	}
	if cfg.BootstrapBaseURL != "https://flags.example.com" {
		t.Fatalf("bootstrapBaseURL = %q", cfg.BootstrapBaseURL)
	}
	if cfg.DeploymentMode != DeploymentModeDesktop || cfg.AccessMode != AccessModeSingleUser {
		t.Fatalf("unexpected deployment/access modes: %q/%q", cfg.DeploymentMode, cfg.AccessMode)
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
	if cfg.Files.StorageMode != FilesStorageModeFile || cfg.Files.ActiveStorageBackend != "local" {
		t.Fatalf("unexpected file storage selection: %+v", cfg.Files)
	}
	if cfg.Desktop.AppDir != "/flag/desktop" || cfg.Desktop.ActiveBackend != "local" || cfg.Desktop.AllowUserBackends {
		t.Fatalf("unexpected desktop config: %+v", cfg.Desktop)
	}
}

func TestLoadRejectsInternalRuntimeFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--desktop-mode"},
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
		if _, err := Load(args); err == nil {
			t.Fatalf("expected internal runtime flag %v to fail", args)
		}
	}
}

func TestLoadRejectsUnsupportedFileConfiguration(t *testing.T) {
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
		if _, err := Load(args); err == nil {
			t.Fatalf("expected removed runtime flag %v to fail", args)
		}
	}
}

func TestLoadRejectsUnsupportedLogConfiguration(t *testing.T) {
	if _, err := Load([]string{"--log-level", "verbose"}); err == nil {
		t.Fatal("expected unsupported log level to fail")
	}
	if _, err := Load([]string{"--log-format", "xml"}); err == nil {
		t.Fatal("expected unsupported log format to fail")
	}
}

func TestLoadRejectsEnabledTLSWithoutCertOrKey(t *testing.T) {
	if _, err := Load([]string{"--tls-enabled"}); err == nil {
		t.Fatal("expected tls.enabled without cert/key to fail")
	}
	if _, err := Load([]string{"--tls-enabled", "--tls-cert-file", "/tmp/tls.crt"}); err == nil {
		t.Fatal("expected tls.enabled without key to fail")
	}
	if _, err := Load([]string{"--tls-enabled", "--tls-key-file", "/tmp/tls.key"}); err == nil {
		t.Fatal("expected tls.enabled without cert to fail")
	}
}

func TestLoadVersionFlag(t *testing.T) {
	loaded, err := Load([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}

	if !loaded.ShowVersion {
		t.Fatal("expected ShowVersion to be true")
	}
	if loaded.Config.BootstrapBaseURL != defaultBaseURL {
		t.Fatalf("bootstrapBaseURL = %q, want %q", loaded.Config.BootstrapBaseURL, defaultBaseURL)
	}
}

func TestLoadConventionalFileLookup(t *testing.T) {
	chdirTemp(t)

	content := []byte(`
base_url: https://discovered.example.com
db:
  dsn: discovered.db
`)
	if err := os.WriteFile("sqlwarden.yaml", content, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Config.BootstrapBaseURL != "https://discovered.example.com" {
		t.Fatalf("bootstrapBaseURL = %q", loaded.Config.BootstrapBaseURL)
	}
	if loaded.Config.DB.DSN != "discovered.db" {
		t.Fatalf("db.dsn = %q", loaded.Config.DB.DSN)
	}
}

// chdirTemp moves the test into an empty directory so conventional config file
// discovery cannot pick up a file from the repository.
func chdirTemp(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})
	return dir
}
