//go:build bindings

package main

import (
	desktopconfig "github.com/sqlwarden/internal/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

// Binding generation executes a temporary binary. Keep this entrypoint free of
// filesystem and database initialization so desktop builds have no side effects.
func main() {
	_ = wails.Run(&options.App{Bind: []interface{}{newDesktopBridge(nil, desktopconfig.Paths{}, nil)}})
}
