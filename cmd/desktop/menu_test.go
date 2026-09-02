//go:build !bindings

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	desktopconfig "github.com/sqlwarden/internal/desktop"
)

func TestNativeSQLLaunchPathIsQueuedBeforeFrontendStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.sql")
	if err := os.WriteFile(path, []byte("select 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := newDesktopBridge(nil, desktopconfig.Paths{}, nil)

	handleNativeOpenPath(bridge, path)

	requests := bridge.DrainOpenRequests()
	if len(requests.Files) != 1 {
		t.Fatalf("queued files = %+v", requests.Files)
	}
	if requests.Files[0].Path != path || requests.Files[0].Content != "select 1" {
		t.Fatalf("queued file = %+v", requests.Files[0])
	}
}

func TestDesktopMenuHasNamedPrimaryMenus(t *testing.T) {
	root := desktopMenu(newDesktopBridge(nil, desktopconfig.Paths{}, nil))
	labels := make([]string, 0, len(root.Items))
	for _, item := range root.Items {
		labels = append(labels, item.Label)
	}

	want := []string{"File", "Edit", "View", "Help"}
	for _, label := range want {
		if !containsLabel(labels, label) {
			t.Fatalf("desktop menu labels = %q, missing %q", labels, label)
		}
	}

	if runtime.GOOS == "darwin" {
		return
	}
	for _, item := range root.Items {
		if item.Label == "Edit" && item.SubMenu != nil && len(item.SubMenu.Items) >= 7 {
			return
		}
	}
	t.Fatal("non-macOS Edit menu has no actionable submenu")
}

func containsLabel(labels []string, target string) bool {
	for _, label := range labels {
		if label == target {
			return true
		}
	}
	return false
}
