package main

import _ "embed"

// appIcon is the source used by Wails for native window and application icons.
//
//go:embed build/appicon.png
var appIcon []byte
