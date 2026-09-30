package conformance

import (
	"go/ast"
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
