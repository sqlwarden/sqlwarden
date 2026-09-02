package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func applyDesktopBranding(app *options.App) {
	app.Windows = &windows.Options{DisableWindowIcon: false, Theme: windows.SystemDefault}
	app.Mac = &mac.Options{
		About: &mac.AboutInfo{
			Title: "SQLWarden",
			Icon:  appIcon,
		},
	}
	app.Linux = &linux.Options{
		Icon:             appIcon,
		ProgramName:      "sqlwarden-desktop",
		WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
	}
}
