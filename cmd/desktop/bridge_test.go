//go:build !bindings

package main

import (
	"errors"
	"testing"

	desktopconfig "github.com/sqlwarden/internal/desktop"
)

func TestDesktopBridgeRevealsConfiguredDirectories(t *testing.T) {
	original := openDirectory
	t.Cleanup(func() { openDirectory = original })

	var paths []string
	openDirectory = func(path string) error {
		paths = append(paths, path)
		return nil
	}
	bridge := newDesktopBridge(nil, desktopconfig.Paths{DataDir: "data-dir", Logs: "logs-dir"}, nil)

	if err := bridge.RevealDataDirectory(); err != nil {
		t.Fatal(err)
	}
	if err := bridge.RevealLogDirectory(); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "data-dir" || paths[1] != "logs-dir" {
		t.Fatalf("revealed paths = %v", paths)
	}
}

func TestDesktopBridgeReturnsRevealFailure(t *testing.T) {
	original := openDirectory
	t.Cleanup(func() { openDirectory = original })

	want := errors.New("explorer unavailable")
	openDirectory = func(string) error { return want }
	bridge := newDesktopBridge(nil, desktopconfig.Paths{DataDir: "data-dir"}, nil)

	if err := bridge.RevealDataDirectory(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
