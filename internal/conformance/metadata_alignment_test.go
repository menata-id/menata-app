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
