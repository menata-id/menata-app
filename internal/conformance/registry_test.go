package conformance

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/registry"
)

// TestServiceRegistryAndExecutorsAgree is what makes the Service seam *one* seam.
//
// 007 §14 sketches a single seam running `type → contract → validator → resolver → renderer`, and one Go
// package cannot hold all of it here -- `internal/metadata` must import the catalogue to validate and is
// forbidden from `internal/data`; `internal/execution` must import it to dispatch and is forbidden from
// `internal/metadata`; an executor needs the Store and the Mailer. So `internal/registry` owns the names
// and their load-time contracts, `internal/execution` owns the executors, and **this gate is the only
// thing binding the two**. Without it they are two lists, which is the shape the whole refactor exists to
// leave.
//
// What it replaced (2026-09-30): `domain.KnownServices`, a names-only `map[string]bool`, plus a four-case
// `switch e.Then.Name` in `internal/metadata/validate.go` and **three more** in `internal/execution` --
// each covering the subset its own dispatch path happened to need, with nothing checking that a new
// Service reached all four. A Service added to the change path and forgotten on the create path would
// simply never fire there, silently, and every test would stay green.
//
// Read by AST rather than by calling into `internal/execution`: the executor table is unexported, and it
// should stay that way -- exporting a dispatch table so a test can read it would widen a package's API for
// the convenience of its gate.
func TestServiceRegistryAndExecutorsAgree(t *testing.T) {
	executors := serviceExecutorKeys(t)
	if len(executors) == 0 {
		t.Fatal("found no entries in internal/execution's serviceExecutors table -- either it was renamed or this gate stopped looking; both are worth a failure")
	}

	for _, name := range sortedKeys(registry.Services) {
		if !executors[name] {
			t.Errorf("registry.Services declares %q but internal/execution's serviceExecutors has no entry for it -- a Service the loader accepts and nothing executes is a declaration that silently does nothing", name)
		}
	}
	for name := range executors {
		if _, ok := registry.Services[name]; !ok {
			t.Errorf("internal/execution's serviceExecutors has an entry for %q, which registry.Services does not declare -- an executor no metadata can name is reachable only from Go, which is 001 #3 inverted", name)
		}
	}
}

// serviceExecutorKeys parses the `serviceExecutors` composite literal and returns the Service constants it
// is keyed by, resolved through internal/domain's own `Service*` constants so the keys compared are values
// rather than identifier names.
func serviceExecutorKeys(t *testing.T) map[string]bool {
	t.Helper()
	byConstName := map[string]string{}
	for name, value := range stringConstantsIn(t, filepath.Join(repoRoot(), "internal", "domain")) {
		if strings.HasPrefix(name, "Service") {
			byConstName[name] = value
		}
	}
	if len(byConstName) == 0 {
		t.Fatal("internal/domain declares no Service* constants -- the resolution below would silently find nothing")
	}

	out := map[string]bool{}
	for _, file := range parsePackage(t, filepath.Join(repoRoot(), "internal", "execution")) {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			// A map literal whose keys are `domain.Service*` selectors. Matching on the key shape rather
			// than on the variable name keeps the gate working if the table is split in two.
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				sel, ok := kv.Key.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "domain" {
					continue
				}
				if value, known := byConstName[sel.Sel.Name]; known {
					out[value] = true
				}
			}
			return true
		})
	}
	return out
}

