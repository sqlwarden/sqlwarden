package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestForbiddenProductionImports protects the first application boundaries
// before later service extraction adds more packages to the rule set. Tests
// are excluded because external-package contract tests intentionally compose
// multiple layers.
func TestForbiddenProductionImports(t *testing.T) {
	internalRoot := filepath.Clean(filepath.Join(packageDir(t), ".."))
	fset := token.NewFileSet()

	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		owner := strings.Split(filepath.ToSlash(rel), "/")[0]

		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if owner != "ee" && strings.Contains(importPath, "/ee/") {
				t.Errorf("%s imports enterprise implementation %q", rel, importPath)
			}
			if owner == "app" || owner == "config" {
				for _, forbidden := range []string{"/internal/web", "/internal/rpc", "/internal/realtime"} {
					if strings.Contains(importPath, forbidden) {
						t.Errorf("%s imports outer adapter %q", rel, importPath)
					}
				}
			}
			if owner == "app" && (strings.Contains(importPath, "k8s.io/") || strings.Contains(importPath, "sigs.k8s.io/")) {
				t.Errorf("%s imports Kubernetes API %q", rel, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture package directory")
	}
	return filepath.Dir(file)
}
