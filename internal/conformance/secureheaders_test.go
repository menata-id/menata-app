package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestRoutesRegistersSecureHeaders guards security audit 2026-09-19's M3/H2 fix: a future refactor
// of internal/web/router.go's Routes() must not silently drop the r.Use(secureHeaders) call that
// sets baseline security response headers (nosniff, CSP, frame-ancestors) on every response,
// including /uploads/*. No database, no live server needed -- this is a structural check over
// router.go's own source, the same static-analysis shape TestHandlersStaySmall and
// TestNavigationRoutesAreRegistered already use, so it runs in the fast pre-commit gate
// (menata-app-document's guides/primitive-security-audit.md "where this gets enforced" table).
func TestRoutesRegistersSecureHeaders(t *testing.T) {
	path := filepath.Join(repoRoot(), "internal", "web", "router.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var routesFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "Routes" {
			routesFunc = fn
			break
		}
	}
	if routesFunc == nil {
		t.Fatal("router.go declares no Routes function")
	}

	found := false
	ast.Inspect(routesFunc.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Use" {
			return true
		}
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == "secureHeaders" {
				found = true
			}
		}
		return true
	})

	if !found {
		t.Error("Routes() no longer calls r.Use(secureHeaders) -- baseline security headers (nosniff, CSP, frame-ancestors) would stop applying to every response")
	}
}

// TestRoutesRegistersCSRFProtect is TestRoutesRegistersSecureHeaders' own counterpart for security
// audit 2026-09-19's L1: a future refactor of Routes() must not silently drop the
// r.Use(csrfProtect(d.Cfg)) call, since that's the only place the double-submit CSRF check
// (internal/web/csrf.go) is actually wired into the request path -- every public route and every
// pr/ar-grouped route relies on this one global registration, none of them check it individually.
func TestRoutesRegistersCSRFProtect(t *testing.T) {
	path := filepath.Join(repoRoot(), "internal", "web", "router.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var routesFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "Routes" {
			routesFunc = fn
			break
		}
	}
	if routesFunc == nil {
		t.Fatal("router.go declares no Routes function")
	}

	found := false
	ast.Inspect(routesFunc.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Use" {
			return true
		}
		for _, arg := range call.Args {
			// r.Use(csrfProtect(d.Cfg)) -- the argument is itself a call expression, not a bare
			// identifier the way r.Use(secureHeaders) is, since csrfProtect needs config.Config.
			inner, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			if ident, ok := inner.Fun.(*ast.Ident); ok && ident.Name == "csrfProtect" {
				found = true
			}
		}
		return true
	})

	if !found {
		t.Error("Routes() no longer calls r.Use(csrfProtect(...)) -- CSRF protection would stop applying to every route")
	}
}
