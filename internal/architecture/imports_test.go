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
		"access": true, "audit": true, "completion": true, "config": true, "files": true,
		"identity": true, "jobs": true, "schema": true, "platform": true, "database": true, "orgs": true, "profile": true,
		"rbac": true,
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
			if strings.HasPrefix(rel, "internal/platform/clientip/") && strings.Contains(strings.Split(importPath, "/")[0], ".") {
				t.Errorf("%s imports %q; platform/clientip may import only the standard library", rel, importPath)
			}
			if parts[0] != "ee" && rel != "cmd/sqlwarden/edition_ee.go" && (importPath == "github.com/sqlwarden/ee" || strings.HasPrefix(importPath, "github.com/sqlwarden/ee/")) {
				t.Errorf("%s imports enterprise implementation %q", rel, importPath)
			}
			if applicationModules[owner] {
				for _, forbidden := range []string{"github.com/sqlwarden/internal/web"} {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						t.Errorf("%s imports outer adapter %q", rel, importPath)
					}
				}
			}
			if authzServicePackages[owner] {
				for _, forbidden := range []string{"internal/web", "internal/app", "internal/profile"} {
					full := "github.com/sqlwarden/" + forbidden
					if importPath == full || strings.HasPrefix(importPath, full+"/") {
						t.Errorf("%s imports %q; authorization services must not depend on adapters, the composition root, or profiles", rel, importPath)
					}
				}
			}
			if owner == "services" {
				for _, forbidden := range []string{"internal/web", "internal/app", "internal/profile"} {
					full := "github.com/sqlwarden/" + forbidden
					if importPath == full || strings.HasPrefix(importPath, full+"/") {
						t.Errorf("%s imports %q; the service graph is built below transports, the composition root, and profiles", rel, importPath)
					}
				}
			}
			if owner == "web" {
				for _, forbidden := range webForbiddenImports {
					full := "github.com/sqlwarden/" + forbidden
					if importPath == full || strings.HasPrefix(importPath, full+"/") {
						t.Errorf("%s imports %q; handlers reach metadata, secrets, and engines only through services", rel, importPath)
					}
				}
			}
			if (owner == "files" || owner == "jobs") && (importPath == "github.com/sqlwarden/internal/execution" || importPath == "github.com/sqlwarden/internal/connection") {
				t.Errorf("%s imports %q; only query, exportjob, and execution reach live sessions", rel, importPath)
			}
			// credentials adapts stored TLS/SSH documents into the connection
			// package's input types, so it is the only other legitimate importer.
			if importPath == "github.com/sqlwarden/internal/connection" && owner != "execution" && owner != "credentials" && owner != "connection" {
				t.Errorf("%s imports internal/connection; only internal/execution and internal/credentials may", rel)
			}
			if owner == "credentials" && importPath == "github.com/sqlwarden/internal/database" {
				t.Errorf("%s imports internal/database; credentials must depend on metadata ports", rel)
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

// authzServicePackages own the authorization decisions for their resources.
var authzServicePackages = map[string]bool{"query": true, "catalog": true, "files": true, "exportjob": true, "jobs": true, "settings": true, "orgs": true, "rbac": true, "identity": true, "keyrotation": true}

// webForbiddenImports are the packages production web code must not import,
// each also banning its subpackages. Service packages own type aliases for
// the metadata and engine types handlers encode, so handlers never need these.
var webForbiddenImports = []string{
	"internal/database",
	"internal/encrypt",
	"internal/connection",
	"internal/engine",
	"internal/schema",
	"internal/completion",
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
