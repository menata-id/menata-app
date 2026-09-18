package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maxHandlerLines bounds one HTTP handler's body in internal/web.
//
// The number is measured, not chosen: after ROADMAP.md Phase 19 Step 3 the largest handler is
// decideStep at 55 lines, and 70 leaves room for an honest handler to grow without leaving room
// for a screen's worth of derivation to move back in. Raising it is a decision someone should
// have to make deliberately, in a commit that says why.
//
// The budget is per handler rather than per file on purpose. A file-length limit is satisfied by
// splitting handlers_a.go from handlers_b.go, which fixes nothing: what went wrong in
// cmd/server/main.go was that joins, rollups and SLA bucketing accumulated inside handler bodies
// where no test could reach them, and that is what this measures.
const maxHandlerLines = 70

// TestHandlersStaySmall fails when a handler in internal/web outgrows the budget.
//
// A handler that has genuinely earned more lines is usually a handler holding work that belongs
// in another plane: composing a screen's content is internal/composition's job, deciding a
// business rule is internal/behavior's or internal/action's. Moving it there is what makes it
// testable, which is the point of the budget rather than a side effect.
func TestHandlersStaySmall(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "web")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read internal/web: %v", err)
	}

	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			body, isHandler := handlerBody(fn)
			if !isHandler {
				continue
			}
			checked++
			lines := fset.Position(body.Rbrace).Line - fset.Position(body.Lbrace).Line - 1
			if lines > maxHandlerLines {
				t.Errorf("%s: handler %s is %d lines, over the %d-line budget\n"+
					"  a handler this size is usually holding work that belongs in another plane --\n"+
					"  move the derivation into internal/composition (where it can be unit-tested)\n"+
					"  rather than raising the budget",
					e.Name(), fn.Name.Name, lines, maxHandlerLines)
			}
		}
	}

	// A refactor that renamed the handler shape out from under this check would leave it passing
	// while measuring nothing at all.
	if checked == 0 {
		t.Fatal("no handlers found in internal/web -- this check is no longer measuring anything")
	}
}

// handlerBody returns the body this budget applies to: the returned closure for a
// func(...) http.HandlerFunc constructor, or the function itself when it is a plain
// (http.ResponseWriter, *http.Request) handler. The closure is what matters for a constructor --
// the wrapper around it is a signature, not logic.
func handlerBody(fn *ast.FuncDecl) (*ast.BlockStmt, bool) {
	if isHandlerSignature(fn.Type) {
		return fn.Body, true
	}
	if !returnsHandlerFunc(fn.Type) {
		return nil, false
	}
	// Exactly one statement, `return func(w, req) {...}`, is the shape every constructor here
	// uses; anything else is not a plain constructor and is left to the reader.
	if len(fn.Body.List) != 1 {
		return fn.Body, true
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return fn.Body, true
	}
	lit, ok := ret.Results[0].(*ast.FuncLit)
	if !ok {
		return fn.Body, true
	}
	return lit.Body, true
}

func returnsHandlerFunc(t *ast.FuncType) bool {
	if t.Results == nil || len(t.Results.List) != 1 {
		return false
	}
	switch r := t.Results.List[0].Type.(type) {
	case *ast.SelectorExpr: // http.HandlerFunc
		return r.Sel.Name == "HandlerFunc"
	case *ast.FuncType: // func(http.ResponseWriter, *http.Request)
		return isHandlerSignature(r)
	}
	return false
}

// isHandlerSignature reports whether t takes (http.ResponseWriter, *http.Request) and returns
// nothing.
func isHandlerSignature(t *ast.FuncType) bool {
	if t.Results != nil && len(t.Results.List) > 0 {
		return false
	}
	if t.Params == nil || len(t.Params.List) != 2 {
		return false
	}
	w, ok := t.Params.List[0].Type.(*ast.SelectorExpr)
	if !ok || w.Sel.Name != "ResponseWriter" {
		return false
	}
	req, ok := t.Params.List[1].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := req.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Request"
}
