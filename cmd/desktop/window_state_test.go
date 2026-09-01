//go:build !bindings

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/desktop"
	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestLoadWindowStateValidatesPersistedSize(t *testing.T) {
	paths := desktop.Paths{ConfigDir: t.TempDir()}
	statePath := filepath.Join(paths.ConfigDir, "window.json")
	if err := os.WriteFile(statePath, []byte(`{"width":1600,"height":1000,"x":40,"y":50,"maximized":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state := loadWindowState(paths)
	if state.Width != 1600 || state.Height != 1000 || state.X != 40 || state.Y != 50 {
		t.Fatalf("window state = %+v", state)
	}
	if state.startState() != options.Maximised {
		t.Fatal("maximized window state was not restored")
	}

	if err := os.WriteFile(statePath, []byte(`{"width":100,"height":100}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state = loadWindowState(paths)
	if state.Width != 1440 || state.Height != 900 {
		t.Fatalf("invalid window state did not fall back: %+v", state)
	}
}
