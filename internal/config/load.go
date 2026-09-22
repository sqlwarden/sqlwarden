package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Source identifies where an effective configuration value came from.
type Source string

// Configuration sources, ordered from highest to lowest precedence.
const (
	SourceFlag       Source = "flag"
	SourceEnv        Source = "env"
	SourceSecretFile Source = "secret-file"
	SourceConfigFile Source = "config-file"
	SourceDefault    Source = "default"
)

// precedence documents, highest first, the order in which sources win. It is
// exported through [Diagnostic] so operators can see the rules alongside the
// resolved values.
var precedence = []Source{SourceFlag, SourceEnv, SourceSecretFile, SourceConfigFile, SourceDefault}

// secretFileEnvSuffix is the conventional suffix for an environment variable
// that names a file holding a secret instead of the secret itself.
const secretFileEnvSuffix = "_FILE"

// Loaded is the result of resolving bootstrap configuration from all sources.
type Loaded struct {
	Config      Config
	ShowVersion bool
	// Diagnostic reports the effective value and origin of every key with
	// sensitive values redacted.
	Diagnostic Diagnostic
}

// Load resolves bootstrap configuration from CLI flags, environment variables,
// mounted secret files, and configuration files, then normalizes and validates
// the result.
//
// Precedence, highest first:
//
//  1. CLI flag, when the flag was explicitly passed.
//  2. Environment variable, when set to a non-empty value.
//  3. Mounted secret file, named by "<ENV>_FILE" or found in "secrets.dir".
//  4. Configuration file.
//  5. Built-in default.
//
// The configuration file is the path given by --config, or the first of
// "sqlwarden" and ".sqlwarden" found in the working directory or ./config.
func Load(args []string) (Loaded, error) {
	flagSet := pflag.NewFlagSet("sqlwarden", pflag.ContinueOnError)
	flagSet.SortFlags = false

	configPath := flagSet.String("config", "", "Path to a configuration file (yaml, yml, json, toml)")
	showVersion := flagSet.Bool("version", false, "Display version and exit")

	for _, opt := range options {
		switch value := opt.defaultValue.(type) {
		case string:
			flagSet.String(opt.flagName, value, opt.usage)
		case int:
			flagSet.Int(opt.flagName, value, opt.usage)
		case bool:
			flagSet.Bool(opt.flagName, value, opt.usage)
		case []string:
			flagSet.StringSlice(opt.flagName, value, opt.usage)
		case time.Duration:
			flagSet.Duration(opt.flagName, value, opt.usage)
		default:
			return Loaded{}, fmt.Errorf("unsupported config default type for %s", opt.key)
		}
	}

	if err := flagSet.Parse(args); err != nil {
		return Loaded{}, err
	}

	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	for _, opt := range options {
		v.SetDefault(opt.key, opt.defaultValue)
		if err := v.BindEnv(opt.key, opt.env); err != nil {
			return Loaded{}, fmt.Errorf("bind env %s: %w", opt.env, err)
		}
		if err := v.BindPFlag(opt.key, flagSet.Lookup(opt.flagName)); err != nil {
			return Loaded{}, fmt.Errorf("bind flag %s: %w", opt.flagName, err)
		}
	}
	if err := readConfigFiles(v, *configPath); err != nil {
		return Loaded{}, err
	}

	// The secrets directory itself must resolve before it can be used to
	// resolve other keys, and it is never sourced from a secret file.
	secretsDir := v.GetString("secrets.dir")

	sources := make(map[string]Source, len(options))
	for _, opt := range options {
		source, secretValue, err := resolveSource(v, flagSet, opt, secretsDir)
		if err != nil {
			return Loaded{}, err
		}
		if source == SourceSecretFile {
			// viper.Set outranks every bound source, which is correct here
			// only because a secret file is consulted after flags and env.
			v.Set(opt.key, secretValue)
		}
		sources[opt.key] = source
	}

	cfg := configFromViper(v)
	if err := Normalize(&cfg); err != nil {
		return Loaded{}, err
	}
	if err := Validate(cfg); err != nil {
		return Loaded{}, err
	}

	return Loaded{
		Config:      cfg,
		ShowVersion: *showVersion,
		Diagnostic:  newDiagnostic(v, sources),
	}, nil
}

func readConfigFiles(v *viper.Viper, configPath string) error {
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read config file: %w", err)
		}
		return nil
	}

	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	for _, name := range []string{"sqlwarden", ".sqlwarden"} {
		v.SetConfigName(name)
		if err := v.MergeInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return fmt.Errorf("read config file: %w", err)
			}
		}
	}
	return nil
}

// resolveSource determines which source supplies opt, and returns the secret
// file contents when a mounted secret file wins.
func resolveSource(v *viper.Viper, flagSet *pflag.FlagSet, opt option, secretsDir string) (Source, string, error) {
	if flag := flagSet.Lookup(opt.flagName); flag != nil && flag.Changed {
		return SourceFlag, "", nil
	}
	if value, ok := os.LookupEnv(opt.env); ok && value != "" {
		return SourceEnv, "", nil
	}
	if opt.key != "secrets.dir" {
		value, found, err := readSecretFile(opt, secretsDir)
		if err != nil {
			return "", "", err
		}
		if found {
			return SourceSecretFile, value, nil
		}
	}
	if v.InConfig(opt.key) {
		return SourceConfigFile, "", nil
	}
	return SourceDefault, "", nil
}

