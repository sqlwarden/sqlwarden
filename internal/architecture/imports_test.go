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

// TestForbiddenProductionImports protects application, adapter, edition, and
// process-topology boundaries. Tests are excluded because external-package
// contract tests intentionally compose multiple layers.
func TestForbiddenProductionImports(t *testing.T) {
	repositoryRoot := repoRoot(t)
	fset := token.NewFileSet()
	applicationModules := map[string]bool{
		"access": true, "app": true, "audit": true, "catalog": true,
		"completion": true, "config": true, "files": true, "identity": true,
		"jobs": true, "schema": true, "settings": true,
	}

	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != repositoryRoot && (entry.Name() == ".git" || entry.Name() == ".codegraph" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		owner := ""
		if len(parts) > 1 && parts[0] == "internal" {
			owner = parts[1]
		}
		processKindLayer := owner == "app" || owner == "web" || parts[0] == "cmd"

		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if parts[0] != "ee" && (importPath == "github.com/sqlwarden/ee" || strings.HasPrefix(importPath, "github.com/sqlwarden/ee/")) {
				t.Errorf("%s imports enterprise implementation %q", rel, importPath)
			}
			if applicationModules[owner] {
				for _, forbidden := range []string{"github.com/sqlwarden/internal/web", "github.com/sqlwarden/internal/rpc", "github.com/sqlwarden/internal/realtime"} {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						t.Errorf("%s imports outer adapter %q", rel, importPath)
					}
				}
			}
			if processKindLayer && (strings.HasPrefix(importPath, "k8s.io/") || strings.HasPrefix(importPath, "sigs.k8s.io/")) {
				t.Errorf("%s imports Kubernetes API %q; process topology belongs in deployment manifests", rel, importPath)
			}
			if owner == "web" && (importPath == "github.com/sqlwarden/internal/connection" || strings.HasPrefix(importPath, "github.com/sqlwarden/internal/connection/")) {
				t.Errorf("%s bypasses execution runtime with connection import %q", rel, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join(packageDir(t), "..", ".."))
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture package directory")
	}
	return filepath.Dir(file)
}
