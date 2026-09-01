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

func TestDesktopBridgeQueuesOpenRequestsUntilFrontendIsReady(t *testing.T) {
	bridge := newDesktopBridge(nil, desktopconfig.Paths{}, nil)
	file := NativeTextFile{Path: "/tmp/query.sql", Name: "query.sql", Content: "select 1"}
	bridge.dispatchFileOpened(file)
	bridge.dispatchSQLiteSelected("/tmp/local.db")

	requests := bridge.DrainOpenRequests()
	if len(requests.Files) != 1 || requests.Files[0] != file {
		t.Fatalf("queued SQL files = %+v", requests.Files)
	}
	if len(requests.SQLiteFiles) != 1 || requests.SQLiteFiles[0] != "/tmp/local.db" {
		t.Fatalf("queued SQLite files = %+v", requests.SQLiteFiles)
	}
	if next := bridge.DrainOpenRequests(); len(next.Files) != 0 || len(next.SQLiteFiles) != 0 {
		t.Fatalf("open requests were not drained: %+v", next)
	}
}
