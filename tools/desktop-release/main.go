package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const releaseManifestSchemaVersion = 1

var releaseTagPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type releaseVersion struct {
	Tag       string
	Version   string
	OSVersion string
}

type releaseArtifact struct {
	Filename     string `json:"filename"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Format       string `json:"format"`
	SHA256       string `json:"sha256"`
}

type releaseManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Version       string            `json:"version"`
	Tag           string            `json:"tag"`
	PublishedAt   time.Time         `json:"published_at"`
	Artifacts     []releaseArtifact `json:"artifacts"`
}

type expectedArtifact struct {
	filename     string
	os           string
	architecture string
	format       string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: desktop-release <prepare|manifest> [flags]")
	}
	switch args[0] {
	case "prepare":
		return runPrepare(args[1:], stdout)
	case "manifest":
		return runManifest(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runPrepare(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tag := flags.String("tag", "", "release tag")
	configPath := flags.String("wails-config", "", "wails.json path to update")
	githubOutput := flags.String("github-output", "", "optional GitHub Actions output file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	version, err := parseReleaseTag(*tag)
	if err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--wails-config is required")
	}
	if err := setWailsProductVersion(*configPath, version.OSVersion); err != nil {
		return err
	}

	values := []string{
		"tag=" + version.Tag,
		"version=" + version.Version,
		"os_version=" + version.OSVersion,
	}
	for _, value := range values {
		fmt.Fprintln(stdout, value)
	}
	if *githubOutput != "" {
		file, err := os.OpenFile(*githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open GitHub output: %w", err)
		}
		defer file.Close()
		for _, value := range values {
			if _, err := fmt.Fprintln(file, value); err != nil {
				return fmt.Errorf("write GitHub output: %w", err)
			}
		}
	}
	return nil
}

func runManifest(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("manifest", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tag := flags.String("tag", "", "release tag")
	directory := flags.String("artifacts-dir", "", "directory containing release artifacts")
	publishedAt := flags.String("published-at", "", "release timestamp in RFC3339 format")
	manifestPath := flags.String("manifest", "", "manifest output path")
	checksumsPath := flags.String("checksums", "", "checksums output path")
	if err := flags.Parse(args); err != nil {
		return err
	}

	version, err := parseReleaseTag(*tag)
	if err != nil {
		return err
	}
	if *directory == "" || *manifestPath == "" || *checksumsPath == "" {
		return errors.New("--artifacts-dir, --manifest, and --checksums are required")
	}
	timestamp, err := time.Parse(time.RFC3339, *publishedAt)
	if err != nil {
		return fmt.Errorf("parse --published-at: %w", err)
	}

	artifacts, err := collectArtifacts(*directory, version.Version)
	if err != nil {
		return err
	}
	manifest := releaseManifest{
		SchemaVersion: releaseManifestSchemaVersion,
		Version:       version.Version,
		Tag:           version.Tag,
		PublishedAt:   timestamp.UTC(),
		Artifacts:     artifacts,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write release manifest: %w", err)
	}

	var checksums strings.Builder
	for _, artifact := range artifacts {
		fmt.Fprintf(&checksums, "%s  %s\n", artifact.SHA256, artifact.Filename)
	}
	if err := os.WriteFile(*checksumsPath, []byte(checksums.String()), 0o644); err != nil {
		return fmt.Errorf("write release checksums: %w", err)
	}
	fmt.Fprintf(stdout, "wrote metadata for %d desktop artifacts\n", len(artifacts))
	return nil
}

func parseReleaseTag(tag string) (releaseVersion, error) {
	matches := releaseTagPattern.FindStringSubmatch(tag)
	if matches == nil {
		return releaseVersion{}, fmt.Errorf("invalid release tag %q: expected vX.Y.Z or vX.Y.Z-prerelease", tag)
	}
	return releaseVersion{
		Tag:       tag,
		Version:   strings.TrimPrefix(tag, "v"),
		OSVersion: strings.Join(matches[1:4], "."),
	}, nil
}

func setWailsProductVersion(path, version string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Wails config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode Wails config: %w", err)
	}
	info, ok := config["info"].(map[string]any)
	if !ok {
		return errors.New("wails config is missing the info object")
	}
	info["productVersion"] = version
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Wails config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write Wails config: %w", err)
	}
	return nil
}

func expectedArtifacts(version string) []expectedArtifact {
	prefix := "sqlwarden-desktop_" + version + "_"
	return []expectedArtifact{
		{filename: prefix + "darwin_universal.dmg", os: "darwin", architecture: "universal", format: "dmg"},
		{filename: prefix + "linux_x86_64.deb", os: "linux", architecture: "x86_64", format: "deb"},
		{filename: prefix + "linux_x86_64.tar.gz", os: "linux", architecture: "x86_64", format: "tar.gz"},
		{filename: prefix + "windows_x86_64_setup.exe", os: "windows", architecture: "x86_64", format: "nsis"},
	}
}

func collectArtifacts(directory, version string) ([]releaseArtifact, error) {
	expected := expectedArtifacts(version)
	artifacts := make([]releaseArtifact, 0, len(expected))
	for _, item := range expected {
		path := filepath.Join(directory, item.filename)
		hash, err := fileSHA256(path)
		if err != nil {
			return nil, fmt.Errorf("hash required artifact %q: %w", item.filename, err)
		}
		artifacts = append(artifacts, releaseArtifact{
			Filename:     item.filename,
			OS:           item.os,
			Architecture: item.architecture,
			Format:       item.format,
			SHA256:       hash,
		})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Filename < artifacts[j].Filename })
	return artifacts, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
