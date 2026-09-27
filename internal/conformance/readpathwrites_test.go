package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// storeWriteMethods are the Store calls that mutate. Reaching one of these from a GET route is
// what this file is about.
var storeWriteMethods = map[string]bool{
	"CreateRecord": true,
	"UpdateRecord": true,
	"DeleteRecord": true,
}

// readPathWriters are the functions allowed to write while serving a read, by name, with the
// reason each is allowed.
//
// **It is empty, and reaching empty is the point of this gate** -- the same shape
// getSweepRatchet's own doc comment (internal/web) already established for its ratchet: an empty
// map is an ordinary gate, not a retired one, so it stays declared. It held exactly one entry from
// when this test was added until 2026-09-27: the 2026-09-22 log review found the nav badge
// composing the whole Approval Inbox to print one integer, and ApprovalInbox wrote SLA-breach rows
// through logSLABreaches (internal/composition) as a documented, narrow exception to "reads don't
// write" -- this app had no scheduler to do it any other way. It now does
// (execution.RunScheduledEvents, on a ticker, cmd/server/main.go), so logSLABreaches was deleted
// rather than kept, and the exception went with it.
//
// Adding an entry here is allowed and is meant to be uncomfortable: per CLAUDE.md, an exception
// needs a comment naming the missing capability and a forward-checkable pointer. An entry that
// stops matching a real function fails this test rather than lingering.
var readPathWriters = map[string]string{}

// TestGetRoutesDoNotWrite fails when a handler registered for GET can reach a Store write.
//
// It walks the call graph across internal/web and internal/composition rather than checking one
// function body, because the write that first prompted this was four calls deep: showPendingCount
// -> ApprovalInbox -> logSLABreaches -> CreateRecord (since retired, 2026-09-27 -- see
// readPathWriters' own doc comment). A one-level check would have seen a handler that only
// composes and called it clean.
func TestGetRoutesDoNotWrite(t *testing.T) {
	funcs := map[string]*ast.FuncDecl{}
	for _, pkg := range []string{"web", "composition"} {
		collectFuncs(t, filepath.Join(repoRoot(), "internal", pkg), funcs)
	}
	if len(funcs) == 0 {
		t.Fatal("no functions parsed -- this check is no longer measuring anything")
	}
	for name := range readPathWriters {
		if _, ok := funcs[name]; !ok {
			t.Errorf("readPathWriters names %q, which no longer exists in internal/web or internal/composition\n"+
				"  a stale exception is an exception nobody is checking -- remove it", name)
		}
	}

	writers := writingFuncs(funcs)
	handlers := getRouteHandlers(t)
	if len(handlers) == 0 {
		t.Fatal("router.go registers no GET routes -- this check is no longer measuring anything")
	}

	var offenders []string
	for _, h := range handlers {
		if path := writePath(h, funcs, writers, map[string]bool{}); path != nil {
			offenders = append(offenders, strings.Join(path, " -> "))
		}
	}
	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("a GET route reaches a Store write: %s\n"+
			"  serving a read must not mutate. If this is deliberate, the writing function goes in\n"+
			"  readPathWriters with its reason and a forward-checkable pointer (CLAUDE.md), rather\n"+
			"  than being left for a log review to find by accident", o)
	}
}

// collectFuncs indexes every function and method in a package directory by name. Methods are
// indexed by their bare name, which is imprecise and sufficient: this repo has no two functions
// across these two packages whose names collide, and the failure mode of a collision is a false
// positive that someone must then look at -- the safe direction for a gate like this.
func collectFuncs(t *testing.T, dir string, out map[string]*ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
					out[fn.Name.Name] = fn
				}
			}
		}
	}
}

// writingFuncs returns every function that writes, directly or through another that does, minus
// the declared read-path exceptions. Fixpoint rather than one pass: a writer three calls up is
// still a writer.
func writingFuncs(funcs map[string]*ast.FuncDecl) map[string]bool {
	writers := map[string]bool{}
	for name, fn := range funcs {
		if readPathWriters[name] != "" {
			continue
		}
		if writeMethodIn(fn) != "" {
			writers[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for name, fn := range funcs {
			if writers[name] || readPathWriters[name] != "" {
				continue
			}
			for _, callee := range calledNames(fn) {
				if writers[callee] {
					writers[name] = true
					changed = true
					break
				}
			}
		}
	}
	return writers
}

// writePath returns the chain from name down to the write, for an error message that says how the
// route gets there rather than only that it does.
func writePath(name string, funcs map[string]*ast.FuncDecl, writers map[string]bool, seen map[string]bool) []string {
	if seen[name] || readPathWriters[name] != "" {
		return nil
	}
	seen[name] = true
	fn, ok := funcs[name]
	if !ok {
		return nil
	}
	if writeMethodIn(fn) != "" {
		return []string{name, writeMethodIn(fn)}
	}
	for _, callee := range calledNames(fn) {
		if !writers[callee] {
			continue
		}
		if rest := writePath(callee, funcs, writers, seen); rest != nil {
			return append([]string{name}, rest...)
		}
	}
	return nil
}

// getRouteHandlers returns the handler constructor named by each GET registration in router.go.
func getRouteHandlers(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot(), "internal", "web", "router.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Get" && sel.Sel.Name != "Head") || len(call.Args) != 2 {
			return true
		}
		if name := handlerName(call.Args[1]); name != "" {
			names = append(names, name)
		}
		return true
	})
	return names
}

// handlerName reduces a route's handler argument to a function name, whether it is a constructor
// call (showApprovalInbox(...)), a bare identifier, or a wrapper whose own argument is the real
// handler (rateLimitLogin(limiter, submitLogin(...))).
func handlerName(arg ast.Expr) string {
	switch a := arg.(type) {
	case *ast.Ident:
		return a.Name
	case *ast.CallExpr:
		if id, ok := a.Fun.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// writeMethodIn returns the first Store write method fn calls directly, or "".
func writeMethodIn(fn *ast.FuncDecl) string {
	found := ""
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && storeWriteMethods[sel.Sel.Name] {
			found = sel.Sel.Name
			return false
		}
		return true
	})
	return found
}

// calledNames returns the names of functions fn calls, both bare (helper(...)) and qualified
// (composition.ApprovalInbox(...)) -- the latter reduced to the bare name, matching how
// collectFuncs indexes them.
func calledNames(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.Ident:
			out = append(out, f.Name)
		case *ast.SelectorExpr:
			out = append(out, f.Sel.Name)
		}
		return true
	})
	return out
}
