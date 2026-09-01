package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneCacheRemovesOldestFilesAndPreservesLogs(t *testing.T) {
	paths, err := ResolvePaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := createDesktopDirectories(paths); err != nil {
		t.Fatal(err)
	}
	oldest := filepath.Join(paths.Cache, "old.cache")
	newest := filepath.Join(paths.Cache, "new.cache")
	logFile := filepath.Join(paths.Logs, "sqlwarden.log")
	for _, path := range []string{oldest, newest, logFile} {
		if err := os.WriteFile(path, make([]byte, 8), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldest, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := PruneCache(paths, 8); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("oldest cache entry was not removed: %v", err)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Fatalf("newest cache entry was removed: %v", err)
	}
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("rotated log scope was pruned: %v", err)
	}
}
