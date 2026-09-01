//go:build !bindings

package main

import (
	"runtime"
	"testing"

	desktopconfig "github.com/sqlwarden/internal/desktop"
)

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
