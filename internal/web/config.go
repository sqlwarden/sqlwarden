package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	defaultBaseURL               = "http://localhost:6020"
	defaultHTTPPort              = 6020
	defaultDeploymentMode        = DeploymentModeServer
	defaultAccessMode            = AccessModeMultiUser
	defaultPersonalSpacesEnabled = true
	defaultCookieSecretKey       = "cpcgzjcote6h5hakeglpbzixhbuog2zc"
	defaultDBLogQueries          = false
	defaultDBDriver              = "sqlite"
	defaultDBDSN                 = "sqlwarden.db"
	defaultDBAutomigrate         = true
	defaultEncryptionKey         = "dev-insecure-key-32byteslong!!"
	defaultJWTSecretKey          = "fb57i5hiud5mzmykaquqsln5gcmolbac"
	defaultJWTAccessTokenTTL     = 24 * time.Hour
	defaultNotificationsEmail    = ""
	defaultSMTPHost              = "example.smtp.host"
	defaultSMTPPort              = 25
	defaultSMTPUsername          = "example_username"
	defaultSMTPPassword          = "pa55word"
	defaultSMTPFrom              = "Example Name <no_reply@example.org>"
	defaultDesktopAppDir         = ""
	defaultDesktopActiveBackend  = "local"
	defaultAllowUserBackends     = true
)

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
	}
	JWT struct {
		SecretKey      string
		AccessTokenTTL time.Duration
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
	{key: "jwt.secret_key", env: "JWT_SECRET_KEY", flagName: "jwt-secret-key", defaultValue: defaultJWTSecretKey, usage: "JWT signing secret"},
	{key: "jwt.access_token_ttl", env: "JWT_ACCESS_TOKEN_TTL", flagName: "jwt-access-token-ttl", defaultValue: defaultJWTAccessTokenTTL, usage: "JWT access token lifetime (for example: 24h, 8h, 30m)"},
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
	cfg.JWT.SecretKey = v.GetString("jwt.secret_key")
	cfg.JWT.AccessTokenTTL = v.GetDuration("jwt.access_token_ttl")
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