// readSecretFile looks for a mounted secret in "<ENV>_FILE" first, then in the
// secrets directory under the configuration key with separators normalized to
// underscores. Trailing newlines written by secret mounts are trimmed.
func readSecretFile(opt option, secretsDir string) (string, bool, error) {
	if path, ok := os.LookupEnv(opt.env + secretFileEnvSuffix); ok && strings.TrimSpace(path) != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", false, fmt.Errorf("read secret file for %s: %w", opt.key, err)
		}
		return strings.TrimRight(string(contents), "\r\n"), true, nil
	}

	secretsDir = strings.TrimSpace(secretsDir)
	if secretsDir == "" {
		return "", false, nil
	}
	path := filepath.Join(secretsDir, secretFileName(opt.key))
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read secret file for %s: %w", opt.key, err)
	}
	return strings.TrimRight(string(contents), "\r\n"), true, nil
}

func secretFileName(key string) string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(key)
}

func configFromViper(v *viper.Viper) Config {
	cfg := Default()
	cfg.BootstrapBaseURL = v.GetString("base_url")
	cfg.HTTPPort = v.GetInt("http_port")
	cfg.DeploymentMode = strings.ToLower(strings.TrimSpace(v.GetString("deployment_mode")))
	cfg.AccessMode = strings.ToLower(strings.TrimSpace(v.GetString("access_mode")))
	cfg.ProcessKinds = splitStringList(v.GetStringSlice("process_kinds"))
	cfg.SessionDirectory = strings.ToLower(strings.TrimSpace(v.GetString("session_directory")))
	cfg.Connector.Replicas = v.GetInt("connector.replicas")
	cfg.Connector.Address = strings.TrimSpace(v.GetString("connector.address"))
	cfg.Connector.ListenAddress = strings.TrimSpace(v.GetString("connector.listen_address"))
	cfg.Connector.HealthAddress = strings.TrimSpace(v.GetString("connector.health_address"))
	cfg.Connector.Transport = strings.ToLower(strings.TrimSpace(v.GetString("connector.transport")))
	cfg.Connector.GrantSigningKey = v.GetString("connector.grant_signing_key")
	cfg.Connector.TLS.CAFile = v.GetString("connector.tls.ca_file")
	cfg.Connector.TLS.CertFile = v.GetString("connector.tls.cert_file")
	cfg.Connector.TLS.KeyFile = v.GetString("connector.tls.key_file")
	cfg.Connector.TLS.ServerName = v.GetString("connector.tls.server_name")
	cfg.Log.Format = strings.ToLower(strings.TrimSpace(v.GetString("log.format")))
	cfg.Cookie.SecretKey = v.GetString("cookie.secret_key")
	cfg.DB.Driver = v.GetString("db.driver")
	cfg.DB.DSN = v.GetString("db.dsn")
	cfg.DB.Automigrate = v.GetBool("db.automigrate")
	cfg.DB.MigrationTimeout = v.GetDuration("db.migration_timeout")
	cfg.ShutdownTimeout = v.GetDuration("shutdown_timeout")
	cfg.Encryption.Key = v.GetString("encryption.key")
	cfg.Encryption.PreviousKeys = splitCommaList(v.GetString("encryption.previous_keys"))
	cfg.JWT.SecretKey = v.GetString("jwt.secret_key")
	cfg.TLS.Enabled = v.GetBool("tls.enabled")
	cfg.TLS.CertFile = v.GetString("tls.cert_file")
	cfg.TLS.KeyFile = v.GetString("tls.key_file")
	cfg.Files.StorageMode = strings.ToLower(strings.TrimSpace(v.GetString("files.storage_mode")))
	cfg.Files.ActiveStorageBackend = strings.TrimSpace(v.GetString("files.active_storage_backend"))
	cfg.Edition.Name = strings.ToLower(strings.TrimSpace(v.GetString("edition.name")))
	cfg.Edition.LicenseFile = v.GetString("edition.license_file")
	cfg.SecretsDir = v.GetString("secrets.dir")
	cfg.Desktop.AppDir = v.GetString("desktop.app_dir")
	cfg.Desktop.ActiveBackend = strings.TrimSpace(v.GetString("desktop.active_backend"))
	cfg.Desktop.AllowUserBackends = v.GetBool("desktop.allow_user_backends")

	cfg.Files.StorageBackends = DefaultFileStorageBackends()
	localBackend := cfg.Files.StorageBackends[defaultFilesActiveBackend]
	localBackend.RootDir = v.GetString("files.root_dir")
	cfg.Files.StorageBackends[defaultFilesActiveBackend] = localBackend
	return cfg
}

// Normalize expands "~" prefixes in filesystem paths so later validation and
// startup see absolute locations.
func Normalize(cfg *Config) error {
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

// splitCommaList parses a comma-separated string, trimming whitespace and
// dropping empty entries.
func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	for _, part := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// splitStringList flattens a slice whose entries may themselves be
// comma-separated, which is how a single environment variable carries a list.
func splitStringList(values []string) []string {
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
