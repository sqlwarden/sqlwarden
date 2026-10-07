package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// walkProductionFiles calls visit for every non-test Go file in the root
// module. Dot-prefixed directories (sibling worktrees, tool caches) and
// directories that hold their own go.mod are not part of the module's package
// set, so they are skipped.
func walkProductionFiles(root string, visit func(path string) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return visit(path)
	})
}

func TestWalkProductionFilesSkipsDotDirsAndNestedModules(t *testing.T) {
	root := t.TempDir()
	files := map[string]bool{
		"main.go":               true,
		"internal/a/a.go":       true,
		"internal/a/a_test.go":  false,
		".worktrees/x/go.mod":   false,
		".worktrees/x/cmd/x.go": false,
		".git/hooks/h.go":       false,
		"tools/go.mod":          false,
		"tools/t.go":            false,
		"tools/deep/d.go":       false,
		"node_modules/pkg/n.go": false,
		"internal/sub/go.mod":   false,
		"internal/sub/s.go":     false,
		"internal/b/b.go":       true,
	}
	for name := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	err := walkProductionFiles(root, func(path string) error {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		seen[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		if seen[name] != want {
			t.Errorf("%s visited = %v, want %v", name, seen[name], want)
		}
	}
}
