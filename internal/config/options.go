package config

// Category groups configuration keys by who owns them and when they may change.
type Category string

// Configuration categories. Runtime settings live in the application database
// and are not loadable here; the category exists so vocabulary shared with
// runtime settings can be labelled correctly in diagnostics.
const (
	CategoryBootstrap Category = "bootstrap"
	CategoryRuntime   Category = "runtime"
	CategorySecrets   Category = "secrets"
	CategoryEdition   Category = "edition"
)

// option describes one loadable configuration key and how it is exposed to
// operators. Keys marked sensitive never have their value printed in a
// diagnostic, regardless of category.
type option struct {
	key          string
	env          string
	flagName     string
	defaultValue any
	usage        string
	category     Category
	sensitive    bool
}

// options is the canonical registry of loadable bootstrap configuration. Every
// key here is settable from a config file, an environment variable, a CLI flag,
// and (for sensitive keys) a mounted secret file.
var options = []option{
	{key: "base_url", env: "BASE_URL", flagName: "base-url", defaultValue: defaultBaseURL, category: CategoryBootstrap, usage: "Initial instance base URL used only when bootstrapping runtime settings"},
	{key: "http_port", env: "HTTP_PORT", flagName: "http-port", defaultValue: defaultHTTPPort, category: CategoryBootstrap, usage: "HTTP server port"},
	{key: "deployment_mode", env: "DEPLOYMENT_MODE", flagName: "deployment-mode", defaultValue: defaultDeploymentMode, category: CategoryBootstrap, usage: "Deployment mode (server or desktop)"},
	{key: "access_mode", env: "ACCESS_MODE", flagName: "access-mode", defaultValue: defaultAccessMode, category: CategoryBootstrap, usage: "Instance access mode (multi_user or single_user)"},
	{key: "process_kinds", env: "PROCESS_KINDS", flagName: "process-kinds", defaultValue: []string{ProcessKindAll}, category: CategoryBootstrap, usage: "Comma-separated runtime process kinds this process serves"},
	{key: "session_directory", env: "SESSION_DIRECTORY", flagName: "session-directory", defaultValue: defaultSessionDirectory, category: CategoryBootstrap, usage: "Live target-database session directory backend (static or redis)"},
	{key: "connector.replicas", env: "CONNECTOR_REPLICAS", flagName: "connector-replicas", defaultValue: defaultConnectorReplicas, category: CategoryBootstrap, usage: "Number of replicas serving the connector process kind"},
	{key: "connector.address", env: "CONNECTOR_ADDRESS", flagName: "connector-address", defaultValue: defaultConnectorAddress, category: CategoryBootstrap, usage: "Connector host and port used by API processes"},
	{key: "connector.listen_address", env: "CONNECTOR_LISTEN_ADDRESS", flagName: "connector-listen-address", defaultValue: defaultConnectorListen, category: CategoryBootstrap, usage: "Address on which the connector process serves internal execution requests"},
	{key: "connector.health_address", env: "CONNECTOR_HEALTH_ADDRESS", flagName: "connector-health-address", defaultValue: defaultConnectorHealth, category: CategoryBootstrap, usage: "Address on which the connector process serves liveness and readiness probes"},
	{key: "connector.transport", env: "CONNECTOR_TRANSPORT", flagName: "connector-transport", defaultValue: defaultConnectorTransport, category: CategoryBootstrap, usage: "Internal connector transport credentials (insecure or tls)"},
	{key: "connector.grant_signing_key", env: "CONNECTOR_GRANT_SIGNING_KEY", flagName: "connector-grant-signing-key", defaultValue: defaultConnectorGrantKey, category: CategorySecrets, sensitive: true, usage: "Execution grant signing secret shared by API and connector processes"},
	{key: "connector.tls.ca_file", env: "CONNECTOR_TLS_CA_FILE", flagName: "connector-tls-ca-file", defaultValue: "", category: CategoryBootstrap, usage: "PEM CA bundle for connector TLS clients"},
	{key: "connector.tls.cert_file", env: "CONNECTOR_TLS_CERT_FILE", flagName: "connector-tls-cert-file", defaultValue: "", category: CategoryBootstrap, usage: "PEM certificate for the connector TLS listener"},
	{key: "connector.tls.key_file", env: "CONNECTOR_TLS_KEY_FILE", flagName: "connector-tls-key-file", defaultValue: "", category: CategoryBootstrap, usage: "PEM private key for the connector TLS listener"},
	{key: "connector.tls.server_name", env: "CONNECTOR_TLS_SERVER_NAME", flagName: "connector-tls-server-name", defaultValue: "", category: CategoryBootstrap, usage: "TLS server name expected by connector clients"},
	{key: "log.format", env: "LOG_FORMAT", flagName: "log-format", defaultValue: defaultLogFormat, category: CategoryBootstrap, usage: "Log format (json or text)"},
	{key: "cookie.secret_key", env: "COOKIE_SECRET_KEY", flagName: "cookie-secret-key", defaultValue: defaultCookieSecretKey, category: CategorySecrets, sensitive: true, usage: "Cookie signing secret"},
	{key: "db.driver", env: "DB_DRIVER", flagName: "db-driver", defaultValue: defaultDBDriver, category: CategoryBootstrap, usage: "Database driver (sqlite or postgres)"},
	{key: "db.dsn", env: "DB_DSN", flagName: "db-dsn", defaultValue: defaultDBDSN, category: CategoryBootstrap, sensitive: true, usage: "Database DSN"},
	{key: "db.automigrate", env: "DB_AUTOMIGRATE", flagName: "db-automigrate", defaultValue: defaultDBAutomigrate, category: CategoryBootstrap, usage: "Run database migrations at startup"},
	{key: "db.migration_timeout", env: "DB_MIGRATION_TIMEOUT", flagName: "db-migration-timeout", defaultValue: defaultMigrationTimeout, category: CategoryBootstrap, usage: "Maximum time one migrate run may spend waiting for the migration lock and applying migrations"},
	{key: "shutdown_timeout", env: "SHUTDOWN_TIMEOUT", flagName: "shutdown-timeout", defaultValue: defaultShutdownTimeout, category: CategoryBootstrap, usage: "Maximum time graceful shutdown may take after a termination signal"},
	{key: "encryption.key", env: "ENCRYPTION_KEY", flagName: "encryption-key", defaultValue: defaultEncryptionKey, category: CategorySecrets, sensitive: true, usage: "Application encryption key"},
	{key: "encryption.previous_keys", env: "ENCRYPTION_PREVIOUS_KEYS", flagName: "encryption-previous-keys", defaultValue: "", category: CategorySecrets, sensitive: true, usage: "Comma-separated retired encryption keys retained for decryption during rotation"},
	{key: "jwt.secret_key", env: "JWT_SECRET_KEY", flagName: "jwt-secret-key", defaultValue: defaultJWTSecretKey, category: CategorySecrets, sensitive: true, usage: "JWT signing secret"},
	{key: "tls.enabled", env: "TLS_ENABLED", flagName: "tls-enabled", defaultValue: defaultTLSEnabled, category: CategoryBootstrap, usage: "Serve HTTPS using configured TLS certificate and key files"},
	{key: "tls.cert_file", env: "TLS_CERT_FILE", flagName: "tls-cert-file", defaultValue: defaultTLSCertFile, category: CategoryBootstrap, usage: "Path to PEM encoded TLS certificate file"},
	{key: "tls.key_file", env: "TLS_KEY_FILE", flagName: "tls-key-file", defaultValue: defaultTLSKeyFile, category: CategoryBootstrap, usage: "Path to PEM encoded TLS private key file"},
	{key: "files.storage_mode", env: "FILES_STORAGE_MODE", flagName: "files-storage-mode", defaultValue: defaultFilesStorageMode, category: CategoryBootstrap, usage: "Workspace file storage mode (file or object)"},
	{key: "files.active_storage_backend", env: "FILES_ACTIVE_STORAGE_BACKEND", flagName: "files-active-storage-backend", defaultValue: defaultFilesActiveBackend, category: CategoryBootstrap, usage: "Backend ID used for new workspace file content"},
	{key: "files.root_dir", env: "FILES_ROOT_DIR", flagName: "files-root-dir", defaultValue: defaultFilesRootDir, category: CategoryBootstrap, usage: "Filesystem root directory for stored workspace files"},
	{key: "desktop.app_dir", env: "DESKTOP_APP_DIR", flagName: "desktop-app-dir", defaultValue: defaultDesktopAppDir, category: CategoryBootstrap, usage: "Desktop application data directory"},
	{key: "desktop.active_backend", env: "DESKTOP_ACTIVE_BACKEND", flagName: "desktop-active-backend", defaultValue: defaultDesktopActiveBackend, category: CategoryBootstrap, usage: "Desktop backend selected at startup"},
	{key: "desktop.allow_user_backends", env: "DESKTOP_ALLOW_USER_BACKENDS", flagName: "desktop-allow-user-backends", defaultValue: defaultAllowUserBackends, category: CategoryBootstrap, usage: "Allow desktop users to add backend definitions"},
	{key: "edition.name", env: "EDITION", flagName: "edition", defaultValue: defaultEdition, category: CategoryEdition, usage: "Licensed edition (community or enterprise)"},
	{key: "edition.license_file", env: "EDITION_LICENSE_FILE", flagName: "edition-license-file", defaultValue: "", category: CategoryEdition, usage: "Path to the enterprise license file"},
	{key: "secrets.dir", env: "SECRETS_DIR", flagName: "secrets-dir", defaultValue: defaultSecretsDir, category: CategoryBootstrap, usage: "Directory of mounted secret files, one file per configuration key"},
}

// EnvVars returns the environment variable name of every loadable
// configuration key. Deployment artefacts that set configuration through the
// environment check their variable names against this list.
func EnvVars() []string {
	names := make([]string, 0, len(options))
	for _, opt := range options {
		names = append(names, opt.env)
	}
	return names
}
