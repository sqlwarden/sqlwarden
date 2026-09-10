package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/web"
)

func TestLoadWebConfigCreatesAndReusesProtectedSecrets(t *testing.T) {
	dataDir := t.TempDir()
	first, paths, err := LoadWebConfig(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := LoadWebConfig(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if first.JWT.SecretKey != second.JWT.SecretKey || first.Encryption.Key != second.Encryption.Key || first.Cookie.SecretKey != second.Cookie.SecretKey {
		t.Fatal("desktop secrets changed between launches")
	}
	if first.Mode != web.ModeDesktop {
		t.Fatalf("unexpected mode: %s", first.Mode)
	}
	if first.DB.DSN != paths.Database || first.Files.StorageBackends["local"].RootDir != paths.Files {
		t.Fatal("desktop paths were not applied to web configuration")
	}
	info, err := os.Stat(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("desktop configuration mode = %o, want 600", got)
	}
}

func TestLoadWebConfigRejectsExistingDatabaseWithoutSecrets(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "sqlwarden.db"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, paths, err := LoadWebConfig(dataDir)
	if !errors.Is(err, ErrSecretsMissing) {
		t.Fatalf("error = %v, want ErrSecretsMissing", err)
	}
	if paths.DataDir != filepath.Join(dataDir, "data") || paths.Database == "" || paths.Logs == "" {
		t.Fatalf("startup failure did not preserve diagnostic paths: %+v", paths)
	}
}

func TestLoadWebConfigSeparatesSecretsFromApplicationDatabase(t *testing.T) {
	root := t.TempDir()
	_, paths, err := LoadWebConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(paths.Database) == filepath.Dir(paths.ConfigFile) {
		t.Fatalf("database and configuration share a directory: %+v", paths)
	}
	contents, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, secretField := range []string{"cookie_secret", "encryption_key", "jwt_secret"} {
		if strings.Contains(string(contents), secretField) {
			t.Fatalf("configuration contains plaintext secret field %q", secretField)
		}
	}
}

func TestLoadWebConfigMigratesLegacyFlatLayout(t *testing.T) {
	root := t.TempDir()
	legacy := `{"version":1,"cookie_secret":"cookie","encryption_key":"encryption","jwt_secret":"jwt"}`
	if err := os.WriteFile(filepath.Join(root, "desktop.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sqlwarden.db"), []byte("database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, paths, err := LoadWebConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Database); err != nil {
		t.Fatalf("migrated database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sqlwarden.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy database still exists: %v", err)
	}
}
