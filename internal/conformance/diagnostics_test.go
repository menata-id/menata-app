package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// The per-request query diagnostic has two parts that can each be disconnected without breaking a
// single test or changing a single response, which is exactly how it came to under-report every
// authenticated request between Phase 18 Step 3 and 2026-09-22: it counted honestly, but it was
// wired below four middlewares that issue six queries between them, so what it counted was not
// what the request cost. Nobody noticed, because a diagnostic has no user to complain.
//
// Both tests below are static reads of the wiring, the same shape TestRoutesRegistersSecureHeaders
// and TestHandlersStaySmall already use, so they run in the fast pre-commit gate.

// TestQueryDiagnosticsRunsBeforeAuth fails if web.queryDiagnostics is not the first middleware in
// the authenticated group.
//
// Order is the whole correctness of this middleware: it installs the ReadLog on ctx, and a query
// issued before that lands on a nil log and is silently not counted. requireAuth alone issues two
// (the session generation and the user's Workspace), so registering the diagnostic after it is
// not a smaller measurement -- it is a measurement that omits the part nobody thinks to check.
func TestQueryDiagnosticsRunsBeforeAuth(t *testing.T) {
	uses := middlewareOrder(t)
	if len(uses) == 0 {
		t.Fatal("router.go registers no middleware in the authenticated group -- this check is no longer measuring anything")
	}
	if uses[0] != "queryDiagnostics" {
		t.Errorf("the authenticated group's first middleware is %q, not queryDiagnostics (order: %v)\n"+
			"  queryDiagnostics installs the ReadLog on ctx; every query issued by a middleware\n"+
			"  registered before it is counted by nothing. That is the 2026-09-22 regression --\n"+
			"  see internal/web/middleware.go's own doc comment -- not a stylistic preference",
			uses[0], uses)
	}
}

// middlewareOrder returns the middleware named in each pr.Use(...) call inside router.go's Routes,
// in source order. It reads the *inner* group (the one that calls requireAuth), since the outer
// router's own r.Use calls are pre-auth chrome (secureHeaders, csrfProtect) that issue no queries.
func middlewareOrder(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot(), "internal", "web", "router.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var order []string
	var found bool
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		fn, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		var names []string
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Use" || len(call.Args) != 1 {
				return true
			}
			names = append(names, middlewareName(call.Args[0]))
			return true
		})
		// The authenticated group is the one that installs requireAuth.
		for _, n := range names {
			if n == "requireAuth" {
				order, found = names, true
				return false
			}
		}
		return true
	})
	return order
}

// middlewareName reduces a Use argument to the function's own name, whether it is passed bare
// (queryDiagnostics) or built by a constructor call (requireAuth(...)).
func middlewareName(arg ast.Expr) string {
	switch a := arg.(type) {
	case *ast.Ident:
		return a.Name
	case *ast.CallExpr:
		if id, ok := a.Fun.(*ast.Ident); ok {
			return id.Name
		}
	}
	return "?"
}

// TestPoolInstallsQueryTracer fails if internal/db stops installing the tracer it is handed.
//
// The tracer is what makes the query count un-forgettable: it counts at the driver, so a Store
// method added later is counted whether or not its author knows the diagnostic exists. Dropping
// the assignment would leave every test passing and every log line printing a smaller number --
// the same silent failure this file exists for, one layer down.
func TestPoolInstallsQueryTracer(t *testing.T) {
	path := filepath.Join(repoRoot(), "internal", "db", "pool.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	assigned := false
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "Tracer" {
				assigned = true
			}
		}
		return true
	})
	if !assigned {
		t.Error("internal/db/pool.go never assigns ConnConfig.Tracer\n" +
			"  without it the pool issues untraced queries and web.queryDiagnostics' own\n" +
			"  queries= count -- the number the log's honesty rests on -- silently reads zero")
	}
}
