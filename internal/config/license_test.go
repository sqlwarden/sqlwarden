package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLicenseLoadsFromEverySupportedSourceAndIsRedacted(t *testing.T) {
	tests := map[string]struct {
		prepare func(*testing.T) []string
		want    string
		source  Source
	}{
		"config": {prepare: func(t *testing.T) []string {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("license: from-config\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{"--config", path}
		}, want: "from-config", source: SourceConfigFile},
		"environment": {prepare: func(t *testing.T) []string {
			t.Setenv("LICENSE", "from-env")
			return nil
		}, want: "from-env", source: SourceEnv},
		"file": {prepare: func(t *testing.T) []string {
			path := filepath.Join(t.TempDir(), "license")
			if err := os.WriteFile(path, []byte("from-file\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LICENSE_FILE", path)
			return nil
		}, want: "from-file", source: SourceSecretFile},
		"secrets directory": {prepare: func(t *testing.T) []string {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "license"), []byte("from-directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{"--secrets-dir", dir}
		}, want: "from-directory", source: SourceSecretFile},
		"flag": {prepare: func(*testing.T) []string {
			return []string{"--license", "from-flag"}
		}, want: "from-flag", source: SourceFlag},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			loaded, err := Load(test.prepare(t))
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Config.License != test.want {
				t.Fatalf("license = %q, want %q", loaded.Config.License, test.want)
			}
			if got := diagnosticSource(t, loaded.Diagnostic, "license"); got != test.source {
				t.Fatalf("license source = %q, want %q", got, test.source)
			}
			if strings.Contains(loaded.Diagnostic.String(), test.want) {
				t.Fatal("diagnostic leaked license material")
			}
		})
	}
}
