//go:build !bindings

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingLogWriterBoundsRetainedLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sqlwarden.log")
	writer, err := openRotatingLog(path, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, contents := range []string{"first---", "second--", "third---"} {
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(candidate); err != nil {
			t.Fatalf("retained log %q: %v", candidate, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("log retention exceeded its bound: %v", err)
	}
}
