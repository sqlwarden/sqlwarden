package web

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	defaultBaseURL                 = "http://localhost:6020"
	defaultHTTPPort                = 6020
	defaultDeploymentMode          = DeploymentModeServer
	defaultAccessMode              = AccessModeMultiUser
	defaultPersonalSpacesEnabled   = true
	defaultCookieSecretKey         = "cpcgzjcote6h5hakeglpbzixhbuog2zc"
	defaultDBLogQueries            = false
	defaultDBDriver                = "sqlite"
	defaultDBDSN                   = "~/.sqlwarden/sqlwarden.db"
	defaultDBAutomigrate           = true
	defaultEncryptionKey           = "dev-insecure-key-32byteslong!!"
	defaultJWTSecretKey            = "fb57i5hiud5mzmykaquqsln5gcmolbac"
	defaultJWTAccessTokenTTL       = 24 * time.Hour
	defaultSessionRevocation       = true
	defaultTLSEnabled              = false
	defaultTLSCertFile             = ""
	defaultTLSKeyFile              = ""
	defaultFilesStorageMode        = FilesStorageModeObject
	defaultFilesActiveBackend      = "local"
	defaultFilesRootDir            = "~/.sqlwarden/files"
	defaultFilesRevisionPolicy     = FilesRevisionPolicyVersioned
	defaultFilesRevisionKeepLatest = 50
	defaultNotificationsEmail      = ""
	defaultSMTPHost                = "example.smtp.host"
	defaultSMTPPort                = 25
	defaultSMTPUsername            = "example_username"
	defaultSMTPPassword            = "pa55word"
	defaultSMTPFrom                = "Example Name <no_reply@example.org>"
	defaultDesktopAppDir           = ""
	defaultDesktopActiveBackend    = "local"
	defaultAllowUserBackends       = true
)

var defaultSQLiteDriverSources = []string{}

const (
	DeploymentModeServer  = "server"
	DeploymentModeDesktop = "desktop"
)

const (
	AccessModeMultiUser  = "multi_user"
	AccessModeSingleUser = "single_user"
)

const (
	DesktopBackendKindLocal  = "local"
	DesktopBackendKindRemote = "remote"
)

const (
	SQLiteDriverSourceLocal = "local"
)

const (
	FilesStorageModeFile          = "file"
	FilesStorageModeObject        = "object"
	FilesStorageBackendFilesystem = "filesystem"
	FilesStorageBackendS3         = "s3"
	FilesRevisionPolicyDisabled   = "disabled"
	FilesRevisionPolicyVersioned  = "versioned"
)

type Config struct {
	BaseURL               string
	HTTPPort              int
	DeploymentMode        string
	AccessMode            string
	PersonalSpacesEnabled bool
	Cookie                struct {
		SecretKey string
	}
	DB struct {
		LogQueries  bool
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
		SecretKey      string
		AccessTokenTTL time.Duration
	}
	Sessions struct {
		RevocationEnabled bool
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
		Revisions            struct {
			DefaultPolicy string
			KeepLatest    int
		}
	}
	Notifications struct {
		Email string
	}
	SMTP struct {
		Host     string
		Port     int
		Username string
		Password string
		From     string
	}
	Desktop struct {
		AppDir            string
		ActiveBackend     string
		AllowUserBackends bool
		Backends          []DesktopBackend
	}
}

type FileStorageBackend struct {
	Type    string `mapstructure:"type"`
	RootDir string `mapstructure:"root_dir"`
}

type DesktopBackend struct {
	ID          string `mapstructure:"id"`
	Name        string `mapstructure:"name"`
	Kind        string `mapstructure:"kind"`
	URL         string `mapstructure:"url"`
	Environment string `mapstructure:"environment"`
	AccessMode  string `mapstructure:"access_mode"`
	Locked      bool   `mapstructure:"locked"`
}

