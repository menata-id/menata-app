package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"menata.app/internal/metadata"
)

// Package metadata_alignment_test.go holds executable checks for CLAUDE.md's "What this project
// is": this is a composable runtime, not an ordinary application, so evolution is supposed to
// happen by editing metadata/*.yaml rather than by hand-writing something metadata could already
// express (001 Principle #3 Metadata First, #8 Reference over Duplication). Prose says that;
// these tests are what actually holds it, the same posture boundary_test.go and
// handlersize_test.go already take for the plane-boundary and handler-size obligations.

// TestAppManifestLoads is a fast, no-database sanity check that metadata/app.yaml -- the real
// manifest this runtime ships, not a synthetic fixture -- still parses and validates. A metadata
// typo otherwise surfaces only when someone starts the server; this catches it at `go test` time,
// cheap enough to run on every commit.
func TestAppManifestLoads(t *testing.T) {
	path := filepath.Join(repoRoot(), "metadata", "app.yaml")
	if _, err := metadata.LoadApplication(path); err != nil {
		t.Fatalf("LoadApplication(%s) = %v, want a valid manifest", path, err)
	}
}

// declaredNavRoutes reads every route: straight out of metadata/app.yaml's own navigation list --
// including items hidden_nav_groups removes from the runtime Navigation slice, since app.yaml's
// own comment on hidden_nav_groups documents those routes as staying valid destinations, just not
// topbar entries. A minimal struct on purpose: this only needs route, not the full navItemDoc
// shape internal/metadata already owns privately.
func declaredNavRoutes(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot(), "metadata", "app.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		Application struct {
			Navigation []struct {
				Route string `yaml:"route"`
			} `yaml:"navigation"`
		} `yaml:"application"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	routes := make([]string, 0, len(doc.Application.Navigation))
	for _, n := range doc.Application.Navigation {
		routes = append(routes, n.Route)
	}
	return routes
}

// chiGetCall matches one chi `.Get("/route", ...)` registration in router.go's source text.
// Navigation items are always link destinations (a GET), so Get is the only verb this checks.
var chiGetCall = regexp.MustCompile(`\.Get\(\s*"([^"]+)"`)

// TestNavigationRoutesAreRegistered closes the gap domain.NavigationItem's own doc comment names:
// "Route existence is therefore not cross-checked against anything at load time... routes are a
// runtime/internal/web concern, not a metadata one." metadata/app.yaml can declare a route no
// handler ever serves, and nothing fails until someone actually clicks it -- this makes that fail
// at commit time instead. An exact-string match on purpose (not a chi-pattern match against
// path-parameter segments like {id}): no navigation route declared today has one, and a future
// one gaining a parameter is a decision this test should force someone to look at, not silently
// paper over.
func TestNavigationRoutesAreRegistered(t *testing.T) {
	routerSrc, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "web", "router.go"))
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	registered := make(map[string]bool)
	for _, m := range chiGetCall.FindAllStringSubmatch(string(routerSrc), -1) {
		registered[m[1]] = true
	}

	for _, route := range declaredNavRoutes(t) {
		if !registered[route] {
			t.Errorf("metadata/app.yaml declares navigation route %q, but internal/web/router.go registers no GET handler for it", route)
		}
	}
}

// workspaceLevelHrefs are the only literal href="/..." values a Workspace-level page (see
// workspaceLevelTemplFiles) may contain: Workspace/runtime-level destinations that exist
// regardless of which Application is configured, never one Application's own business route. Any
// other literal href is exactly the class of drift this session found and fixed
// (workspacehome.templ hand-typing /approval-inbox and /documents/new, both already declared in
// metadata/app.yaml's navigation) -- see domain.Application.HomeRoute / internal/web/
// workspacehome.go for the metadata-driven replacement every Application-specific link on such a
// page must use instead.
var workspaceLevelHrefs = map[string]bool{
	"/home":              true,
	"/workspace-members": true,
	"/switch-workspace":  true,
	"/login":             true,
}

var templHref = regexp.MustCompile(`href="(/[^"{]*)"`)

// workspaceHomeShellCall matches a .templ page composing workspaceHomeShell (internal/rendering/
// machine.templ) -- the Workspace-level, Application-agnostic chrome, as opposed to pageShell
// (every Application-level screen's own topbar, which legitimately links between that same
// Application's own sibling routes -- approvalinbox.templ's "+ New Approval" linking to
// /documents/new is normal same-Application navigation, not the drift this test looks for).
var workspaceHomeShellCall = regexp.MustCompile(`@workspaceHomeShell\(`)

