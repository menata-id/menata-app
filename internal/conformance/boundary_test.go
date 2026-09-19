package conformance

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const module = "menata.app"

// Physical/transport/presentation dependencies, named once so each rule below reads as intent
// rather than as a list of import paths.
const (
	postgres = "github.com/jackc/pgx"
	templ    = "github.com/a-h/templ"
	httpPkg  = "net/http"
	sqlPkg   = "database/sql"
)

func internalPkg(name string) string { return module + "/internal/" + name }

// rule states one package's import obligation. forbidden holds import-path prefixes the package
// must not depend on; because cites the clause the rule enforces, so a failure explains itself
// without a trip to the concept docs.
//
// dir is the package's path relative to the repo root, needed only when the package does not live
// under internal/ -- the composition root in cmd/ is the case that forced the field (ROADMAP.md
// Phase 19 Step 1). Leaving it empty means internal/<pkg>, which is every other rule here.
type rule struct {
	pkg       string
	dir       string
	forbidden []string
	because   string
}

func (r rule) path() string {
	if r.dir != "" {
		return r.dir
	}
	return "internal/" + r.pkg
}

// rules covers every package under internal/ and every binary under cmd/. A package with no entry
// fails TestEveryPackageHasARule below -- adding a package therefore forces a decision about what
// it may depend on, which is the point: an unnamed boundary is how drift starts.
var rules = []rule{
	{
		pkg:       "domain",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, internalPkg("data"), internalPkg("db"), internalPkg("rendering"), internalPkg("storage"), internalPkg("execution")},
		because:   "004 §Domain Plane: the Domain Plane declares business meaning; physical storage, transport and presentation are other planes' concerns",
	},
	{
		pkg:       "expression",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, module + "/internal"},
		because:   "internal/expression is a pure evaluator -- no I/O, and no dependency on any other plane",
	},
	{
		pkg:       "metadata",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, internalPkg("data"), internalPkg("db"), internalPkg("rendering")},
		because:   "004: Runtime Metadata normalizes into Domain concepts; it must not reach physical storage or presentation",
	},
	{
		pkg:       "db",
		forbidden: []string{templ, httpPkg, module + "/internal"},
		because:   "internal/db doc.go: owns the connection pool and has no knowledge of Runtime Metadata semantics",
	},
	{
		pkg:       "data",
		forbidden: []string{httpPkg, templ, internalPkg("rendering"), internalPkg("metadata"), internalPkg("authorization")},
		because:   "007 §20: the Data Plane consumes permission decisions and renders nothing; it must not own transport or presentation",
	},
	{
		pkg:       "behavior",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, internalPkg("db"), internalPkg("rendering")},
		because:   "006 §Behavioral Model: Behavior evaluates declared rules over already-fetched records -- it neither queries nor renders",
	},
	{
		pkg:       "action",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, internalPkg("db"), internalPkg("rendering")},
		because:   "006 §Behavioral Model: an Action decides and mutates Field values; it must not reach the driver or the renderer",
	},
	{
		pkg:       "authorization",
		forbidden: []string{postgres, sqlPkg, templ, internalPkg("db"), internalPkg("rendering"), internalPkg("data")},
		because:   "007 §20: permissions are owned by the authorization architecture and evaluated over already-fetched records (Phase 16's AllowsAction is a pure function)",
	},
	{
		pkg:       "experience",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, internalPkg("db"), internalPkg("rendering")},
		because:   "006 §View: the Experience Plane's logic (grouping, SLA) is pure -- I/O belongs to its callers, templ to internal/rendering",
	},
	{
		pkg:       "rendering",
		forbidden: []string{postgres, sqlPkg, internalPkg("db"), internalPkg("metadata")},
		because:   "007 §20 names the anti-pattern directly: no plane may query all data then trim at render time -- the renderer must not reach the database at all",
	},
	{
		pkg:       "composition",
		forbidden: []string{postgres, sqlPkg, httpPkg, internalPkg("db")},
		because:   "007 §5, §18.2: composition builds the Experience tree and derives the dependency graph; physical execution belongs to internal/data and internal/execution",
	},
	{
		pkg:       "ir",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, internalPkg("db"), internalPkg("data"), internalPkg("rendering")},
		because:   "internal/ir doc.go: IR must not embed HTML, CSS classes, SQL or other physical implementation",
	},
	{
		pkg:       "planner",
		forbidden: []string{httpPkg, templ, internalPkg("rendering")},
		because:   "007 §18: the CEP plans; it does not render, and it does not own transport",
	},
	{
		pkg:       "execution",
		forbidden: []string{httpPkg, templ, internalPkg("rendering"), internalPkg("metadata")},
		because:   "007 §17: physical execution plans are runtime-internal and must never become portable Runtime Metadata or presentation",
	},
	{
		pkg:       "registry",
		forbidden: []string{postgres, sqlPkg, httpPkg},
		because:   "007 §14: a static compile-time dispatch seam, not a loader -- it needs neither a database nor a network",
	},
	{
		pkg:       "storage",
		forbidden: []string{postgres, sqlPkg, templ, internalPkg("rendering"), internalPkg("data")},
		because:   "007 §4.10: file storage is local-disk blob I/O, independent of the record store and the renderer",
	},
	{
		pkg:       "pdf",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, module + "/internal"},
		because:   "internal/pdf is a pure document operation over bytes -- it knows nothing of Machines, records or pages",
	},
	{
		pkg:       "config",
		forbidden: []string{postgres, sqlPkg, templ, module + "/internal"},
		because:   "internal/config doc.go: carries no business or Runtime Metadata concerns",
	},
	{
		pkg:       "mail",
		forbidden: []string{postgres, sqlPkg, templ, internalPkg("data"), internalPkg("rendering")},
		because:   "internal/mail doc.go: sends bytes over SMTP; it has no business touching the database or the renderer",
	},
	{
		pkg:       "conformance",
		forbidden: []string{postgres, sqlPkg, httpPkg, templ, module + "/internal"},
		because:   "these checks must not depend on the code they police",
	},
	{
		pkg:       "web",
		forbidden: []string{postgres, sqlPkg, internalPkg("db"), internalPkg("metadata")},
		because:   "internal/web doc.go: the transport layer adapts HTTP to the planes; it holds no pool and loads no Runtime Metadata -- the composition root builds both once at startup (005 Phase 3-4)",
	},
	{
		pkg: "server",
		dir: "cmd/server",
		// internal/db is deliberately absent: the composition root is the one place that must
		// build the pool, so db.Connect is its job. What it may not do is speak the driver's own
		// language, or render -- both became enforceable once Phase 19 Step 2 moved the handlers
		// into internal/web.
		forbidden: []string{postgres, sqlPkg, templ, internalPkg("rendering"), internalPkg("composition")},
		because:   "002 §Runtime Boundary: the composition root wires dependencies and mounts the router; it holds the pool through internal/db but never speaks SQL, renders, or composes",
	},
}

