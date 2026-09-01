package web

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/sqlwarden/internal/validator"
)

const (
	defaultBaseURL            = "http://localhost:6020"
	defaultHTTPPort           = 6020
	defaultMode               = ModeServer
	defaultLogFormat          = LogFormatJSON
	defaultCookieSecretKey    = "cpcgzjcote6h5hakeglpbzixhbuog2zc"
	defaultDBDriver           = "sqlite"
	defaultDBDSN              = "~/.sqlwarden/sqlwarden.db"
	defaultDBAutomigrate      = true
	defaultEncryptionKey      = "dev-insecure-key-32byteslong!!"
	defaultJWTSecretKey       = "fb57i5hiud5mzmykaquqsln5gcmolbac"
	defaultTLSEnabled         = false
	defaultTLSCertFile        = ""
	defaultTLSKeyFile         = ""
	defaultFilesStorageMode   = FilesStorageModeObject
	defaultFilesActiveBackend = "local"
	defaultFilesRootDir       = "~/.sqlwarden/files"
)

var defaultSQLiteDriverSources = []string{}

type Mode string

const (
	ModeServer  Mode = "server"
	ModeDesktop Mode = "desktop"
)

const (
	SQLiteDriverSourceLocal = "local"
)

const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

const (
	FilesStorageModeFile          = "file"
	FilesStorageModeObject        = "object"
	FilesStorageBackendFilesystem = "filesystem"
	FilesStorageBackendS3         = "s3"
)

type Config struct {
	BootstrapBaseURL string
	HTTPPort         int
	Mode             Mode
	Log              struct {
		Format string
	}
	Cookie struct {
		SecretKey string
	}
	DB struct {
		Driver      string
		DSN         string
		Automigrate bool
	}
	Encryption struct {
		Key string
		// PreviousKeys are retired encryption keys kept only so existing
		// ciphertext stays decryptable until it is rotated to the current key.
		PreviousKeys []string
	}
	JWT struct {
		SecretKey string
	}
	TLS struct {
		Enabled  bool
		CertFile string
		KeyFile  string
	}
	Drivers struct {
		SQLite struct {
			AllowedSources []string
		}
	}
	Files struct {
		StorageMode          string
		ActiveStorageBackend string
		StorageBackends      map[string]FileStorageBackend
	}
}

type FileStorageBackend struct {
	Type    string `mapstructure:"type"`
	RootDir string `mapstructure:"root_dir"`
}

func DefaultConfig() Config {
	cfg := Config{}
	cfg.BootstrapBaseURL = defaultBaseURL
	cfg.HTTPPort = defaultHTTPPort
	cfg.Mode = defaultMode
	cfg.Log.Format = defaultLogFormat
	cfg.Cookie.SecretKey = defaultCookieSecretKey
	cfg.DB.Driver = defaultDBDriver
	cfg.DB.DSN = defaultDBDSN
	cfg.DB.Automigrate = defaultDBAutomigrate
	cfg.Encryption.Key = defaultEncryptionKey
	cfg.JWT.SecretKey = defaultJWTSecretKey
	cfg.TLS.Enabled = defaultTLSEnabled
	cfg.TLS.CertFile = defaultTLSCertFile
	cfg.TLS.KeyFile = defaultTLSKeyFile
	cfg.Drivers.SQLite.AllowedSources = append([]string(nil), defaultSQLiteDriverSources...)
	cfg.Files.StorageMode = defaultFilesStorageMode
	cfg.Files.ActiveStorageBackend = defaultFilesActiveBackend
	cfg.Files.StorageBackends = defaultFileStorageBackends()
	return cfg
}

func defaultFileStorageBackends() map[string]FileStorageBackend {
	return map[string]FileStorageBackend{
		defaultFilesActiveBackend: {
			Type:    FilesStorageBackendFilesystem,
			RootDir: defaultFilesRootDir,
		},
	}
}

