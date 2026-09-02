//go:build windows

package main

func openDirectoryNative(path string) error {
	return startDirectoryOpener("explorer.exe", path)
}
