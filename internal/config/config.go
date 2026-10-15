package config

import "time"

const (
	defaultBaseURL              = "http://localhost:6020"
	defaultHTTPPort             = 6020
	defaultProfile              = ProfileServer
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
	defaultSecretsDir           = ""
	defaultShutdownTimeout      = 30 * time.Second
	defaultMigrationTimeout     = 5 * time.Minute
	defaultLicense              = ""
)

// DefaultFilesActiveBackend is the backend ID used when file storage runs in
// single-root file mode or no active backend is configured.
const DefaultFilesActiveBackend = defaultFilesActiveBackend

// Profiles select the product composition a process runs as.
const (
	ProfileServer  = "server"
	ProfileDesktop = "desktop"
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

// Process kinds name the runtime responsibilities a process takes on. Only all,
// api, and jobs are implemented. Validation rejects the others.
const (
	ProcessKindAll         = "all"
	ProcessKindAPI         = "api"
	ProcessKindConnector   = "connector"
	ProcessKindEdgeGateway = "edge-gateway"
	ProcessKindJobs        = "jobs"
	ProcessKindRealtime    = "realtime"
)

// MigrateCommand is the binary subcommand that applies database migrations
// under the migration lock. Topologies that reject db.automigrate run it once
// before their serving replicas start.
const MigrateCommand = "migrate"

// Config holds every deployment-managed input needed to build an application.
// Fields are grouped by the categories documented on the package.
type Config struct {
	// BootstrapBaseURL seeds the database-backed instance base URL on first
	// start. After bootstrap the database value is authoritative.
	BootstrapBaseURL string
	HTTPPort         int
	// Profile selects the product composition: setup, sign-in and
	// invitation behavior. It is fixed for the life of a process.
	Profile string
	// License is accepted only by an Enterprise build. It is intentionally
	// opaque until a real license verifier replaces the no-op licenser.
	License string
	Server  struct {
		// TrustedProxies lists CIDRs or IPs whose X-Forwarded-For header is
		// believed when resolving the client IP.
		TrustedProxies []string
	}
	// ProcessKinds selects the runtime responsibilities this process takes on.
	ProcessKinds []string
	Log          struct {
		Format string
	}
	Cookie struct {
		SecretKey string
	}
	DB struct {
		Driver string
		DSN    string
		// Automigrate runs migrations during Build. Validation rejects it when
		// api or jobs is selected.
		Automigrate bool
		// MigrationTimeout bounds only the wait for the migration lock. Once
		// the lock is held, the migration runs to completion regardless of
		// how long it takes.
		MigrationTimeout time.Duration
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
	// ShutdownTimeout bounds graceful shutdown after a termination signal.
	ShutdownTimeout time.Duration

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
	Locked      bool   `mapstructure:"locked"`
}

// Default returns configuration with every field set to its documented default.
func Default() Config {
	cfg := Config{}
	cfg.BootstrapBaseURL = defaultBaseURL
	cfg.HTTPPort = defaultHTTPPort
	cfg.Profile = defaultProfile
	cfg.License = defaultLicense
	cfg.ProcessKinds = []string{ProcessKindAll}
	cfg.Log.Format = defaultLogFormat
	cfg.Cookie.SecretKey = defaultCookieSecretKey
	cfg.DB.Driver = defaultDBDriver
	cfg.DB.DSN = defaultDBDSN
	cfg.DB.Automigrate = defaultDBAutomigrate
	cfg.DB.MigrationTimeout = defaultMigrationTimeout
	cfg.ShutdownTimeout = defaultShutdownTimeout
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
			ID:   "local",
			Name: "Local",
			Kind: DesktopBackendKindLocal,
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

// SelectsOnly reports whether kind is the single configured process kind.
func (c Config) SelectsOnly(kind string) bool {
	return len(c.ProcessKinds) == 1 && c.ProcessKinds[0] == kind
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
