package main

import (
	"fmt"
	"os/exec"
)

func startDirectoryOpener(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open desktop directory: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}
