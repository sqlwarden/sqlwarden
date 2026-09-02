//go:build !bindings

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sqlwarden/internal/desktop"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type windowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximized bool `json:"maximized"`
}

func loadWindowState(paths desktop.Paths) windowState {
	state := windowState{Width: 1440, Height: 900}
	contents, err := os.ReadFile(filepath.Join(paths.ConfigDir, "window.json"))
	if err != nil || json.Unmarshal(contents, &state) != nil {
		return windowState{Width: 1440, Height: 900}
	}
	if state.Width < 1024 || state.Height < 680 {
		return windowState{Width: 1440, Height: 900}
	}
	return state
}

func (state windowState) startState() options.WindowStartState {
	if state.Maximized {
		return options.Maximised
	}
	return options.Normal
}

func applyWindowPosition(ctx context.Context, state windowState) {
	if state.X != 0 || state.Y != 0 {
		wailsruntime.WindowSetPosition(ctx, state.X, state.Y)
	}
}

func saveWindowState(ctx context.Context, paths desktop.Paths) {
	width, height := wailsruntime.WindowGetSize(ctx)
	x, y := wailsruntime.WindowGetPosition(ctx)
	state := windowState{
		Width:     width,
		Height:    height,
		X:         x,
		Y:         y,
		Maximized: wailsruntime.WindowIsMaximised(ctx),
	}
	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = writeAtomic(filepath.Join(paths.ConfigDir, "window.json"), append(contents, '\n'), 0o600)
}
