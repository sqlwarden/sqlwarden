package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageGraphHasNoCycles(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./...")
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list rejected the package graph: %v\n%s", err, output)
	}
}

func TestConstructorsDoNotStartBackgroundWork(t *testing.T) {
	walkProductionGo(t, func(_ string, rel string, file *ast.File, fset *token.FileSet) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Body == nil || !strings.HasPrefix(function.Name.Name, "New") {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch current := node.(type) {
				case *ast.GoStmt:
					t.Errorf("%s:%d constructor %s launches a goroutine", rel, fset.Position(current.Go).Line, function.Name.Name)
				case *ast.CallExpr:
					selector, ok := current.Fun.(*ast.SelectorExpr)
					if ok && strings.HasPrefix(selector.Sel.Name, "Start") {
						t.Errorf("%s:%d constructor %s invokes lifecycle method %s", rel, fset.Position(current.Pos()).Line, function.Name.Name, selector.Sel.Name)
					}
				}
				return true
			})
		}
	})
}

func TestStartedComponentsExposeShutdownPair(t *testing.T) {
	type methods map[string]token.Position
	starts := make(map[string]methods)
	stops := make(map[string]bool)

	walkProductionGo(t, func(_ string, rel string, file *ast.File, fset *token.FileSet) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			receiver := receiverName(function.Recv.List[0].Type)
			if receiver == "" {
				continue
			}
			qualified := filepath.Dir(rel) + ":" + file.Name.Name + "." + receiver
			switch function.Name.Name {
			case "Start", "StartReaper":
				if starts[qualified] == nil {
					starts[qualified] = make(methods)
				}
				starts[qualified][function.Name.Name] = fset.Position(function.Pos())
			case "Close", "Stop":
				stops[qualified] = true
			}
		}
	})

	for component, methods := range starts {
		if stops[component] {
			continue
		}
		for method, position := range methods {
			t.Errorf("%s:%d %s.%s has no Close or Stop method", filepath.ToSlash(position.Filename), position.Line, component, method)
		}
	}
}

func walkProductionGo(t *testing.T, visit func(path, rel string, file *ast.File, fset *token.FileSet)) {
	t.Helper()
	root := repoRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == ".codegraph" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		visit(path, filepath.ToSlash(rel), file, fset)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func receiverName(expression ast.Expr) string {
	switch receiver := expression.(type) {
	case *ast.Ident:
		return receiver.Name
	case *ast.StarExpr:
		return receiverName(receiver.X)
	case *ast.IndexExpr:
		return receiverName(receiver.X)
	case *ast.IndexListExpr:
		return receiverName(receiver.X)
	default:
		return ""
	}
}
