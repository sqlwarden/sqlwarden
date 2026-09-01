package desktop

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sqlwarden/internal/web"
	"github.com/zalando/go-keyring"
)

const (
	configVersion         = 2
	keyringService        = "SQLWarden Desktop"
	secretStoreKeyring    = "keyring"
	secretStoreFile       = "protected-file"
	legacyConfigVersion   = 1
	desktopDirectoryMode  = 0o700
	desktopSecretFileMode = 0o600
)

var ErrSecretsMissing = errors.New("desktop secrets are unavailable for an existing database")

type Paths struct {
	ConfigDir  string `json:"config_dir"`
	DataDir    string `json:"data_dir"`
	Database   string `json:"database"`
	Files      string `json:"files"`
	Cache      string `json:"cache"`
	Logs       string `json:"logs"`
	Temp       string `json:"temp"`
	Backups    string `json:"backups"`
	ConfigFile string `json:"config_file"`
	Secrets    string `json:"secrets_file,omitempty"`
	legacyRoot string
}

type configFile struct {
	Version     int    `json:"version"`
	SecretStore string `json:"secret_store"`
}

type secretsFile struct {
	Version       int    `json:"version"`
	CookieSecret  string `json:"cookie_secret"`
	EncryptionKey string `json:"encryption_key"`
	JWTSecret     string `json:"jwt_secret"`
}

func SecretStore(paths Paths) string {
	metadata, _, found, err := readConfigFile(paths.ConfigFile)
	if err != nil || !found {
		return "unavailable"
	}
	return metadata.SecretStore
}

// ResolvePaths returns explicit platform-native locations for durable data,
// configuration, caches, logs, temporary files, and backups. An override keeps
// everything below one root for portable installations and tests while still
// preserving the same directory boundaries.
func ResolvePaths(rootOverride string) (Paths, error) {
	var configDir, dataDir, cacheDir, tempDir, legacyRoot string
	if rootOverride != "" {
		root, err := filepath.Abs(rootOverride)
		if err != nil {
			return Paths{}, fmt.Errorf("resolve desktop root: %w", err)
		}
		configDir = filepath.Join(root, "config")
		dataDir = filepath.Join(root, "data")
		cacheDir = filepath.Join(root, "cache")
		tempDir = filepath.Join(root, "temp")
		legacyRoot = root
	} else {
		var err error
		configDir, dataDir, cacheDir, tempDir, legacyRoot, err = defaultRoots()
		if err != nil {
			return Paths{}, err
		}
	}

	return Paths{
		ConfigDir:  configDir,
		DataDir:    dataDir,
		Database:   filepath.Join(dataDir, "sqlwarden.db"),
		Files:      filepath.Join(dataDir, "files"),
		Cache:      cacheDir,
		Logs:       filepath.Join(cacheDir, "logs"),
		Temp:       tempDir,
		Backups:    filepath.Join(dataDir, "backups"),
		ConfigFile: filepath.Join(configDir, "desktop.json"),
		Secrets:    filepath.Join(configDir, "secrets.json"),
		legacyRoot: legacyRoot,
	}, nil
}

func defaultRoots() (configDir, dataDir, cacheDir, tempDir, legacyRoot string, err error) {
	userConfig, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("resolve user config directory: %w", err)
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	legacyRoot = filepath.Join(userConfig, "SQLWarden")
	configDir = legacyRoot
	dataDir = configDir
	if runtime.GOOS == "linux" {
		dataBase := os.Getenv("XDG_DATA_HOME")
		if dataBase == "" {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", "", "", "", "", fmt.Errorf("resolve user data directory: %w", homeErr)
			}
			dataBase = filepath.Join(home, ".local", "share")
		}
		dataDir = filepath.Join(dataBase, "SQLWarden")
	} else if runtime.GOOS == "darwin" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", "", "", "", "", fmt.Errorf("resolve user data directory: %w", homeErr)
		}
		configDir = filepath.Join(home, "Library", "Preferences", "SQLWarden")
		dataDir = filepath.Join(home, "Library", "Application Support", "SQLWarden")
	} else if runtime.GOOS == "windows" {
		dataDir = filepath.Join(userCache, "SQLWarden", "data")
	}
	cacheDir = filepath.Join(userCache, "SQLWarden")
	if runtime.GOOS == "windows" {
		cacheDir = filepath.Join(userCache, "SQLWarden", "cache")
	}
	tempDir = filepath.Join(os.TempDir(), "SQLWarden")
	return configDir, dataDir, cacheDir, tempDir, legacyRoot, nil
}

