package desktop

import (
	"errors"
	"os"
	"path/filepath"
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
	if first.DeploymentMode != web.DeploymentModeDesktop || first.AccessMode != web.AccessModeSingleUser {
		t.Fatalf("unexpected topology: %s/%s", first.DeploymentMode, first.AccessMode)
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
	if paths.DataDir != dataDir || paths.Database == "" || paths.Logs == "" {
		t.Fatalf("startup failure did not preserve diagnostic paths: %+v", paths)
	}
}
