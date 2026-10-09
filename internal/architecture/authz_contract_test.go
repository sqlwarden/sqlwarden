package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// authzExemptMethods lists exported Service methods that run without a caller.
// Everything else on an authorization service must take (ctx, access.Principal).
var authzExemptMethods = map[string]map[string]string{
	"files": {
		"ReapContentDeletionsOnce": "background reaper with no caller",
		"RunContentDeletionReaper": "background reaper with no caller",
	},
	"exportjob": {
		"RunJob": "job runner entry point; it derives the owner principal from the stored job input",
	},
}

func TestServiceMethodsTakeContextAndPrincipal(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range []string{"query", "files", "exportjob", "jobs"} {
		t.Run(pkg, func(t *testing.T) {
			fset := token.NewFileSet()
			dir := filepath.Join(root, "internal", pkg)
			parsed, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
				return !strings.HasSuffix(info.Name(), "_test.go")
			}, 0)
			if err != nil {
				t.Fatal(err)
			}
			checked := 0
			seenExempt := map[string]bool{}
			for _, p := range parsed {
				for _, file := range p.Files {
					for _, decl := range file.Decls {
						fn, ok := decl.(*ast.FuncDecl)
						if !ok || fn.Recv == nil || !fn.Name.IsExported() || len(fn.Recv.List) != 1 || receiverName(fn.Recv.List[0].Type) != "Service" {
							continue
						}
						if _, exempt := authzExemptMethods[pkg][fn.Name.Name]; exempt {
							seenExempt[fn.Name.Name] = true
							continue
						}
						checked++
						params := flattenParams(fn.Type.Params)
						if len(params) < 2 || !isSelector(params[0], "context", "Context") || !isSelector(params[1], "access", "Principal") {
							t.Errorf("%s.Service.%s must take (context.Context, access.Principal, ...)", pkg, fn.Name.Name)
						}
					}
				}
			}
			if checked == 0 {
				t.Fatalf("no exported Service methods found in internal/%s", pkg)
			}
			for name := range authzExemptMethods[pkg] {
				if !seenExempt[name] {
					t.Errorf("exempt method %s.Service.%s no longer exists; remove it from authzExemptMethods", pkg, name)
				}
			}
		})
	}
}

func flattenParams(fields *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	if fields == nil {
		return out
	}
	for _, field := range fields.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			out = append(out, field.Type)
		}
	}
	return out
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}
