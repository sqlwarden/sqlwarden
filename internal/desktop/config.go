package desktop

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sqlwarden/internal/web"
)

const configVersion = 1

var ErrSecretsMissing = errors.New("desktop configuration is missing for an existing database")

type Paths struct {
	DataDir    string `json:"data_dir"`
	Database   string `json:"database"`
	Files      string `json:"files"`
	Logs       string `json:"logs"`
	ConfigFile string `json:"config_file"`
}

type secretsFile struct {
	Version       int    `json:"version"`
	CookieSecret  string `json:"cookie_secret"`
	EncryptionKey string `json:"encryption_key"`
	JWTSecret     string `json:"jwt_secret"`
}

// ResolvePaths returns platform-native desktop storage locations. dataDir may
// override the root for development, portable installations, and tests.
func ResolvePaths(dataDir string) (Paths, error) {
	if dataDir == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve user config directory: %w", err)
		}
		dataDir = filepath.Join(root, "SQLWarden")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve desktop data directory: %w", err)
	}
	return Paths{
		DataDir:    abs,
		Database:   filepath.Join(abs, "sqlwarden.db"),
		Files:      filepath.Join(abs, "files"),
		Logs:       filepath.Join(abs, "logs"),
		ConfigFile: filepath.Join(abs, "desktop.json"),
	}, nil
}

// LoadWebConfig creates protected installation secrets on first launch and
// produces the in-process web application configuration used by Wails.
func LoadWebConfig(dataDir string) (web.Config, Paths, error) {
	paths, err := ResolvePaths(dataDir)
	if err != nil {
		return web.Config{}, Paths{}, err
	}
	if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
		return web.Config{}, Paths{}, fmt.Errorf("create desktop data directory: %w", err)
	}
	if err := os.Chmod(paths.DataDir, 0o700); err != nil {
		return web.Config{}, Paths{}, fmt.Errorf("protect desktop data directory: %w", err)
	}

	secrets, err := loadOrCreateSecrets(paths)
	if err != nil {
		return web.Config{}, paths, err
	}
	for _, dir := range []string{paths.Files, paths.Logs} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return web.Config{}, paths, fmt.Errorf("create desktop directory %q: %w", dir, err)
		}
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

func loadOrCreateSecrets(paths Paths) (secretsFile, error) {
	contents, err := os.ReadFile(paths.ConfigFile)
	if err == nil {
		var secrets secretsFile
		if err := json.Unmarshal(contents, &secrets); err != nil {
			return secretsFile{}, fmt.Errorf("decode desktop configuration: %w", err)
		}
		if secrets.Version != configVersion || secrets.CookieSecret == "" || secrets.EncryptionKey == "" || secrets.JWTSecret == "" {
			return secretsFile{}, errors.New("desktop configuration is incomplete or unsupported")
		}
		if err := os.Chmod(paths.ConfigFile, 0o600); err != nil {
			return secretsFile{}, fmt.Errorf("protect desktop configuration: %w", err)
		}
		return secrets, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return secretsFile{}, fmt.Errorf("read desktop configuration: %w", err)
	}
	if _, err := os.Stat(paths.Database); err == nil {
		return secretsFile{}, ErrSecretsMissing
	} else if !errors.Is(err, fs.ErrNotExist) {
		return secretsFile{}, fmt.Errorf("inspect desktop database: %w", err)
	}

	secrets := secretsFile{Version: configVersion}
	if secrets.CookieSecret, err = randomSecret(); err != nil {
		return secretsFile{}, err
	}
	if secrets.EncryptionKey, err = randomSecret(); err != nil {
		return secretsFile{}, err
	}
	if secrets.JWTSecret, err = randomSecret(); err != nil {
		return secretsFile{}, err
	}
	encoded, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return secretsFile{}, fmt.Errorf("encode desktop configuration: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := writeProtectedFile(paths.ConfigFile, encoded); err != nil {
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

func writeProtectedFile(path string, contents []byte) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create desktop configuration: %w", err)
	}
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("write desktop configuration: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync desktop configuration: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close desktop configuration: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("install desktop configuration: %w", err)
	}
	removeTemporary = false
	return nil
}
