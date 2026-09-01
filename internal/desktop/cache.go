package desktop

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const DefaultCacheLimitBytes = int64(512 << 20)

type cacheEntry struct {
	path    string
	size    int64
	modTime int64
}

// PruneCache removes the oldest cache files until the bounded cache is within
// limit. Logs are independently rotated and are deliberately excluded.
func PruneCache(paths Paths, limit int64) error {
	if limit <= 0 {
		limit = DefaultCacheLimitBytes
	}
	entries := make([]cacheEntry, 0)
	var total int64
	err := filepath.WalkDir(paths.Cache, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != paths.Cache && filepath.Clean(path) == filepath.Clean(paths.Logs) {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		entries = append(entries, cacheEntry{path: path, size: info.Size(), modTime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil || total <= limit {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].modTime < entries[j].modTime })
	for _, entry := range entries {
		if total <= limit {
			break
		}
		if err := os.Remove(entry.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= entry.size
	}
	return nil
}
