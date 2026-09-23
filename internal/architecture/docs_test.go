package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// skippedDirNames hold dependency or fixture Go files wherever they appear.
// The go tool also ignores directories beginning with "." or "_".
var skippedDirNames = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"testdata":     true,
}

// skippedRootPaths are repository-relative build-output and local-only
// directories listed in .gitignore.
var skippedRootPaths = map[string]bool{
	"assets/static":     true,
	"bin":               true,
	"dist":              true,
	"frontend/coverage": true,
	"postgres_data":     true,
	"references":        true,
	"tmp":               true,
}

func TestEveryGoPackageHasDocFile(t *testing.T) {
	root := repoRoot(t)
	dirs := goPackageDirs(t, root)
	if len(dirs) == 0 {
		t.Fatal("found no Go package directories; repository walk is broken")
	}

	for _, rel := range dirs {
		if problem := checkPackageDoc(filepath.Join(root, filepath.FromSlash(rel))); problem != "" {
			t.Errorf("%s: %s", rel, problem)
		}
	}
}

func TestRepositoryAgentGuidanceExists(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "AGENTS.md")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("AGENTS.md must be a regular file, found mode %s", info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatal("AGENTS.md must not be empty")
	}
}

// goPackageDirs returns the sorted, slash-separated repository-relative paths
// of every directory containing at least one .go file, including test-only
// packages.
func goPackageDirs(t *testing.T, root string) []string {
	t.Helper()
	seen := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := entry.Name()
		if entry.IsDir() {
			if skippedDirNames[name] || skippedRootPaths[rel] || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && strings.HasSuffix(name, ".go") {
			seen[pathpkg.Dir(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	return dirs
}

// checkPackageDoc reports why dir's package documentation is invalid, or ""
// when doc.go exists, carries the only package comment in the directory, and
// that comment names the package.
func checkPackageDoc(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err.Error()
	}
	fset := token.NewFileSet()
	var docFile *ast.File
	var sourcePackage, testPackage string
	var strayDocs []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly|parser.ParseComments)
		if err != nil {
			return "parse " + name + ": " + err.Error()
		}
		switch {
		case name == "doc.go":
			docFile = file
		case file.Doc != nil:
			strayDocs = append(strayDocs, name)
		}
		if strings.HasSuffix(name, "_test.go") {
			testPackage = strings.TrimSuffix(file.Name.Name, "_test")
		} else if name != "doc.go" {
			sourcePackage = file.Name.Name
		}
	}
	packageName := sourcePackage
	if packageName == "" {
		packageName = testPackage
	}
	if packageName == "" && docFile != nil {
		packageName = docFile.Name.Name
	}

	if docFile == nil {
		return "missing doc.go; add one with a \"// Package " + packageName + " ...\" comment"
	}
	if strings.HasSuffix(docFile.Name.Name, "_test") {
		return "doc.go declares external test package " + docFile.Name.Name + "; use package " + packageName
	}
	if docFile.Name.Name != packageName {
		return "doc.go declares package " + docFile.Name.Name + " but the directory's package is " + packageName
	}
	if docFile.Doc == nil || strings.TrimSpace(docFile.Doc.Text()) == "" {
		return "doc.go has no package comment directly above its package clause"
	}
	prefix := "Package " + packageName + " "
	if packageName == "main" {
		prefix = "Command " + filepath.Base(dir) + " "
	}
	if !strings.HasPrefix(docFile.Doc.Text(), prefix) {
		return "doc.go package comment must begin with \"" + prefix + "\""
	}
	if len(strayDocs) > 0 {
		return "package comment also appears in " + strings.Join(strayDocs, ", ") + "; keep it only in doc.go"
	}
	return ""
}