// workspaceLevelTemplFiles finds every Workspace-level page by what it composes, not by name --
// today that's only workspacehome.templ, but a second page built the same way (e.g. a future
// Workspace Settings screen) is picked up automatically, no test edit required. Generalizing this
// check to *every* .templ file instead (not just Workspace-level ones) was considered and
// rejected: an Application-level page's own internal links are that Application's real
// implementation, and flagging them would be exactly the false-positive failure mode
// TestWritingGuideMachinesMatchMetadata was dropped for (see this file's git history) -- broader
// isn't better if it stops being precise.
func workspaceLevelTemplFiles(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if workspaceHomeShellCall.Match(src) {
			files = append(files, path)
		}
	}
	if len(files) == 0 {
		t.Fatal("no .templ file composes workspaceHomeShell -- expected at least workspacehome.templ; did it get renamed or restructured?")
	}
	return files
}

// TestWorkspaceLevelPagesHaveNoHardcodedApplicationRoute is the gate: every Workspace-level page
// (workspaceLevelTemplFiles) is Application-agnostic chrome, so any route it links to that isn't
// Workspace/runtime-level must come from a Go value (metadata, ultimately), never a literal
// string in the .templ itself -- that's the only way the route can follow metadata/app.yaml's own
// navigation instead of silently going stale next to it.
func TestWorkspaceLevelPagesHaveNoHardcodedApplicationRoute(t *testing.T) {
	for _, path := range workspaceLevelTemplFiles(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range templHref.FindAllStringSubmatch(string(src), -1) {
			href := m[1]
			if !workspaceLevelHrefs[href] {
				t.Errorf("%s hardcodes href=%q -- Application routes must come from a Go value (e.g. domain.Application.HomeRoute), not a literal here", path, href)
			}
		}
	}
}

// templFuncDecl matches a top-level `templ SomeName(` declaration -- the exported rendering
// function a Go handler calls to actually render that page.
var templFuncDecl = regexp.MustCompile(`(?m)^templ ([A-Z]\w*)\(`)

// workspaceLevelPageFuncs is the Go-callable identity of every Workspace-level page
// (workspaceLevelTemplFiles), e.g. "WorkspaceHomePage" -- what a handler in internal/web actually
// calls as rendering.WorkspaceHomePage(...).
func workspaceLevelPageFuncs(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, path := range workspaceLevelTemplFiles(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range templFuncDecl.FindAllStringSubmatch(string(src), -1) {
			names = append(names, m[1])
		}
	}
	return names
}

// workspaceLevelHandlerFiles is the Go-side counterpart to workspaceLevelTemplFiles: every
// internal/web/*.go file that calls one of workspaceLevelPageFuncs, found by what it calls rather
// than by name -- so the "no hardcoded Application route" obligation is checked at both ends of
// the chain CLAUDE.md's "Where a metadata-derived value belongs" describes. A handler could
// otherwise satisfy TestWorkspaceLevelPagesHaveNoHardcodedApplicationRoute by hardcoding the route
// itself and merely passing a clean-looking variable into the page -- pushing the same violation
// down one layer instead of removing it, which is exactly what this closes.
func workspaceLevelHandlerFiles(t *testing.T) []string {
	t.Helper()
	funcs := workspaceLevelPageFuncs(t)
	dir := filepath.Join(repoRoot(), "internal", "web")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, fn := range funcs {
			if strings.Contains(string(src), "rendering."+fn+"(") {
				files = append(files, path)
				break
			}
		}
	}
	return files
}

// TestWorkspaceLevelHandlersHaveNoHardcodedApplicationRoute is
// TestWorkspaceLevelPagesHaveNoHardcodedApplicationRoute's Go-side counterpart: no string literal
// in a Workspace-level handler (workspaceLevelHandlerFiles) may equal a route metadata/app.yaml
// already declares, except "/home" -- the one documented, deliberate fallback for "no home_card
// item declared" (Principle #5 Convention over Configuration: a safe default when config is
// absent is not the same thing as a hardcoded Application assumption). Parsed with go/ast, not a
// text regexp, so a route mentioned in a comment doesn't false-positive -- the same rigor
// boundary_test.go/handlersize_test.go already use for their own Go-source checks.
func TestWorkspaceLevelHandlersHaveNoHardcodedApplicationRoute(t *testing.T) {
	navRoutes := make(map[string]bool)
	for _, route := range declaredNavRoutes(t) {
		if route == "/home" {
			continue
		}
		navRoutes[route] = true
	}

	fset := token.NewFileSet()
	for _, path := range workspaceLevelHandlerFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if navRoutes[value] {
				t.Errorf("%s hardcodes %q, a route metadata/app.yaml already declares -- it must come from a domain.Application field (e.g. HomeRoute) threaded through web.Deps, not be retyped here", fset.Position(lit.Pos()), value)
			}
			return true
		})
	}
}

