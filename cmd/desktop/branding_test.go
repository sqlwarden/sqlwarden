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

	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	var hasBlue, hasWhite bool
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, alpha := image.At(x, y).RGBA()
			if alpha < 0x8000 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			hasBlue = hasBlue || (red < 0x1000 && green > 0x5000 && blue > 0xC000)
			hasWhite = hasWhite || (red > 0xE000 && green > 0xE000 && blue > 0xE000)
		}
	}
	if !hasBlue || !hasWhite {
		t.Fatal("app icon must contain an opaque brand-blue background and white mark")
	}
	if minX != 100 || minY != 100 || maxX != 923 || maxY != 923 {
		t.Fatalf("app icon opaque bounds = (%d,%d)-(%d,%d), want standard 824px macOS footprint", minX, minY, maxX, maxY)
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