// LoadWebConfig migrates the v1 flat layout, loads installation secrets from
// the OS credential store (with a protected-file fallback), and constructs the
// in-process server configuration used by Wails.
func LoadWebConfig(rootOverride string) (web.Config, Paths, error) {
	paths, err := ResolvePaths(rootOverride)
	if err != nil {
		return web.Config{}, Paths{}, err
	}
	if err := createDirectories([]string{paths.ConfigDir, paths.DataDir, paths.Cache, paths.Temp, paths.Backups}); err != nil {
		return web.Config{}, paths, err
	}
	if err := migrateLegacyLayout(paths); err != nil {
		return web.Config{}, paths, err
	}
	if err := ApplyPendingRestore(paths); err != nil {
		return web.Config{}, paths, fmt.Errorf("apply pending desktop restore: %w", err)
	}
	if err := createDesktopDirectories(paths); err != nil {
		return web.Config{}, paths, err
	}
	if err := PruneCache(paths, DefaultCacheLimitBytes); err != nil {
		return web.Config{}, paths, fmt.Errorf("prune desktop cache: %w", err)
	}
	secrets, err := loadOrCreateSecrets(paths)
	if err != nil {
		return web.Config{}, paths, err
	}

	cfg := web.DefaultConfig()
	cfg.BootstrapBaseURL = "http://localhost"
	cfg.Mode = web.ModeDesktop
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = paths.Database
	cfg.DB.Automigrate = true
	cfg.Cookie.SecretKey = secrets.CookieSecret
	cfg.Encryption.Key = secrets.EncryptionKey
	cfg.Encryption.PreviousKeys = nil
	cfg.JWT.SecretKey = secrets.JWTSecret
	cfg.TLS.Enabled = false
	cfg.Drivers.SQLite.AllowedSources = []string{web.SQLiteDriverSourceLocal}
	cfg.Files.StorageMode = web.FilesStorageModeObject
	cfg.Files.ActiveStorageBackend = "local"
	cfg.Files.StorageBackends = map[string]web.FileStorageBackend{
		"local": {Type: web.FilesStorageBackendFilesystem, RootDir: paths.Files},
	}
	return cfg, paths, nil
}

func createDesktopDirectories(paths Paths) error {
	return createDirectories([]string{paths.ConfigDir, paths.DataDir, paths.Files, paths.Cache, paths.Logs, paths.Temp, paths.Backups})
}

func createDirectories(directories []string) error {
	for _, dir := range directories {
		if err := os.MkdirAll(dir, desktopDirectoryMode); err != nil {
			return fmt.Errorf("create desktop directory %q: %w", dir, err)
		}
		if err := os.Chmod(dir, desktopDirectoryMode); err != nil {
			return fmt.Errorf("protect desktop directory %q: %w", dir, err)
		}
	}
	return nil
}

func migrateLegacyLayout(paths Paths) error {
	if paths.legacyRoot == "" {
		return nil
	}
	for _, item := range []struct{ old, next string }{
		{filepath.Join(paths.legacyRoot, "desktop.json"), paths.ConfigFile},
		{filepath.Join(paths.legacyRoot, "sqlwarden.db"), paths.Database},
		{filepath.Join(paths.legacyRoot, "files"), paths.Files},
		{filepath.Join(paths.legacyRoot, "logs"), paths.Logs},
	} {
		if filepath.Clean(item.old) == filepath.Clean(item.next) {
			continue
		}
		if _, err := os.Stat(item.next); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("inspect desktop migration target %q: %w", item.next, err)
		}
		if _, err := os.Stat(item.old); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect legacy desktop path %q: %w", item.old, err)
		}
		if err := os.Rename(item.old, item.next); err != nil {
			return fmt.Errorf("migrate legacy desktop path %q: %w", item.old, err)
		}
	}
	return nil
}

func loadOrCreateSecrets(paths Paths) (secretsFile, error) {
	metadata, legacySecrets, found, err := readConfigFile(paths.ConfigFile)
	if err != nil {
		return secretsFile{}, err
	}
	if legacySecrets != nil {
		legacySecrets.Version = configVersion
		return persistSecrets(paths, *legacySecrets)
	}
	if found {
		switch metadata.SecretStore {
		case secretStoreKeyring:
			encoded, getErr := keyring.Get(keyringService, keyringAccount(paths))
			if getErr != nil {
				return secretsFile{}, fmt.Errorf("read desktop secrets from OS credential store: %w", ErrSecretsMissing)
			}
			return decodeSecrets([]byte(encoded))
		case secretStoreFile:
			contents, readErr := os.ReadFile(paths.Secrets)
			if readErr != nil {
				return secretsFile{}, fmt.Errorf("read protected desktop secrets: %w", ErrSecretsMissing)
			}
			if chmodErr := os.Chmod(paths.Secrets, desktopSecretFileMode); chmodErr != nil {
				return secretsFile{}, fmt.Errorf("protect desktop secrets: %w", chmodErr)
			}
			return decodeSecrets(contents)
		default:
			return secretsFile{}, errors.New("desktop configuration has an unsupported secret store")
		}
	}
	if _, statErr := os.Stat(paths.Database); statErr == nil {
		return secretsFile{}, ErrSecretsMissing
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return secretsFile{}, fmt.Errorf("inspect desktop database: %w", statErr)
	}

	secrets, err := generateSecrets()
	if err != nil {
		return secretsFile{}, err
	}
	return persistSecrets(paths, secrets)
}