func TestPlaneBoundaries(t *testing.T) {
	for _, r := range rules {
		t.Run(r.pkg, func(t *testing.T) {
			imports, err := packageImports(r.path())
			if err != nil {
				t.Fatalf("read imports: %v", err)
			}
			for _, imported := range imports {
				for _, bad := range r.forbidden {
					if strings.HasPrefix(imported, bad) {
						t.Errorf("%s must not import %q\n  rule: %s", r.path(), imported, r.because)
					}
				}
			}
		})
	}
}

// TestEveryPackageHasARule is the anti-drift half: a new package has no declared boundary until
// someone writes one, and an undeclared boundary is exactly what nobody notices until it is too
// large to correct.
//
// cmd/ is scanned alongside internal/ because until Phase 19 it was not: cmd/server was the
// largest file in the repo and the only one no rule covered, which is precisely backwards.
func TestEveryPackageHasARule(t *testing.T) {
	covered := make(map[string]bool, len(rules))
	for _, r := range rules {
		covered[r.path()] = true
	}

	for _, parent := range []string{"internal", "cmd"} {
		entries, err := os.ReadDir(filepath.Join(repoRoot(), parent))
		if err != nil {
			t.Fatalf("read %s/: %v", parent, err)
		}
		for _, e := range entries {
			path := parent + "/" + e.Name()
			if !e.IsDir() || covered[path] {
				continue
			}
			t.Errorf("%s has no boundary rule -- add one to rules in this file, stating what it may not depend on and why", path)
		}
	}
}

func repoRoot() string { return filepath.Join("..", "..") }

// packageImports returns the package's non-test imports. Test files are excluded deliberately: a
// test may reach for whatever it needs to exercise the package, and constraining that would
// discourage testing without protecting the architecture.
//
// Every .go file in the directory must be accounted for. go/build skips a file it cannot parse,
// which would let an unparseable file hide its own imports and turn this whole check into a
// silent pass -- the exact failure this package exists to prevent, so it is an error here rather
// than a shrug.
// path is the package directory relative to the repo root, e.g. "internal/domain" or "cmd/server".
func packageImports(path string) ([]string, error) {
	dir := filepath.Join(repoRoot(), path)
	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(pkg.GoFiles)+len(pkg.TestGoFiles)+len(pkg.IgnoredGoFiles))
	for _, group := range [][]string{pkg.GoFiles, pkg.TestGoFiles, pkg.XTestGoFiles, pkg.IgnoredGoFiles, pkg.InvalidGoFiles} {
		for _, f := range group {
			seen[f] = true
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if !seen[e.Name()] {
			return nil, fmt.Errorf("%s/%s was not parsed, so its imports were never checked", path, e.Name())
		}
	}
	if len(pkg.InvalidGoFiles) > 0 {
		return nil, fmt.Errorf("%s has unparseable files: %v", path, pkg.InvalidGoFiles)
	}
	return pkg.Imports, nil
}
