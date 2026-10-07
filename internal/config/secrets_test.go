package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSecretFileFromEnvSuffix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")
	if err := os.WriteFile(path, []byte("secret-from-mounted-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWT_SECRET_KEY_FILE", path)

	loaded, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Config.JWT.SecretKey != "secret-from-mounted-file" {
		t.Fatalf("jwt.secret_key = %q, want the trimmed file contents", loaded.Config.JWT.SecretKey)
	}
	if got := diagnosticSource(t, loaded.Diagnostic, "jwt.secret_key"); got != SourceSecretFile {
		t.Fatalf("jwt.secret_key source = %q, want %q", got, SourceSecretFile)
	}
}

func TestLoadSecretFileFromSecretsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "encryption_key"), []byte("secret-from-secrets-dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load([]string{"--secrets-dir", dir})
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Config.Encryption.Key != "secret-from-secrets-dir" {
		t.Fatalf("encryption.key = %q, want the secrets directory contents", loaded.Config.Encryption.Key)
	}
	if got := diagnosticSource(t, loaded.Diagnostic, "encryption.key"); got != SourceSecretFile {
		t.Fatalf("encryption.key source = %q, want %q", got, SourceSecretFile)
	}
	if got := diagnosticSource(t, loaded.Diagnostic, "cookie.secret_key"); got != SourceDefault {
		t.Fatalf("cookie.secret_key source = %q, want %q", got, SourceDefault)
	}
}

func TestLoadSecretFileOutranksConfigFileButNotEnvOrFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jwt_secret_key"), []byte("from-secret-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "encryption_key"), []byte("from-secret-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cookie_secret_key"), []byte("from-secret-file"), 0o600); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	content := []byte(`
jwt:
  secret_key: from-config-file
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ENCRYPTION_KEY", "from-env")

	loaded, err := Load([]string{
		"--config", configPath,
		"--secrets-dir", dir,
		"--cookie-secret-key", "from-flag",
	})
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Config.JWT.SecretKey != "from-secret-file" {
		t.Fatalf("jwt.secret_key = %q, want the secret file to outrank the config file", loaded.Config.JWT.SecretKey)
	}
	if loaded.Config.Encryption.Key != "from-env" {
		t.Fatalf("encryption.key = %q, want the environment to outrank the secret file", loaded.Config.Encryption.Key)
	}
	if loaded.Config.Cookie.SecretKey != "from-flag" {
		t.Fatalf("cookie.secret_key = %q, want the flag to outrank the secret file", loaded.Config.Cookie.SecretKey)
	}

	for key, want := range map[string]Source{
		"jwt.secret_key":    SourceSecretFile,
		"encryption.key":    SourceEnv,
		"cookie.secret_key": SourceFlag,
	} {
		if got := diagnosticSource(t, loaded.Diagnostic, key); got != want {
			t.Errorf("%s source = %q, want %q", key, got, want)
		}
	}
}

func TestLoadMissingSecretsDirEntryFallsBack(t *testing.T) {
	loaded, err := Load([]string{"--secrets-dir", t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Config.JWT.SecretKey != defaultJWTSecretKey {
		t.Fatalf("jwt.secret_key = %q, want the default when no secret file exists", loaded.Config.JWT.SecretKey)
	}
	if got := diagnosticSource(t, loaded.Diagnostic, "jwt.secret_key"); got != SourceDefault {
		t.Fatalf("jwt.secret_key source = %q, want %q", got, SourceDefault)
	}
}

func TestLoadUnreadableEnvSecretFileFails(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY_FILE", filepath.Join(t.TempDir(), "missing"))

	if _, err := Load(nil); err == nil {
		t.Fatal("expected a missing secret file named by env to fail startup")
	}
}

func diagnosticSource(t *testing.T, diagnostic Diagnostic, key string) Source {
	t.Helper()
	for _, entry := range diagnostic.Entries {
		if entry.Key == key {
			return entry.Source
		}
	}
	t.Fatalf("diagnostic has no entry for %q", key)
	return ""
}
