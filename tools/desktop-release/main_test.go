package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseReleaseTag(t *testing.T) {
	testCases := []struct {
		tag       string
		version   string
		osVersion string
		valid     bool
	}{
		{tag: "v0.9.0", version: "0.9.0", osVersion: "0.9.0", valid: true},
		{tag: "v1.2.3-rc.1", version: "1.2.3-rc.1", osVersion: "1.2.3", valid: true},
		{tag: "1.2.3", valid: false},
		{tag: "v1.2", valid: false},
		{tag: "v01.2.3", valid: false},
		{tag: "v1.2.3+build", valid: false},
	}
	for _, tc := range testCases {
		t.Run(tc.tag, func(t *testing.T) {
			version, err := parseReleaseTag(tc.tag)
			if !tc.valid {
				if err == nil {
					t.Fatal("expected invalid tag error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if version.Version != tc.version || version.OSVersion != tc.osVersion {
				t.Fatalf("version = %+v", version)
			}
		})
	}
}

func TestPrepareUpdatesWailsVersionAndGitHubOutputs(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "wails.json")
	outputPath := filepath.Join(directory, "github-output")
	if err := os.WriteFile(configPath, []byte(`{"name":"SQLWarden","info":{"productVersion":"0.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{
		"prepare",
		"--tag", "v1.4.0-rc.2",
		"--wails-config", configPath,
		"--github-output", outputPath,
	}, &stdout); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Info.ProductVersion != "1.4.0" {
		t.Fatalf("product version = %q", config.Info.ProductVersion)
	}
	outputs, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "tag=v1.4.0-rc.2\nversion=1.4.0-rc.2\nos_version=1.4.0\n"
	if string(outputs) != want || stdout.String() != want {
		t.Fatalf("outputs = %q, stdout = %q", outputs, stdout.String())
	}
}

func TestManifestRequiresAndHashesEveryDesktopArtifact(t *testing.T) {
	directory := t.TempDir()
	for index, artifact := range expectedArtifacts("1.5.0") {
		if err := os.WriteFile(filepath.Join(directory, artifact.filename), []byte{byte(index + 1)}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(directory, "release.json")
	checksumsPath := filepath.Join(directory, "checksums.txt")
	if err := run([]string{
		"manifest",
		"--tag", "v1.5.0",
		"--published-at", "2026-08-21T12:00:00Z",
		"--artifacts-dir", directory,
		"--manifest", manifestPath,
		"--checksums", checksumsPath,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest releaseManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.Version != "1.5.0" || manifest.Tag != "v1.5.0" {
		t.Fatalf("manifest identity = %+v", manifest)
	}
	if !manifest.PublishedAt.Equal(time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("published at = %s", manifest.PublishedAt)
	}
	if len(manifest.Artifacts) != 4 {
		t.Fatalf("artifact count = %d", len(manifest.Artifacts))
	}
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range manifest.Artifacts {
		if len(artifact.SHA256) != 64 || !strings.Contains(string(checksums), artifact.SHA256+"  "+artifact.Filename) {
			t.Fatalf("missing checksum for %+v", artifact)
		}
	}
}

func TestManifestFailsWhenARequiredArtifactIsMissing(t *testing.T) {
	directory := t.TempDir()
	err := run([]string{
		"manifest",
		"--tag", "v1.5.0",
		"--published-at", "2026-08-21T12:00:00Z",
		"--artifacts-dir", directory,
		"--manifest", filepath.Join(directory, "release.json"),
		"--checksums", filepath.Join(directory, "checksums.txt"),
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "required artifact") {
		t.Fatalf("error = %v", err)
	}
}
