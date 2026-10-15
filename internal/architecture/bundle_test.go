package architecture

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// webBundlePorts are the field types of services.Services that are not
// authorization service types. Each is a narrow port that cannot read or
// change data around service authorization.
var webBundlePorts = map[string]string{
	"settings.Sender":              "sends notification email; it cannot replace the live mailer or read SMTP settings",
	"identity.Authentication":      "builds the bearer session authenticator and instance admin lookup for the request chain without exposing the auth store",
	"catalog.ConnectErrorRedactor": "removes resolved connection secrets from a connect failure message without returning the secrets",
}

// webDependencyFields are the exported fields of web.Dependencies and their
// types. A new field, or a changed type, must be added here with its reason.
var webDependencyFields = map[string]struct{ typ, reason string }{
	"Config":                {"config.Config", "static process configuration the server, middleware, and token issuance read"},
	"Logger":                {"*slog.Logger", "process logger"},
	"Services":              {"*services.Services", "the transport bundle; TestWebServiceBundleHoldsOnlyServicesAndPorts pins its fields"},
	"SettingsAppliers":      {"[]settings.Applier", "closures from services.Infra that apply instance settings; they expose no handle"},
	"Edition":               {"EditionCapabilities", "edition name and feature descriptors for the capabilities endpoint"},
	"EditionHandler":        {"http.Handler", "edition routes mounted under /ee; edition handlers authorize through their own services"},
	"Authenticators":        {"[]identity.Authenticator", "edition authenticators appended to the request authentication chain"},
	"DecorateAuthenticator": {"func(identity.Authenticator) identity.Authenticator", "edition wrapper around the session authenticator"},
	"RequestPolicies":       {"[]identity.RequestPolicy", "edition request policies evaluated after authentication"},
	"PostureProviders":      {"[]identity.PostureProvider", "edition posture signals collected after authentication"},
	"TrustedProxies":        {"[]netip.Prefix", "proxy networks whose X-Forwarded-For hops are trusted"},
	"Invitations":           {"orgs.InvitationPolicy", "profile flag reporting whether organization invitations are enabled"},
}

// webAllowedImports are the module packages production web code may import,
// each with what web uses it for. Adding an import means adding it here; a
// package in webForbiddenImports can never be added.
var webAllowedImports = map[string]string{
	"assets":                          "embedded frontend files served by the static handler",
	"internal/access":                 "Principal, request attributes, and the shared access errors handlers map to statuses",
	"internal/catalog":                "catalog service and the request, response, and error types handlers encode",
	"internal/config":                 "static configuration types",
	"internal/credentials":            "SecretName for secret request bodies and the reveal route; no provider or secret value",
	"internal/execution":              "session and cursor ids, result and limit types, and the failure types the error mapping classifies",
	"internal/exportjob":              "export service",
	"internal/exports":                "ErrByteLimitExceeded, mapped for synchronous exports",
	"internal/files":                  "workspace file service and its types",
	"internal/identity":               "identity service and the request authentication chain types",
	"internal/jobs":                   "job service and its validation error",
	"internal/keyrotation":            "key rotation options, report, and the operator principal of the rotate-keys command",
	"internal/orgs":                   "organization service and the invitation policy",
	"internal/platform/clientip":      "client IP resolution over trusted proxies",
	"internal/platform/observability": "runtime log level switching and request id context",
	"internal/query":                  "query service and its error and result types",
	"internal/rbac":                   "RBAC service and its types",
	"internal/request":                "JSON request decoding",
	"internal/response":               "JSON response and error envelopes",
	"internal/services":               "the transport bundle",
	"internal/settings":               "settings service, instance settings, and the Applier type",
	"internal/smtp":                   "ErrDisabled, so a disabled mailer is not reported as an error",
	"internal/validator":              "request validation",
	"pkg/result":                      "normalized query result types handlers encode",
}

const modulePrefix = "github.com/sqlwarden/"

