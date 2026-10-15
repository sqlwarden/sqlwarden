package architecture

import (
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// authzExemptMethods lists exported Service methods that run without a caller,
// each with the reason it is safe. Everything else on the Service type of every
// package in authzServicePackages must take (ctx, access.Principal).
var authzExemptMethods = map[string]map[string]string{
	"files": {
		"ReapContentDeletionsOnce": "background reaper with no caller",
		"RunContentDeletionReaper": "background reaper with no caller",
	},
	"exportjob": {
		"RunJob": "job runner entry point; it derives the owner principal from the stored job input",
	},
	"settings": {
		"Instance":              "instance settings consumed by middleware, startup, and runtime operations before any caller is known",
		"EffectiveForOrg":       "callers outside the settings package pass a nil org and read instance-wide values only; org-scoped reads are made by settings methods that authorize the org first",
		"EffectiveForWorkspace": "limits enforced on the caller's own requests; the owning service has already authorized the workspace",
		"RotateSMTPPassword":    "stage of key rotation; keyrotation.Service authorizes the caller before invoking it",
	},
	"identity": {
		"Register":      "public self-registration; it runs before the caller has an account",
		"Login":         "public sign-in; the submitted credentials are what authenticate the caller",
		"Refresh":       "public token refresh; the refresh token is the credential and there is no access token yet",
		"Logout":        "public logout; the refresh token cookie is the credential, so it works after the access token expired",
		"SignInEnabled": "profile capability checked before the sign-in request is decoded; it reads no account data",
		"StartSession":  "issues a session for an account another service already verified, such as an accepted invitation",
		"Setup":         "first-run bootstrap; it runs before any account exists and seals itself once an instance admin exists",
		"SetupStatus":   "public setup state read before anyone can sign in; it reveals only whether setup is done",
	},
	"orgs": {
		"OnSchemaCachePurged": "wires the schema navigator cache observer at startup; it reads no organization data",
		"GetInvitation":       "public invitation lookup; the invitation token is the credential and the caller may be anonymous",
		"AcceptInvitation":    "public invitation acceptance; the invitation token is the credential and the caller may be anonymous or not yet have an account",
	},
}

func TestServiceMethodsTakeContextAndPrincipal(t *testing.T) {
	root := repoRoot(t)
	for pkg, methods := range authzExemptMethods {
		if !authzServicePackages[pkg] {
			t.Errorf("authzExemptMethods lists %q, which is not an authorization service package", pkg)
		}
		for name, reason := range methods {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("exempt method %s.Service.%s needs a reason", pkg, name)
			}
		}
	}
	pkgs := slices.Sorted(maps.Keys(authzServicePackages))
	for _, pkg := range pkgs {
		t.Run(pkg, func(t *testing.T) {
			fset := token.NewFileSet()
			dir := filepath.Join(root, "internal", pkg)
			checked := 0
			seenExempt := map[string]bool{}
			for _, file := range parseProductionDir(t, fset, dir, 0) {
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
