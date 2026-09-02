//go:build linux

package main

func openDirectoryNative(path string) error {
	return startDirectoryOpener("xdg-open", path)
}