func readConfigFile(path string) (configFile, *secretsFile, bool, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return configFile{}, nil, false, nil
	}
	if err != nil {
		return configFile{}, nil, false, fmt.Errorf("read desktop configuration: %w", err)
	}
	var version struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(contents, &version); err != nil {
		return configFile{}, nil, false, fmt.Errorf("decode desktop configuration: %w", err)
	}
	if version.Version == legacyConfigVersion {
		var legacy secretsFile
		if err := json.Unmarshal(contents, &legacy); err != nil {
			return configFile{}, nil, false, fmt.Errorf("decode legacy desktop configuration: %w", err)
		}
		if err := validateSecrets(legacy); err != nil {
			return configFile{}, nil, false, err
		}
		return configFile{}, &legacy, true, nil
	}
	var metadata configFile
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return configFile{}, nil, false, fmt.Errorf("decode desktop configuration: %w", err)
	}
	if metadata.Version != configVersion {
		return configFile{}, nil, false, errors.New("desktop configuration version is unsupported")
	}
	return metadata, nil, true, nil
}

func persistSecrets(paths Paths, secrets secretsFile) (secretsFile, error) {
	encoded, err := json.Marshal(secrets)
	if err != nil {
		return secretsFile{}, fmt.Errorf("encode desktop secrets: %w", err)
	}
	store := secretStoreKeyring
	if err := keyring.Set(keyringService, keyringAccount(paths), string(encoded)); err != nil {
		store = secretStoreFile
		if err := replaceProtectedFile(paths.Secrets, append(encoded, '\n')); err != nil {
			return secretsFile{}, fmt.Errorf("store desktop secrets after OS credential store failure: %w", err)
		}
	}
	metadata, err := json.MarshalIndent(configFile{Version: configVersion, SecretStore: store}, "", "  ")
	if err != nil {
		return secretsFile{}, fmt.Errorf("encode desktop configuration: %w", err)
	}
	if err := replaceProtectedFile(paths.ConfigFile, append(metadata, '\n')); err != nil {
		return secretsFile{}, err
	}
	return secrets, nil
}

func keyringAccount(paths Paths) string {
	sum := sha256.Sum256([]byte(filepath.Clean(paths.ConfigDir)))
	return fmt.Sprintf("installation-%x", sum[:8])
}

func decodeSecrets(contents []byte) (secretsFile, error) {
	var secrets secretsFile
	if err := json.Unmarshal(contents, &secrets); err != nil {
		return secretsFile{}, fmt.Errorf("decode desktop secrets: %w", err)
	}
	if err := validateSecrets(secrets); err != nil {
		return secretsFile{}, err
	}
	return secrets, nil
}

func validateSecrets(secrets secretsFile) error {
	if secrets.Version != configVersion && secrets.Version != legacyConfigVersion {
		return errors.New("desktop secrets version is unsupported")
	}
	if strings.TrimSpace(secrets.CookieSecret) == "" || strings.TrimSpace(secrets.EncryptionKey) == "" || strings.TrimSpace(secrets.JWTSecret) == "" {
		return errors.New("desktop secrets are incomplete")
	}
	return nil
}

func generateSecrets() (secretsFile, error) {
	secrets := secretsFile{Version: configVersion}
	var err error
	if secrets.CookieSecret, err = randomSecret(); err != nil {
		return secretsFile{}, err
	}
	if secrets.EncryptionKey, err = randomSecret(); err != nil {
		return secretsFile{}, err
	}
	if secrets.JWTSecret, err = randomSecret(); err != nil {
		return secretsFile{}, err
	}
	return secrets, nil
}

func randomSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate desktop secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func replaceProtectedFile(path string, contents []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sqlwarden-*")
	if err != nil {
		return fmt.Errorf("create protected temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(desktopSecretFileMode); err != nil {
		return fmt.Errorf("protect temporary file: %w", err)
	}
	if _, err := io.Copy(temporary, strings.NewReader(string(contents))); err != nil {
		return fmt.Errorf("write protected file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync protected file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close protected file: %w", err)
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install protected file: %w", err)
	}
	removeTemporary = false
	return nil
}