type configOption struct {
	key          string
	env          string
	flagName     string
	defaultValue any
	usage        string
}

var configOptions = []configOption{
	{key: "base_url", env: "BASE_URL", flagName: "base-url", defaultValue: defaultBaseURL, usage: "Initial instance base URL used only when bootstrapping runtime settings"},
	{key: "http_port", env: "HTTP_PORT", flagName: "http-port", defaultValue: defaultHTTPPort, usage: "HTTP server port"},
	{key: "log.format", env: "LOG_FORMAT", flagName: "log-format", defaultValue: defaultLogFormat, usage: "Log format (json or text)"},
	{key: "cookie.secret_key", env: "COOKIE_SECRET_KEY", flagName: "cookie-secret-key", defaultValue: defaultCookieSecretKey, usage: "Cookie signing secret"},
	{key: "db.driver", env: "DB_DRIVER", flagName: "db-driver", defaultValue: defaultDBDriver, usage: "Database driver (sqlite or postgres)"},
	{key: "db.dsn", env: "DB_DSN", flagName: "db-dsn", defaultValue: defaultDBDSN, usage: "Database DSN"},
	{key: "db.automigrate", env: "DB_AUTOMIGRATE", flagName: "db-automigrate", defaultValue: defaultDBAutomigrate, usage: "Run database migrations at startup"},
	{key: "encryption.key", env: "ENCRYPTION_KEY", flagName: "encryption-key", defaultValue: defaultEncryptionKey, usage: "Application encryption key"},
	{key: "encryption.previous_keys", env: "ENCRYPTION_PREVIOUS_KEYS", flagName: "encryption-previous-keys", defaultValue: "", usage: "Comma-separated retired encryption keys retained for decryption during rotation"},
	{key: "jwt.secret_key", env: "JWT_SECRET_KEY", flagName: "jwt-secret-key", defaultValue: defaultJWTSecretKey, usage: "JWT signing secret"},
	{key: "tls.enabled", env: "TLS_ENABLED", flagName: "tls-enabled", defaultValue: defaultTLSEnabled, usage: "Serve HTTPS using configured TLS certificate and key files"},
	{key: "tls.cert_file", env: "TLS_CERT_FILE", flagName: "tls-cert-file", defaultValue: defaultTLSCertFile, usage: "Path to PEM encoded TLS certificate file"},
	{key: "tls.key_file", env: "TLS_KEY_FILE", flagName: "tls-key-file", defaultValue: defaultTLSKeyFile, usage: "Path to PEM encoded TLS private key file"},
	{key: "drivers.sqlite.allowed_sources", env: "DRIVERS_SQLITE_ALLOWED_SOURCES", flagName: "drivers-sqlite-allowed-sources", defaultValue: defaultSQLiteDriverSources, usage: "Comma-separated SQLite target sources to allow (currently: local)"},
	{key: "files.root_dir", env: "FILES_ROOT_DIR", flagName: "files-root-dir", defaultValue: defaultFilesRootDir, usage: "Filesystem root directory for stored workspace files"},
}

func LoadConfig(args []string) (Config, bool, error) {
	return loadConfig(args)
}

