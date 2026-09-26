package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
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

// knownPublicRoutes is every route Routes() registers directly on r (the root router, outside
// r.Group's requireAuth gate) today -- deliberately public, not an oversight. A route registered
// on r that isn't in this list is exactly the mistake TestUngatedRoutesAreOnlyTheKnownPublicSet
// exists to catch: a route meant to sit behind requireAuth, accidentally added to r instead of pr.
var knownPublicRoutes = map[string]bool{
	"/health":              true,
	"/manifest.json":       true,
	"/sw.js":               true,
	"/icons/*":             true,
	"/css/*":               true,
	"/vendor/*":            true,
	"/login":               true,
	"/register":            true,
	"/verify-email":        true,
	"/resend-verification": true,
	"/forgot-password":     true,
	"/reset-password":      true,
	"/accept-invite":       true,
	"/choose-workspace":    true,
	// Restore's pre-session entry point (Flow 2 gap study Tahap 7) -- reached the same way
	// /choose-workspace itself is, before any Workspace-scoped session exists: the pending-email
	// cookie names who is acting, checked inside the handler itself
	// (restoreWorkspaceIfAdmin), not by requireAuth.
	"/choose-workspace/restore": true,
}

// routeVerbs is every chi.Router method that registers a route (as opposed to r.Group, r.Use,
// etc., which don't take a route path as their first argument).
var routeVerbs = map[string]bool{
	"Get": true, "Post": true, "Put": true, "Delete": true, "Patch": true, "Handle": true,
}

// TestUngatedRoutesAreOnlyTheKnownPublicSet is the security-audit-2026-09-19 methodology
// recommendation (guides/primitive-security-audit.md, menata-app-document): every route inside
// r.Group(func(pr chi.Router) {...}) is automatically gated by requireAuth (pr.Use(requireAuth...)
// applies to the whole sub-router, structurally impossible to bypass from within), so the real
// risk this app's own route registrations face isn't "a route inside pr forgets requireAuth" --
// it's a route meant to be authenticated getting registered directly on r by mistake, silently
// skipping pr entirely. ast.Inspect walks the whole Routes() body including inside r.Group's
// closure, but a call there has a selector on pr/ar (not r), so it never matches the `r.` check
// below regardless of nesting depth -- no manual scope-tracking needed.
func TestUngatedRoutesAreOnlyTheKnownPublicSet(t *testing.T) {
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

	ast.Inspect(routesFunc.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !routeVerbs[sel.Sel.Name] {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok || recv.Name != "r" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		routePath, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if !knownPublicRoutes[routePath] {
			t.Errorf("%s registers %q directly on r (outside r.Group's requireAuth gate) -- add it to knownPublicRoutes if this is deliberate, or move it inside r.Group(func(pr chi.Router) {...}) if it needs auth", fset.Position(call.Pos()), routePath)
		}
		return true
	})
}