func DefaultConfig() Config {
	cfg := Config{}
	cfg.BaseURL = defaultBaseURL
	cfg.HTTPPort = defaultHTTPPort
	cfg.DeploymentMode = defaultDeploymentMode
	cfg.AccessMode = defaultAccessMode
	cfg.PersonalSpacesEnabled = defaultPersonalSpacesEnabled
	cfg.Cookie.SecretKey = defaultCookieSecretKey
	cfg.DB.LogQueries = defaultDBLogQueries
	cfg.DB.Driver = defaultDBDriver
	cfg.DB.DSN = defaultDBDSN
	cfg.DB.Automigrate = defaultDBAutomigrate
	cfg.Encryption.Key = defaultEncryptionKey
	cfg.JWT.SecretKey = defaultJWTSecretKey
	cfg.JWT.AccessTokenTTL = defaultJWTAccessTokenTTL
	cfg.Sessions.RevocationEnabled = defaultSessionRevocation
	cfg.TLS.Enabled = defaultTLSEnabled
	cfg.TLS.CertFile = defaultTLSCertFile
	cfg.TLS.KeyFile = defaultTLSKeyFile
	cfg.Drivers.SQLite.AllowedSources = append([]string(nil), defaultSQLiteDriverSources...)
	cfg.Files.StorageMode = defaultFilesStorageMode
	cfg.Files.ActiveStorageBackend = defaultFilesActiveBackend
	cfg.Files.StorageBackends = defaultFileStorageBackends()
	cfg.Files.Revisions.DefaultPolicy = defaultFilesRevisionPolicy
	cfg.Files.Revisions.KeepLatest = defaultFilesRevisionKeepLatest
	cfg.Notifications.Email = defaultNotificationsEmail
	cfg.SMTP.Host = defaultSMTPHost
	cfg.SMTP.Port = defaultSMTPPort
	cfg.SMTP.Username = defaultSMTPUsername
	cfg.SMTP.Password = defaultSMTPPassword
	cfg.SMTP.From = defaultSMTPFrom
	cfg.Desktop.AppDir = defaultDesktopAppDir
	cfg.Desktop.ActiveBackend = defaultDesktopActiveBackend
	cfg.Desktop.AllowUserBackends = defaultAllowUserBackends
	cfg.Desktop.Backends = defaultDesktopBackends()
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

func defaultDesktopBackends() []DesktopBackend {
	return []DesktopBackend{
		{
			ID:         "local",
			Name:       "Local",
			Kind:       DesktopBackendKindLocal,
			AccessMode: AccessModeSingleUser,
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
	{key: "base_url", env: "BASE_URL", flagName: "base-url", defaultValue: defaultBaseURL, usage: "Application base URL used in generated links and JWT claims"},
	{key: "http_port", env: "HTTP_PORT", flagName: "http-port", defaultValue: defaultHTTPPort, usage: "HTTP server port"},
	{key: "deployment_mode", env: "DEPLOYMENT_MODE", flagName: "deployment-mode", defaultValue: defaultDeploymentMode, usage: "Deployment mode (server or desktop)"},
	{key: "access_mode", env: "ACCESS_MODE", flagName: "access-mode", defaultValue: defaultAccessMode, usage: "Access model (multi_user or single_user)"},
	{key: "desktop_mode", env: "DESKTOP_MODE", flagName: "desktop-mode", defaultValue: false, usage: "Deprecated alias for deployment-mode=desktop and access-mode=single_user"},
	{key: "personal_spaces_enabled", env: "PERSONAL_SPACES_ENABLED", flagName: "personal-spaces-enabled", defaultValue: defaultPersonalSpacesEnabled, usage: "Enable personal spaces by default"},
	{key: "cookie.secret_key", env: "COOKIE_SECRET_KEY", flagName: "cookie-secret-key", defaultValue: defaultCookieSecretKey, usage: "Cookie signing secret"},
	{key: "db.log_queries", env: "DB_LOG_QUERIES", flagName: "db-log-queries", defaultValue: defaultDBLogQueries, usage: "Enable database query logging"},
	{key: "db.driver", env: "DB_DRIVER", flagName: "db-driver", defaultValue: defaultDBDriver, usage: "Database driver (sqlite or postgres)"},
	{key: "db.dsn", env: "DB_DSN", flagName: "db-dsn", defaultValue: defaultDBDSN, usage: "Database DSN"},
	{key: "db.automigrate", env: "DB_AUTOMIGRATE", flagName: "db-automigrate", defaultValue: defaultDBAutomigrate, usage: "Run database migrations at startup"},
	{key: "encryption.key", env: "ENCRYPTION_KEY", flagName: "encryption-key", defaultValue: defaultEncryptionKey, usage: "Application encryption key"},
	{key: "encryption.previous_keys", env: "ENCRYPTION_PREVIOUS_KEYS", flagName: "encryption-previous-keys", defaultValue: "", usage: "Comma-separated retired encryption keys retained for decryption during rotation"},
	{key: "jwt.secret_key", env: "JWT_SECRET_KEY", flagName: "jwt-secret-key", defaultValue: defaultJWTSecretKey, usage: "JWT signing secret"},
	{key: "jwt.access_token_ttl", env: "JWT_ACCESS_TOKEN_TTL", flagName: "jwt-access-token-ttl", defaultValue: defaultJWTAccessTokenTTL, usage: "JWT access token lifetime (for example: 24h, 8h, 30m)"},
	{key: "sessions.revocation_enabled", env: "SESSIONS_REVOCATION_ENABLED", flagName: "sessions-revocation-enabled", defaultValue: defaultSessionRevocation, usage: "Enable database-backed session revocation checks"},
	{key: "tls.enabled", env: "TLS_ENABLED", flagName: "tls-enabled", defaultValue: defaultTLSEnabled, usage: "Serve HTTPS using configured TLS certificate and key files"},
	{key: "tls.cert_file", env: "TLS_CERT_FILE", flagName: "tls-cert-file", defaultValue: defaultTLSCertFile, usage: "Path to PEM encoded TLS certificate file"},
	{key: "tls.key_file", env: "TLS_KEY_FILE", flagName: "tls-key-file", defaultValue: defaultTLSKeyFile, usage: "Path to PEM encoded TLS private key file"},
	{key: "drivers.sqlite.allowed_sources", env: "DRIVERS_SQLITE_ALLOWED_SOURCES", flagName: "drivers-sqlite-allowed-sources", defaultValue: defaultSQLiteDriverSources, usage: "Comma-separated SQLite target sources to allow (currently: local)"},
	{key: "files.storage_mode", env: "FILES_STORAGE_MODE", flagName: "files-storage-mode", defaultValue: defaultFilesStorageMode, usage: "Workspace file storage mode (file or object)"},
	{key: "files.active_storage_backend", env: "FILES_ACTIVE_STORAGE_BACKEND", flagName: "files-active-storage-backend", defaultValue: defaultFilesActiveBackend, usage: "Active storage backend ID for object-mode writes"},
	{key: "files.storage_backends.local.type", env: "FILES_STORAGE_BACKENDS_LOCAL_TYPE", flagName: "files-storage-backends-local-type", defaultValue: FilesStorageBackendFilesystem, usage: "Type for the default local file storage backend"},
	{key: "files.storage_backends.local.root_dir", env: "FILES_STORAGE_BACKENDS_LOCAL_ROOT_DIR", flagName: "files-storage-backends-local-root-dir", defaultValue: defaultFilesRootDir, usage: "Filesystem root directory for the default local file storage backend"},
	{key: "files.revisions.default_policy", env: "FILES_REVISIONS_DEFAULT_POLICY", flagName: "files-revisions-default-policy", defaultValue: defaultFilesRevisionPolicy, usage: "Default saved-file revision policy (disabled or versioned)"},
	{key: "files.revisions.keep_latest", env: "FILES_REVISIONS_KEEP_LATEST", flagName: "files-revisions-keep-latest", defaultValue: defaultFilesRevisionKeepLatest, usage: "Number of older saved-file revisions to retain per file"},
	{key: "notifications.email", env: "NOTIFICATIONS_EMAIL", flagName: "notifications-email", defaultValue: defaultNotificationsEmail, usage: "Email address that receives error notifications"},
	{key: "smtp.host", env: "SMTP_HOST", flagName: "smtp-host", defaultValue: defaultSMTPHost, usage: "SMTP server host"},
	{key: "smtp.port", env: "SMTP_PORT", flagName: "smtp-port", defaultValue: defaultSMTPPort, usage: "SMTP server port"},
	{key: "smtp.username", env: "SMTP_USERNAME", flagName: "smtp-username", defaultValue: defaultSMTPUsername, usage: "SMTP username"},
	{key: "smtp.password", env: "SMTP_PASSWORD", flagName: "smtp-password", defaultValue: defaultSMTPPassword, usage: "SMTP password"},
	{key: "smtp.from", env: "SMTP_FROM", flagName: "smtp-from", defaultValue: defaultSMTPFrom, usage: "Default SMTP sender"},
	{key: "desktop.app_dir", env: "DESKTOP_APP_DIR", flagName: "desktop-app-dir", defaultValue: defaultDesktopAppDir, usage: "Desktop application data directory"},
	{key: "desktop.active_backend", env: "DESKTOP_ACTIVE_BACKEND", flagName: "desktop-active-backend", defaultValue: defaultDesktopActiveBackend, usage: "Active desktop backend ID"},
	{key: "desktop.allow_user_backends", env: "DESKTOP_ALLOW_USER_BACKENDS", flagName: "desktop-allow-user-backends", defaultValue: defaultAllowUserBackends, usage: "Allow desktop users to add backends"},
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
		case time.Duration:
			flagSet.Duration(opt.flagName, value, opt.usage)
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
	v.SetDefault("desktop.backends", defaultDesktopBackends())
	v.SetDefault("files.storage_backends", defaultFileStorageBackends())

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
	cfg.BaseURL = v.GetString("base_url")
	cfg.HTTPPort = v.GetInt("http_port")
	cfg.DeploymentMode = v.GetString("deployment_mode")
	cfg.AccessMode = v.GetString("access_mode")
	if v.GetBool("desktop_mode") {
		cfg.DeploymentMode = DeploymentModeDesktop
		cfg.AccessMode = AccessModeSingleUser
	}
	cfg.PersonalSpacesEnabled = v.GetBool("personal_spaces_enabled")
	cfg.Cookie.SecretKey = v.GetString("cookie.secret_key")
	cfg.DB.LogQueries = v.GetBool("db.log_queries")
	cfg.DB.Driver = v.GetString("db.driver")
	cfg.DB.DSN = v.GetString("db.dsn")
	cfg.DB.Automigrate = v.GetBool("db.automigrate")
	cfg.Encryption.Key = v.GetString("encryption.key")
	cfg.Encryption.PreviousKeys = splitEncryptionKeys(v.GetString("encryption.previous_keys"))
	cfg.JWT.SecretKey = v.GetString("jwt.secret_key")
	cfg.JWT.AccessTokenTTL = v.GetDuration("jwt.access_token_ttl")
	cfg.Sessions.RevocationEnabled = v.GetBool("sessions.revocation_enabled")
	cfg.TLS.Enabled = v.GetBool("tls.enabled")
	cfg.TLS.CertFile = v.GetString("tls.cert_file")
	cfg.TLS.KeyFile = v.GetString("tls.key_file")
	cfg.Drivers.SQLite.AllowedSources = splitConfigStringList(v.GetStringSlice("drivers.sqlite.allowed_sources"))
	if cfg.DeploymentMode == DeploymentModeDesktop && cfg.AccessMode == AccessModeSingleUser {
		_, sourcesSetInEnv := os.LookupEnv("DRIVERS_SQLITE_ALLOWED_SOURCES")
		if !v.InConfig("drivers.sqlite.allowed_sources") && !flagSet.Changed("drivers-sqlite-allowed-sources") && !sourcesSetInEnv {
			cfg.Drivers.SQLite.AllowedSources = []string{SQLiteDriverSourceLocal}
		}
	}
	cfg.Files.StorageMode = v.GetString("files.storage_mode")
	cfg.Files.ActiveStorageBackend = v.GetString("files.active_storage_backend")
	cfg.Files.StorageBackends = defaultFileStorageBackends()
	if err := v.UnmarshalKey("files.storage_backends", &cfg.Files.StorageBackends); err != nil {
		return Config{}, false, fmt.Errorf("read file storage backends: %w", err)
	}
	localBackend := cfg.Files.StorageBackends[defaultFilesActiveBackend]
	_, localTypeSetInEnv := os.LookupEnv("FILES_STORAGE_BACKENDS_LOCAL_TYPE")
	_, localRootSetInEnv := os.LookupEnv("FILES_STORAGE_BACKENDS_LOCAL_ROOT_DIR")
	if v.InConfig("files.storage_backends.local.type") || flagSet.Changed("files-storage-backends-local-type") || localTypeSetInEnv {
		localBackend.Type = v.GetString("files.storage_backends.local.type")
	}
	if v.InConfig("files.storage_backends.local.root_dir") || flagSet.Changed("files-storage-backends-local-root-dir") || localRootSetInEnv {
		localBackend.RootDir = v.GetString("files.storage_backends.local.root_dir")
	}
	cfg.Files.StorageBackends[defaultFilesActiveBackend] = localBackend
	cfg.Files.Revisions.DefaultPolicy = v.GetString("files.revisions.default_policy")
	cfg.Files.Revisions.KeepLatest = v.GetInt("files.revisions.keep_latest")
	cfg.Notifications.Email = v.GetString("notifications.email")
	cfg.SMTP.Host = v.GetString("smtp.host")
	cfg.SMTP.Port = v.GetInt("smtp.port")
	cfg.SMTP.Username = v.GetString("smtp.username")
	cfg.SMTP.Password = v.GetString("smtp.password")
	cfg.SMTP.From = v.GetString("smtp.from")
	cfg.Desktop.AppDir = v.GetString("desktop.app_dir")
	cfg.Desktop.ActiveBackend = v.GetString("desktop.active_backend")
	cfg.Desktop.AllowUserBackends = v.GetBool("desktop.allow_user_backends")
	if err := v.UnmarshalKey("desktop.backends", &cfg.Desktop.Backends); err != nil {
		return Config{}, false, fmt.Errorf("read desktop backends: %w", err)
	}
	if len(cfg.Desktop.Backends) == 0 {
		cfg.Desktop.Backends = defaultDesktopBackends()
	}
	if len(cfg.Files.StorageBackends) == 0 {
		cfg.Files.StorageBackends = defaultFileStorageBackends()
	}
	if cfg.DeploymentMode == DeploymentModeDesktop {
		_, storageModeSetInEnv := os.LookupEnv("FILES_STORAGE_MODE")
		_, revisionPolicySetInEnv := os.LookupEnv("FILES_REVISIONS_DEFAULT_POLICY")
		if !v.InConfig("files.storage_mode") && !flagSet.Changed("files-storage-mode") && !storageModeSetInEnv {
			cfg.Files.StorageMode = FilesStorageModeFile
		}
		if !v.InConfig("files.revisions.default_policy") && !flagSet.Changed("files-revisions-default-policy") && !revisionPolicySetInEnv {
			cfg.Files.Revisions.DefaultPolicy = FilesRevisionPolicyDisabled
		}
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
	if cfg.DeploymentMode != DeploymentModeServer && cfg.DeploymentMode != DeploymentModeDesktop {
		return fmt.Errorf("deployment_mode must be %q or %q", DeploymentModeServer, DeploymentModeDesktop)
	}
	if cfg.AccessMode != AccessModeMultiUser && cfg.AccessMode != AccessModeSingleUser {
		return fmt.Errorf("access_mode must be %q or %q", AccessModeMultiUser, AccessModeSingleUser)
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
	if cfg.Files.StorageMode == FilesStorageModeFile && cfg.Files.Revisions.DefaultPolicy == FilesRevisionPolicyVersioned {
		return fmt.Errorf("files.revisions.default_policy=%q is not supported with files.storage_mode=%q yet", FilesRevisionPolicyVersioned, FilesStorageModeFile)
	}
	if cfg.Files.Revisions.DefaultPolicy != FilesRevisionPolicyDisabled && cfg.Files.Revisions.DefaultPolicy != FilesRevisionPolicyVersioned {
		return fmt.Errorf("files.revisions.default_policy must be %q or %q", FilesRevisionPolicyDisabled, FilesRevisionPolicyVersioned)
	}
	if cfg.Files.Revisions.KeepLatest < 0 {
		return fmt.Errorf("files.revisions.keep_latest must be greater than or equal to 0")
	}
	if err := validateFileStorageBackends(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Desktop.ActiveBackend) == "" {
		return fmt.Errorf("desktop.active_backend is required")
	}

	seenBackendIDs := map[string]struct{}{}
	activeBackendFound := false
	for _, backend := range cfg.Desktop.Backends {
		if strings.TrimSpace(backend.ID) == "" {
			return fmt.Errorf("desktop.backends[].id is required")
		}
		if _, exists := seenBackendIDs[backend.ID]; exists {
			return fmt.Errorf("desktop backend %q is duplicated", backend.ID)
		}
		seenBackendIDs[backend.ID] = struct{}{}

		if strings.TrimSpace(backend.Name) == "" {
			return fmt.Errorf("desktop backend %q name is required", backend.ID)
		}
		if backend.Kind != DesktopBackendKindLocal && backend.Kind != DesktopBackendKindRemote {
			return fmt.Errorf("desktop backend %q kind must be %q or %q", backend.ID, DesktopBackendKindLocal, DesktopBackendKindRemote)
		}
		if backend.Kind == DesktopBackendKindRemote && strings.TrimSpace(backend.URL) == "" {
			return fmt.Errorf("desktop remote backend %q url is required", backend.ID)
		}
		if backend.Kind == DesktopBackendKindLocal && strings.TrimSpace(backend.URL) != "" {
			return fmt.Errorf("desktop local backend %q must not set url", backend.ID)
		}
		if backend.AccessMode != "" && backend.AccessMode != AccessModeMultiUser && backend.AccessMode != AccessModeSingleUser {
			return fmt.Errorf("desktop backend %q access_mode must be %q or %q", backend.ID, AccessModeMultiUser, AccessModeSingleUser)
		}
		if backend.ID == cfg.Desktop.ActiveBackend {
			activeBackendFound = true
		}
	}
	if !activeBackendFound {
		return fmt.Errorf("desktop.active_backend %q must reference a configured backend", cfg.Desktop.ActiveBackend)
	}

	return nil
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