func loadConfig(args []string) (Config, bool, error) {
	flagSet := pflag.NewFlagSet("sqlwarden", pflag.ContinueOnError)
	flagSet.SortFlags = false

	configPath := flagSet.String("config", "", "Path to a configuration file (yaml, yml, json, toml)")
	showVersion := flagSet.Bool("version", false, "Display version and exit")

	for _, opt := range configOptions {
		switch value := opt.defaultValue.(type) {
		case string:
			flagSet.String(opt.flagName, value, opt.usage)
		case int:
			flagSet.Int(opt.flagName, value, opt.usage)
		case bool:
			flagSet.Bool(opt.flagName, value, opt.usage)
		case []string:
			flagSet.StringSlice(opt.flagName, value, opt.usage)
		default:
			return Config{}, false, fmt.Errorf("unsupported config default type for %s", opt.key)
		}
	}

	if err := flagSet.Parse(args); err != nil {
		return Config{}, false, err
	}

	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	for _, opt := range configOptions {
		v.SetDefault(opt.key, opt.defaultValue)
		if err := v.BindEnv(opt.key, opt.env); err != nil {
			return Config{}, false, fmt.Errorf("bind env %s: %w", opt.env, err)
		}
		if err := v.BindPFlag(opt.key, flagSet.Lookup(opt.flagName)); err != nil {
			return Config{}, false, fmt.Errorf("bind flag %s: %w", opt.flagName, err)
		}
	}
	if *configPath != "" {
		v.SetConfigFile(*configPath)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, false, fmt.Errorf("read config file: %w", err)
		}
	} else {
		v.SetConfigName("sqlwarden")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		if err := v.MergeInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return Config{}, false, fmt.Errorf("read config file: %w", err)
			}
		}

		v.SetConfigName(".sqlwarden")
		if err := v.MergeInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return Config{}, false, fmt.Errorf("read config file: %w", err)
			}
		}
	}

	cfg := DefaultConfig()
	cfg.BootstrapBaseURL = v.GetString("base_url")
	cfg.HTTPPort = v.GetInt("http_port")
	cfg.Log.Format = strings.ToLower(strings.TrimSpace(v.GetString("log.format")))
	cfg.Cookie.SecretKey = v.GetString("cookie.secret_key")
	cfg.DB.Driver = v.GetString("db.driver")
	cfg.DB.DSN = v.GetString("db.dsn")
	cfg.DB.Automigrate = v.GetBool("db.automigrate")
	cfg.Encryption.Key = v.GetString("encryption.key")
	cfg.Encryption.PreviousKeys = splitEncryptionKeys(v.GetString("encryption.previous_keys"))
	cfg.JWT.SecretKey = v.GetString("jwt.secret_key")
	cfg.TLS.Enabled = v.GetBool("tls.enabled")
	cfg.TLS.CertFile = v.GetString("tls.cert_file")
	cfg.TLS.KeyFile = v.GetString("tls.key_file")
	cfg.Drivers.SQLite.AllowedSources = splitConfigStringList(v.GetStringSlice("drivers.sqlite.allowed_sources"))
	cfg.Files.StorageBackends = defaultFileStorageBackends()
	localBackend := cfg.Files.StorageBackends[defaultFilesActiveBackend]
	localBackend.RootDir = v.GetString("files.root_dir")
	cfg.Files.StorageBackends[defaultFilesActiveBackend] = localBackend
	if len(cfg.Files.StorageBackends) == 0 {
		cfg.Files.StorageBackends = defaultFileStorageBackends()
	}

	if err := normalizeConfigPaths(&cfg); err != nil {
		return Config{}, false, err
	}
	if err := validateConfig(cfg); err != nil {
		return Config{}, false, err
	}

	return cfg, *showVersion, nil
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.BootstrapBaseURL) == "" || !validator.IsURL(cfg.BootstrapBaseURL) {
		return fmt.Errorf("base_url must be a valid URL")
	}
	if cfg.Mode != ModeServer && cfg.Mode != ModeDesktop {
		return fmt.Errorf("mode must be %q or %q", ModeServer, ModeDesktop)
	}
	if !isSupportedLogFormat(cfg.Log.Format) {
		return fmt.Errorf("log.format must be %q or %q", LogFormatJSON, LogFormatText)
	}
	if cfg.TLS.Enabled {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" {
			return fmt.Errorf("tls.cert_file is required when tls.enabled is true")
		}
		if strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return fmt.Errorf("tls.key_file is required when tls.enabled is true")
		}
	}
	seenSQLiteSources := make(map[string]struct{}, len(cfg.Drivers.SQLite.AllowedSources))
	for _, source := range cfg.Drivers.SQLite.AllowedSources {
		if source != SQLiteDriverSourceLocal {
			return fmt.Errorf("drivers.sqlite.allowed_sources currently supports only %q", SQLiteDriverSourceLocal)
		}
		if _, ok := seenSQLiteSources[source]; ok {
			return fmt.Errorf("drivers.sqlite.allowed_sources contains duplicate source %q", source)
		}
		seenSQLiteSources[source] = struct{}{}
	}
	if cfg.Files.StorageMode != FilesStorageModeFile && cfg.Files.StorageMode != FilesStorageModeObject {
		return fmt.Errorf("files.storage_mode must be %q or %q", FilesStorageModeFile, FilesStorageModeObject)
	}
	if err := validateFileStorageBackends(cfg); err != nil {
		return err
	}
	return nil
}

