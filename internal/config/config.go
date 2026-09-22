package config

const (
	defaultBaseURL              = "http://localhost:6020"
	defaultHTTPPort             = 6020
	defaultDeploymentMode       = DeploymentModeServer
	defaultAccessMode           = AccessModeMultiUser
	defaultLogFormat            = LogFormatJSON
	defaultCookieSecretKey      = "cpcgzjcote6h5hakeglpbzixhbuog2zc"
	defaultDBDriver             = "sqlite"
	defaultDBDSN                = "~/.sqlwarden/sqlwarden.db"
	defaultDBAutomigrate        = true
	defaultEncryptionKey        = "dev-insecure-key-32byteslong!!"
	defaultJWTSecretKey         = "fb57i5hiud5mzmykaquqsln5gcmolbac"
	defaultTLSEnabled           = false
	defaultTLSCertFile          = ""
	defaultTLSKeyFile           = ""
	defaultFilesStorageMode     = FilesStorageModeObject
	defaultFilesActiveBackend   = "local"
	defaultFilesRootDir         = "~/.sqlwarden/files"
	defaultDesktopAppDir        = ""
	defaultDesktopActiveBackend = "local"
	defaultAllowUserBackends    = true
	defaultConnectorReplicas    = 1
	defaultSessionDirectory     = SessionDirectoryStatic
	defaultEdition              = EditionCommunity
	defaultSecretsDir           = ""
)

// DefaultFilesActiveBackend is the backend ID used when file storage runs in
// single-root file mode or no active backend is configured.
const DefaultFilesActiveBackend = defaultFilesActiveBackend

// Deployment modes describe how the binary is packaged and run.
const (
	DeploymentModeServer  = "server"
	DeploymentModeDesktop = "desktop"
)

// Access modes describe account and authorization behavior for the instance.
const (
	AccessModeMultiUser  = "multi_user"
	AccessModeSingleUser = "single_user"
)

// Desktop backend kinds distinguish an embedded local runtime from a remote
// SQLWarden server the desktop application points at.
const (
	DesktopBackendKindLocal  = "local"
	DesktopBackendKindRemote = "remote"
)

// Log levels accepted by runtime instance settings.
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Log formats accepted by bootstrap configuration.
const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

// File storage modes and backend types.
const (
	FilesStorageModeFile          = "file"
	FilesStorageModeObject        = "object"
	FilesStorageBackendFilesystem = "filesystem"
	FilesStorageBackendS3         = "s3"
)

// Process kinds name the runtime responsibilities a process may take on. Only
// [ProcessKindAll] is produced in production today; the remaining names are
// reserved so configuration and validation can be written against the final
// topology before the processes are split apart.
const (
	ProcessKindAll         = "all"
	ProcessKindAPI         = "api"
	ProcessKindConnector   = "connector"
	ProcessKindEdgeGateway = "edge-gateway"
	ProcessKindJobs        = "jobs"
	ProcessKindRealtime    = "realtime"
)

// Session directory backends resolve which process owns a live target-database
// session. The static directory only knows about the local process, so it
// cannot route between replicas.
const (
	SessionDirectoryStatic = "static"
	SessionDirectoryRedis  = "redis"
)

// Editions select which feature modules are compiled into and licensed for the
// running instance.
const (
	EditionCommunity  = "community"
	EditionEnterprise = "enterprise"
)

// Config holds every deployment-managed input needed to build an application.
// Fields are grouped by the categories documented on the package.
type Config struct {
	// BootstrapBaseURL seeds the database-backed instance base URL on first
	// start. After bootstrap the database value is authoritative.
	BootstrapBaseURL string
	HTTPPort         int
	DeploymentMode   string
	AccessMode       string
	// ProcessKinds selects the runtime responsibilities this process takes on.
	ProcessKinds []string
	// SessionDirectory selects how live target-database sessions are located.
	SessionDirectory string
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
	Files struct {
		StorageMode          string
		ActiveStorageBackend string
		StorageBackends      map[string]FileStorageBackend
	}
	Desktop struct {
		AppDir            string
		ActiveBackend     string
		AllowUserBackends bool
		Backends          []DesktopBackend
	}
	Connector struct {
		// Replicas is the number of processes serving the connector process
		// kind. Values above 1 require a shared session directory.
		Replicas int
	}
	Edition struct {
		Name        string
		LicenseFile string
	}
	// SecretsDir is a directory of mounted secret files, one file per
	// configuration key with dots and dashes replaced by underscores.
	SecretsDir string
}

// FileStorageBackend describes one configured workspace-file storage backend.
type FileStorageBackend struct {
	Type    string `mapstructure:"type"`
	RootDir string `mapstructure:"root_dir"`
}

// DesktopBackend describes one SQLWarden backend the desktop application can
// connect to, either the embedded local runtime or a remote server.
type DesktopBackend struct {
	ID          string `mapstructure:"id"`
	Name        string `mapstructure:"name"`
	Kind        string `mapstructure:"kind"`
	URL         string `mapstructure:"url"`
	Environment string `mapstructure:"environment"`
	AccessMode  string `mapstructure:"access_mode"`
	Locked      bool   `mapstructure:"locked"`
}

// Default returns configuration with every field set to its documented default.
func Default() Config {
	cfg := Config{}
	cfg.BootstrapBaseURL = defaultBaseURL
	cfg.HTTPPort = defaultHTTPPort
	cfg.DeploymentMode = defaultDeploymentMode
	cfg.AccessMode = defaultAccessMode
	cfg.ProcessKinds = []string{ProcessKindAll}
	cfg.SessionDirectory = defaultSessionDirectory
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
	cfg.Files.StorageMode = defaultFilesStorageMode
	cfg.Files.ActiveStorageBackend = defaultFilesActiveBackend
	cfg.Files.StorageBackends = DefaultFileStorageBackends()
	cfg.Desktop.AppDir = defaultDesktopAppDir
	cfg.Desktop.ActiveBackend = defaultDesktopActiveBackend
	cfg.Desktop.AllowUserBackends = defaultAllowUserBackends
	cfg.Desktop.Backends = defaultDesktopBackends()
	cfg.Connector.Replicas = defaultConnectorReplicas
	cfg.Edition.Name = defaultEdition
	cfg.SecretsDir = defaultSecretsDir
	return cfg
}

// DefaultFileStorageBackends returns a fresh single-backend map rooted at the
// default files directory. Callers own the returned map.
func DefaultFileStorageBackends() map[string]FileStorageBackend {
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

// HasProcessKind reports whether the process runs the named process kind,
// either directly or through [ProcessKindAll].
func (c Config) HasProcessKind(kind string) bool {
	for _, configured := range c.ProcessKinds {
		if configured == kind || configured == ProcessKindAll {
			return true
		}
	}
	return false
}

// IsSupportedLogLevel reports whether level is an accepted runtime log level.
func IsSupportedLogLevel(level string) bool {
	switch level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return true
	default:
		return false
	}
}

// IsSupportedLogFormat reports whether format is an accepted bootstrap log format.
func IsSupportedLogFormat(format string) bool {
	switch format {
	case LogFormatJSON, LogFormatText:
		return true
	default:
		return false
	}
}
