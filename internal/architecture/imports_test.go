package architecture

import (
	"go/parser"
	"go/token"
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
		"access": true, "completion": true, "config": true, "files": true,
		"jobs": true, "schema": true, "platform": true, "database": true, "orgs": true,
	}

	err := walkProductionFiles(repositoryRoot, func(path string) error {
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
				for _, forbidden := range []string{"github.com/sqlwarden/internal/web"} {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						t.Errorf("%s imports outer adapter %q", rel, importPath)
					}
				}
			}
			if processKindLayer && (strings.HasPrefix(importPath, "k8s.io/") || strings.HasPrefix(importPath, "sigs.k8s.io/")) {
				t.Errorf("%s imports Kubernetes API %q; process topology belongs in deployment manifests", rel, importPath)
			}
			if importPath == "github.com/sqlwarden/internal/app" && parts[0] != "cmd" {
				t.Errorf("%s imports the composition root; only cmd/* may", rel)
			}
			if (owner == "platform" || owner == "config") && strings.HasPrefix(importPath, "github.com/sqlwarden/internal/") {
				allowed := owner == "config" && importPath == "github.com/sqlwarden/internal/validator"
				allowed = allowed || owner == "platform" && importPath == "github.com/sqlwarden/internal/version"
				if !allowed {
					t.Errorf("%s imports %q; %s must stay a leaf package", rel, importPath, owner)
				}
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
