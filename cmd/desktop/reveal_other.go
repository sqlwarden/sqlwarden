//go:build !windows && !darwin && !linux

package main

import "errors"

func openDirectoryNative(string) error {
	return errors.New("revealing directories is not supported on this platform")
}