// markdownSection returns capabilities.md's lines between a heading exactly matching heading and
// the next line that starts a new section ("---" or another "#"-prefixed heading) -- scoping a
// table-row regexp to the one table it's meant to check, not every backtick-first-column table in
// the document (the Field Types table's `text`, `number`, ... would otherwise collide with the
// Machines table's `mch_*` ids).
func markdownSection(t *testing.T, doc []byte, heading string) []string {
	t.Helper()
	lines := strings.Split(string(doc), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		t.Fatalf("capabilities.md: no section heading %q found -- renamed?", heading)
	}
	var section []string
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || strings.HasPrefix(trimmed, "#") {
			break
		}
		section = append(section, line)
	}
	return section
}

var markdownFirstColumnCode = regexp.MustCompile("^\\| `([a-zA-Z0-9_]+)`")

// markdownFirstColumnCodes returns the backtick-quoted code span opening each row of a markdown
// table -- capabilities.md's own convention for the identifier a row documents (a Machine id, a
// component name, a field type).
func markdownFirstColumnCodes(section []string) []string {
	var codes []string
	for _, line := range section {
		if m := markdownFirstColumnCode.FindStringSubmatch(line); m != nil {
			codes = append(codes, m[1])
		}
	}
	return codes
}

// TestCapabilitiesMachinesTableMatchesMetadata keeps capabilities.md's own "Machines currently
// defined" table -- README.md's stated inventory of "what exists, right now, in technical detail"
// -- honest against metadata/*.yaml: every declared Machine id must be documented, and every
// documented id must still be a real Machine. This is the same "metadata drifting from the doc
// that describes it" gap TestWritingGuideMachinesMatchMetadata tried to close and was dropped for
// false-positiving on (see this file's git history) -- capabilities.md's table is structured, one
// row per Machine with its id as the first column's code span, not prose with incidental
// mentions, so the same idea is precise here where it wasn't there.
func TestCapabilitiesMachinesTableMatchesMetadata(t *testing.T) {
	metaDir := filepath.Join(repoRoot(), "metadata")
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		t.Fatalf("read metadata dir: %v", err)
	}
	declared := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || e.Name() == "app.yaml" || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		m, err := metadata.Load(filepath.Join(metaDir, e.Name()))
		if err != nil {
			t.Fatalf("load %s: %v", e.Name(), err)
		}
		declared[m.ID] = true
	}

	doc, err := os.ReadFile(filepath.Join(repoRoot(), "capabilities.md"))
	if err != nil {
		t.Fatalf("read capabilities.md: %v", err)
	}
	documented := make(map[string]bool)
	for _, id := range markdownFirstColumnCodes(markdownSection(t, doc, "## Machines currently defined")) {
		if strings.HasPrefix(id, "mch_") {
			documented[id] = true
		}
	}

	for id := range declared {
		if !documented[id] {
			t.Errorf("metadata declares machine %q, but capabilities.md's \"Machines currently defined\" table doesn't list it", id)
		}
	}
	for id := range documented {
		if !declared[id] {
			t.Errorf("capabilities.md's \"Machines currently defined\" table lists %q, but no metadata/*.yaml declares it -- renamed or removed?", id)
		}
	}
}

// templDeclAnyCase matches a top-level `templ name(` declaration regardless of exported/
// unexported case -- capabilities.md's "Shared rendering components" table documents both
// (pageShell is exported by convention only because it's called from this same package; the
// component name itself carries no export contract).
var templDeclAnyCase = regexp.MustCompile(`(?m)^templ ([a-zA-Z]\w*)\(`)

func templDeclaredFuncs(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	funcs := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range templDeclAnyCase.FindAllStringSubmatch(string(src), -1) {
			funcs[m[1]] = true
		}
	}
	return funcs
}

// TestCapabilitiesComponentsTableMatchesTempl keeps capabilities.md's "Shared rendering
// components" table -- its own stated purpose: "'does a component for this already exist?' was
// previously answerable only by grep" -- honest against the real internal/rendering/*.templ
// declarations: every documented component name must be a real `templ` function. Checked one
// direction only (documented -> exists): a real templ function with no doc row is a documentation
// gap worth someone's attention, but not the drift a stale/renamed entry is -- machine.templ
// alone declares many templ functions never meant to be catalogued here (navLink, page bodies),
// so the reverse direction would force every future helper to also get a doc row or fail the
// build, which is a documentation-completeness policy nobody asked for, not a composability gate.
func TestCapabilitiesComponentsTableMatchesTempl(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(repoRoot(), "capabilities.md"))
	if err != nil {
		t.Fatalf("read capabilities.md: %v", err)
	}
	funcs := templDeclaredFuncs(t)
	for _, name := range markdownFirstColumnCodes(markdownSection(t, doc, "### Shared rendering components")) {
		if !funcs[name] {
			t.Errorf("capabilities.md's \"Shared rendering components\" table lists %q, but no internal/rendering/*.templ declares `templ %s(...)` -- renamed or removed?", name, name)
		}
	}
}
