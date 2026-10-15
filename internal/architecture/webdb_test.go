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

// webDatabaseUseCeiling is the number of .db selector expressions in
// production web code, on any receiver. Every handler goes through a service,
// so it must stay 0. Counting every receiver keeps a renamed or aliased
// application value from hiding direct metadata access.
const webDatabaseUseCeiling = 0

func TestWebDatabaseUseOnlyDecreases(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "internal", "web")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var uses []string
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
			if ok && sel.Sel.Name == "db" {
				uses = append(uses, fset.Position(sel.Sel.Pos()).String())
			}
			return true
		})
	}
	if len(uses) > webDatabaseUseCeiling {
		t.Fatalf("internal/web has %d .db uses, ceiling is %d; route data access through a service:\n%s", len(uses), webDatabaseUseCeiling, strings.Join(uses, "\n"))
	}
	if len(uses) < webDatabaseUseCeiling {
		t.Fatalf("internal/web has %d .db uses, ceiling is %d; lower webDatabaseUseCeiling to %d", len(uses), webDatabaseUseCeiling, len(uses))
	}
}