// TestRegistryDependsOnDomainOnly is the constraint that decided the seam's shape, enforced rather than
// remembered.
//
// `internal/registry` is importable by both the plane that validates (`internal/metadata`, forbidden from
// `internal/data`) and the plane that executes (`internal/execution`, forbidden from `internal/metadata`).
// That is only true while it depends on `internal/domain` alone. The moment it imports `data`, `mail` or
// `metadata` -- which is exactly what someone moving `Execute` into `registry.Service` would do -- the
// import graph breaks somewhere else, and an import cycle is a confusing way to learn a design rule.
//
// The boundary rule in boundary_test.go states it as forbidden packages; this states it positively, which
// is the stronger form for a package whose whole job is to be importable from two directions.
func TestRegistryDependsOnDomainOnly(t *testing.T) {
	allowed := map[string]bool{"menata.app/internal/domain": true}
	found := 0
	for _, file := range parsePackage(t, filepath.Join(repoRoot(), "internal", "registry")) {
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(path, "menata.app/") {
				continue // standard library is unconstrained
			}
			found++
			if !allowed[path] {
				t.Errorf("internal/registry imports %s -- it may import internal/domain only, because internal/metadata (forbidden from internal/data) and internal/execution (forbidden from internal/metadata) must both be able to import it. See registry.Service's own comment for why the executor cannot live here", path)
			}
		}
	}
	if found == 0 {
		t.Fatal("internal/registry imports nothing from this module -- either it is empty or this gate is looking in the wrong place")
	}
}

// TestDomainHoldsVocabularyAndRegistryHoldsDispatch is the line the Services and workflow-engine moves
// drew, enforced so the next capability lands on the right side of it without anyone remembering which.
//
// **The rule is mechanical, which is why it is gateable at all.** A closed set whose value is `bool` is
// *vocabulary* -- it answers "is this string legal", nothing more, and that is a declaration question, so
// it belongs in internal/domain. A closed set whose value carries **structure** (a validator func, a cast,
// a contract) is a *dispatch seam*: something reads it to decide what to run, which is
// 007 §14's subject and belongs in internal/registry.
//
// Measured when written (2026-09-30): eleven vocabulary maps in domain, all `→ bool`, and two structured
// ones in registry -- `Services` (`map[string]Service`, carrying Validate) and `KnownWorkflowEngines`
// (`map[string]WorkflowEngineSpec`, carrying the cast and the derivations each role owes). Both of the
// structured ones lived in `domain` until today, which is what made the *declaration* plane own a
// dispatch seam and left internal/action and internal/metadata reaching into domain for something
// neither declares.
//
// It does not say a vocabulary map may never move. It says a map that grows a func or a struct has become
// a seam, and the failure is the notice.
func TestDomainHoldsVocabularyAndRegistryHoldsDispatch(t *testing.T) {
	domainMaps := closedSetValueTypes(t, filepath.Join(repoRoot(), "internal", "domain"))
	if len(domainMaps) == 0 {
		t.Fatal("found no Known* maps in internal/domain -- this gate would pass by measuring nothing")
	}
	for _, name := range sortedKeys(domainMaps) {
		if domainMaps[name] != "bool" {
			t.Errorf("internal/domain declares %s with value type %q -- a closed set whose value carries structure is a dispatch seam (007 §14) and belongs in internal/registry, beside Services and KnownWorkflowEngines. A `bool` map is vocabulary and is right where it is",
				name, domainMaps[name])
		}
	}

	registryMaps := closedSetValueTypes(t, filepath.Join(repoRoot(), "internal", "registry"))
	if len(registryMaps) == 0 {
		t.Fatal("found no closed-set maps in internal/registry -- either both catalogues moved out or this gate is looking in the wrong place")
	}
	for _, name := range sortedKeys(registryMaps) {
		if registryMaps[name] == "bool" {
			t.Errorf("internal/registry declares %s as a `bool` map -- that is vocabulary, not dispatch, and internal/domain is where a closed set of legal strings belongs", name)
		}
	}
}

// closedSetValueTypes returns each package-level `map[...]T` variable whose name starts with a capital and
// looks like a closed set, mapped to T rendered as source. Keyed by variable name so the failure message
// can name it.
func closedSetValueTypes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, file := range parsePackage(t, dir) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.CompositeLit)
					if !ok {
						continue
					}
					mt, ok := lit.Type.(*ast.MapType)
					if !ok {
						continue
					}
					// Only the declared catalogues, not an incidental package-level map.
					if !strings.HasPrefix(name.Name, "Known") && name.Name != "Services" {
						continue
					}
					out[name.Name] = exprString(mt.Value)
				}
			}
		}
	}
	return out
}

// exprString renders a type expression the way the source spells it, which is all the message needs.
func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	default:
		return "struct/other"
	}
}
