//go:build !bindings

package main

import (
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func desktopMenu(bridge *DesktopBridge) *menu.Menu {
	root := menu.NewMenu()
	if stdruntime.GOOS == "darwin" {
		root.Append(menu.AppMenu())
	}
	file := root.AddSubmenu("File")
	file.AddText("Open SQL File…", keys.CmdOrCtrl("o"), func(*menu.CallbackData) {
		opened, err := bridge.OpenSQLFile()
		if err == nil && opened.Path != "" {
			bridge.dispatchFileOpened(opened)
		}
	})
	file.AddText("Save", keys.CmdOrCtrl("s"), commandCallback(bridge, "file.save"))
	file.AddText("Save As…", keys.Combo("s", keys.CmdOrCtrlKey, keys.ShiftKey), commandCallback(bridge, "file.save-as"))
	file.AddSeparator()
	file.AddText("Open SQLite Database…", nil, func(*menu.CallbackData) {
		path, err := bridge.ChooseSQLiteFile()
		if err == nil && path != "" {
			bridge.dispatchSQLiteSelected(path)
		}
	})
	if stdruntime.GOOS != "darwin" {
		file.AddSeparator()
		file.AddText("Quit", keys.CmdOrCtrl("q"), func(*menu.CallbackData) {
			wailsruntime.Quit(bridge.context())
		})
	}

	root.Append(menu.EditMenu())
	view := root.AddSubmenu("View")
	view.AddText("Command Palette…", keys.Combo("p", keys.CmdOrCtrlKey, keys.ShiftKey), commandCallback(bridge, "view.command-palette"))
	view.AddText("Toggle Sidebar", keys.CmdOrCtrl("b"), commandCallback(bridge, "view.toggle-sidebar"))
	view.AddText("Reload", keys.CmdOrCtrl("r"), func(*menu.CallbackData) {
		wailsruntime.WindowReload(bridge.context())
	})
	if stdruntime.GOOS == "darwin" {
		root.Append(menu.WindowMenu())
	}
	help := root.AddSubmenu("Help")
	help.AddText("Settings", keys.CmdOrCtrl(","), commandCallback(bridge, "app.settings"))
	help.AddText("Open Logs", nil, func(*menu.CallbackData) { _ = bridge.RevealLogDirectory() })
	help.AddText("Check for Updates…", nil, func(*menu.CallbackData) { bridge.OpenReleasePage() })
	return root
}

func commandCallback(bridge *DesktopBridge, command string) menu.Callback {
	return func(*menu.CallbackData) {
		wailsruntime.EventsEmit(bridge.context(), "desktop:command", command)
	}
}

func handleNativeOpenPath(bridge *DesktopBridge, path string) {
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".sql":
		contents, err := readBoundedFile(path, 20<<20)
		if err == nil {
			bridge.dispatchFileOpened(NativeTextFile{
				Path: path, Name: filepath.Base(path), Content: string(contents),
			})
		}
	case ".db", ".sqlite", ".sqlite3":
		bridge.dispatchSQLiteSelected(path)
	}
}
