//go:build !bindings

package main

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

func TestAppIconIsNativeReady(t *testing.T) {
	image, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatalf("decode app icon: %v", err)
	}
	bounds := image.Bounds()
	if bounds.Dx() != 1024 || bounds.Dy() != 1024 {
		t.Fatalf("app icon size = %dx%d, want 1024x1024", bounds.Dx(), bounds.Dy())
	}

	visible := bounds
	visible.Min = bounds.Max
	visible.Max = bounds.Min
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, alpha := image.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			if red != alpha || green != alpha || blue != alpha {
				t.Fatalf("app icon contains a non-white visible pixel at %d,%d", x, y)
			}
			if x < visible.Min.X {
				visible.Min.X = x
			}
			if y < visible.Min.Y {
				visible.Min.Y = y
			}
			if x >= visible.Max.X {
				visible.Max.X = x + 1
			}
			if y >= visible.Max.Y {
				visible.Max.Y = y + 1
			}
		}
	}
	if visible.Dx() < 850 || visible.Dy() < 850 {
		t.Fatalf("visible app icon size = %dx%d, want at least 850x850", visible.Dx(), visible.Dy())
	}
}

func TestApplyDesktopBranding(t *testing.T) {
	app := &options.App{}
	applyDesktopBranding(app)

	if app.Windows == nil || app.Windows.DisableWindowIcon {
		t.Fatal("Windows window icon is disabled")
	}
	if app.Mac == nil || app.Mac.About == nil {
		t.Fatal("macOS application branding is missing")
	}
	if app.Mac.About.Title != "SQLWarden" || !bytes.Equal(app.Mac.About.Icon, appIcon) {
		t.Fatal("macOS application branding does not use the SQLWarden icon")
	}
	if app.Linux == nil || !bytes.Equal(app.Linux.Icon, appIcon) {
		t.Fatal("Linux window branding does not use the SQLWarden icon")
	}
	if app.Linux.ProgramName != "sqlwarden-desktop" {
		t.Fatalf("Linux program name = %q", app.Linux.ProgramName)
	}
	if app.Linux.WebviewGpuPolicy != linux.WebviewGpuPolicyNever {
		t.Fatalf("Linux GPU policy = %d, want the existing disabled policy", app.Linux.WebviewGpuPolicy)
	}
}
