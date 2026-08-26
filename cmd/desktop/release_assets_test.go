//go:build !bindings

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDesktopReleaseMetadata(t *testing.T) {
	data, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Info struct {
			CompanyName    string `json:"companyName"`
			ProductName    string `json:"productName"`
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Info.CompanyName != "SQLWarden" || config.Info.ProductName != "SQLWarden" {
		t.Fatalf("desktop product metadata = %+v", config.Info)
	}
	if config.Info.ProductVersion != "0.0.0" {
		t.Fatalf("development product version = %q", config.Info.ProductVersion)
	}

	plist := readReleaseAsset(t, "build/darwin/Info.plist")
	for _, value := range []string{"com.sqlwarden.desktop", "<string>11.0</string>", "{{.Info.ProductVersion}}"} {
		if !strings.Contains(plist, value) {
			t.Fatalf("macOS Info.plist is missing %q", value)
		}
	}
}

func TestWindowsInstallerSignsEveryExecutable(t *testing.T) {
	project := readReleaseAsset(t, "build/windows/installer/project.nsi")
	if count := strings.Count(project, "sign.ps1"); count != 3 {
		t.Fatalf("Windows installer signing hooks = %d, want application, uninstaller, and installer", count)
	}
	for _, directive := range []string{"!system", "!uninstfinalize", "!finalize"} {
		if !strings.Contains(project, directive) {
			t.Fatalf("Windows installer is missing %s signing hook", directive)
		}
	}
}

func TestLinuxPackageMetadata(t *testing.T) {
	desktopEntry := readReleaseAsset(t, "build/linux/com.sqlwarden.desktop.desktop")
	for _, value := range []string{
		"Exec=sqlwarden-desktop",
		"Icon=com.sqlwarden.desktop",
		"Categories=Development;Database;",
	} {
		if !strings.Contains(desktopEntry, value) {
			t.Fatalf("Linux desktop entry is missing %q", value)
		}
	}
	packageConfig := readReleaseAsset(t, "build/linux/nfpm.yaml")
	for _, dependency := range []string{"libgtk-3-0", "libwebkit2gtk-4.1-0"} {
		if !strings.Contains(packageConfig, dependency) {
			t.Fatalf("Linux package is missing %q dependency", dependency)
		}
	}
}

func readReleaseAsset(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
