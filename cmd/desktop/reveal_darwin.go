//go:build darwin

package main

func openDirectoryNative(path string) error {
	return startDirectoryOpener("open", path)
}
