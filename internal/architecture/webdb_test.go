package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// webDatabaseUseCeiling is the number of app.db selector expressions in
// production web code. Lower it whenever a change removes uses. It must never
// rise, and it reaches 0 when every handler goes through a service.
const webDatabaseUseCeiling = 228

func TestWebDatabaseUseOnlyDecreases(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "internal", "web")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "db" {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "app" {
				count++
			}
			return true
		})
	}
	if count > webDatabaseUseCeiling {
		t.Fatalf("internal/web has %d app.db uses, ceiling is %d; route new data access through a service", count, webDatabaseUseCeiling)
	}
	if count < webDatabaseUseCeiling {
		t.Fatalf("internal/web has %d app.db uses, ceiling is %d; lower webDatabaseUseCeiling to %d", count, webDatabaseUseCeiling, count)
	}
}