func isSupportedLogLevel(level string) bool {
	switch level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return true
	default:
		return false
	}
}

func isSupportedLogFormat(format string) bool {
	switch format {
	case LogFormatJSON, LogFormatText:
		return true
	default:
		return false
	}
}

func validateFileStorageBackends(cfg Config) error {
	if cfg.Files.StorageMode == FilesStorageModeObject && strings.TrimSpace(cfg.Files.ActiveStorageBackend) == "" {
		return fmt.Errorf("files.active_storage_backend is required when files.storage_mode=%q", FilesStorageModeObject)
	}
	if len(cfg.Files.StorageBackends) == 0 {
		return fmt.Errorf("files.storage_backends must contain at least one backend")
	}

	for id, backend := range cfg.Files.StorageBackends {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("files.storage_backends contains an empty backend ID")
		}
		if backend.Type != FilesStorageBackendFilesystem {
			if backend.Type == FilesStorageBackendS3 {
				return fmt.Errorf("files.storage_backends.%s.type=%q is not implemented yet", id, FilesStorageBackendS3)
			}
			return fmt.Errorf("files.storage_backends.%s.type must be %q", id, FilesStorageBackendFilesystem)
		}
		if strings.TrimSpace(backend.RootDir) == "" {
			return fmt.Errorf("files.storage_backends.%s.root_dir is required", id)
		}
	}

	if cfg.Files.StorageMode == FilesStorageModeObject {
		if _, ok := cfg.Files.StorageBackends[cfg.Files.ActiveStorageBackend]; !ok {
			return fmt.Errorf("files.active_storage_backend %q must reference a configured storage backend", cfg.Files.ActiveStorageBackend)
		}
		return nil
	}

	if _, ok := cfg.Files.StorageBackends[defaultFilesActiveBackend]; !ok {
		return fmt.Errorf("files.storage_backends.%s is required when files.storage_mode=%q", defaultFilesActiveBackend, FilesStorageModeFile)
	}
	return nil
}

func normalizeConfigPaths(cfg *Config) error {
	var err error
	if cfg.DB.Driver == "sqlite" {
		cfg.DB.DSN, err = expandHomePath(cfg.DB.DSN)
		if err != nil {
			return fmt.Errorf("expand db.dsn: %w", err)
		}
	}
	for id, backend := range cfg.Files.StorageBackends {
		if backend.Type != FilesStorageBackendFilesystem {
			continue
		}
		backend.RootDir, err = expandHomePath(backend.RootDir)
		if err != nil {
			return fmt.Errorf("expand files.storage_backends.%s.root_dir: %w", id, err)
		}
		cfg.Files.StorageBackends[id] = backend
	}
	return nil
}

// splitEncryptionKeys parses a comma-separated list of retired encryption keys,
// trimming whitespace and dropping empty entries.
func splitEncryptionKeys(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var keys []string
	for _, part := range strings.Split(raw, ",") {
		if key := strings.TrimSpace(part); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func splitConfigStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if item := strings.TrimSpace(part); item != "" {
				result = append(result, item)
			}
		}
	}
	return result
}

func expandHomePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}