func TestWebServiceBundleHoldsOnlyServicesAndPorts(t *testing.T) {
	for port, reason := range webBundlePorts {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("port %s needs a reason", port)
		}
	}
	fields := exportedStructFields(t, filepath.Join(repoRoot(t), "internal", "services"), "Services")
	used := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		typ := fields[name]
		if pkg, ok := strings.CutSuffix(strings.TrimPrefix(typ, "*"), ".Service"); ok && strings.HasPrefix(typ, "*") && authzServicePackages[pkg] {
			continue
		}
		if _, ok := webBundlePorts[typ]; ok {
			used[typ] = true
			continue
		}
		t.Errorf("services.Services.%s has type %s, which is neither an authorization service nor a port in webBundlePorts; keep infrastructure handles on services.Infra", name, typ)
	}
	for port := range webBundlePorts {
		if !used[port] {
			t.Errorf("webBundlePorts lists %s, which no services.Services field uses; remove it", port)
		}
	}
}

func TestWebDependenciesFieldsAreAllowListed(t *testing.T) {
	fields := exportedStructFields(t, filepath.Join(repoRoot(t), "internal", "web"), "Dependencies")
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		want, ok := webDependencyFields[name]
		if !ok {
			t.Errorf("web.Dependencies.%s (%s) is not in webDependencyFields", name, fields[name])
			continue
		}
		if strings.TrimSpace(want.reason) == "" {
			t.Errorf("webDependencyFields[%q] needs a reason", name)
		}
		if fields[name] != want.typ {
			t.Errorf("web.Dependencies.%s has type %s, webDependencyFields says %s", name, fields[name], want.typ)
		}
	}
	for name := range webDependencyFields {
		if _, ok := fields[name]; !ok {
			t.Errorf("webDependencyFields lists %s, which web.Dependencies no longer has; remove it", name)
		}
	}
}

func TestWebNeverReferencesServiceInfrastructure(t *testing.T) {
	fset := token.NewFileSet()
	for _, file := range parseProductionDir(t, fset, filepath.Join(repoRoot(t), "internal", "web"), 0) {
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && isSelector(sel, "services", "Infra") {
				t.Errorf("%s: web references services.Infra; infrastructure stays with the composition root", fset.Position(sel.Pos()))
			}
			return true
		})
	}
}

func TestWebImportsAreAllowListed(t *testing.T) {
	for _, forbidden := range webForbiddenImports {
		if _, ok := webAllowedImports[forbidden]; ok {
			t.Errorf("webAllowedImports lists %s, which webForbiddenImports denies", forbidden)
		}
	}
	for pkg, reason := range webAllowedImports {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("webAllowedImports[%q] needs a reason", pkg)
		}
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	for path, file := range parseProductionDir(t, fset, filepath.Join(repoRoot(t), "internal", "web"), parser.ImportsOnly) {
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			pkg, ok := strings.CutPrefix(importPath, modulePrefix)
			if !ok {
				continue
			}
			seen[pkg] = true
			if _, allowed := webAllowedImports[pkg]; !allowed {
				t.Errorf("%s imports %q, which is not in webAllowedImports", filepath.Base(path), importPath)
			}
		}
	}
	for pkg := range webAllowedImports {
		if !seen[pkg] {
			t.Errorf("webAllowedImports lists %s, which production web no longer imports; remove it", pkg)
		}
	}
}

// exportedStructFields returns the exported field names of the named struct
// type declared in the production files of dir, mapped to their type
// expressions as written.
func exportedStructFields(t *testing.T, dir, typeName string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	for _, file := range parseProductionDir(t, fset, dir, 0) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts := spec.(*ast.TypeSpec)
				if ts.Name.Name != typeName {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					t.Fatalf("%s in %s is not a struct", typeName, dir)
				}
				fields := map[string]string{}
				for _, field := range st.Fields.List {
					typ := types.ExprString(field.Type)
					if len(field.Names) == 0 {
						t.Errorf("%s embeds %s; embedded fields bypass the field allow-list", typeName, typ)
						continue
					}
					for _, name := range field.Names {
						if name.IsExported() {
							fields[name.Name] = typ
						}
					}
				}
				return fields
			}
		}
	}
	t.Fatalf("type %s not found in %s", typeName, dir)
	return nil
}

func parseProductionDir(t *testing.T, fset *token.FileSet, dir string, mode parser.Mode) map[string]*ast.File {
	t.Helper()
	pkg, err := build.Default.ImportDir(dir, build.ImportComment)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	names := append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...)
	for _, name := range names {
		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, mode)
		if err != nil {
			t.Fatal(err)
		}
		files[path] = file
	}
	return files
}
