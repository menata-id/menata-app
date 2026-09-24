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

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// Package metadata_alignment_test.go holds executable checks for CLAUDE.md's "What this project
// is": this is a composable runtime, not an ordinary application, so evolution is supposed to
// happen by editing metadata/*.yaml rather than by hand-writing something metadata could already
// express (001 Principle #3 Metadata First, #8 Reference over Duplication). Prose says that;
// these tests are what actually holds it, the same posture boundary_test.go and
// handlersize_test.go already take for the plane-boundary and handler-size obligations.

// TestAppManifestLoads is a fast, no-database sanity check that metadata/workspaces/default.yaml -- the real
// manifest this runtime ships, not a synthetic fixture -- still parses and validates. A metadata
// typo otherwise surfaces only when someone starts the server; this catches it at `go test` time,
// cheap enough to run on every commit.
func TestAppManifestLoads(t *testing.T) {
	path := filepath.Join(repoRoot(), "metadata", "workspaces", "default.yaml")
	if _, err := metadata.LoadApplication(path); err != nil {
		t.Fatalf("LoadApplication(%s) = %v, want a valid manifest", path, err)
	}
}

// declaredNavItems is every navigation item declared anywhere in the real manifest: the
// Workspace's own list plus each Application's, unfiltered.
//
// It goes through metadata.LoadApplication rather than re-parsing the YAML with a local struct,
// which is the lesson of how this broke. It used to unmarshal `application.navigation` straight
// out of app.yaml; when Fase 3 moved navigation into per-Application files, that struct silently
// matched nothing and BOTH hardcoding gates started checking an empty list -- passing on every
// hardcoded route and label there is. A gate that reads the manifest through a private copy of
// its shape cannot notice the shape changing, so this reads it through the loader that defines it.
//
// Unfiltered matters for the same reason routeByID/labelByID read AllNavigation: an Application
// with show_nav: false still owns its routes and labels, so both gates must still cover them.
func declaredNavItems(t *testing.T) []domain.NavigationItem {
	t.Helper()
	path := filepath.Join(repoRoot(), "metadata", "workspaces", "default.yaml")
	app, err := metadata.LoadApplication(path)
	if err != nil {
		t.Fatalf("LoadApplication(%s): %v", path, err)
	}
	items := append([]domain.NavigationItem(nil), app.Workspace.Navigation...)
	for _, a := range app.Workspace.Applications {
		items = append(items, a.AllNavigation...)
	}
	if len(items) == 0 {
		t.Fatal("manifest declares no navigation items -- these gates would check nothing, which is how they silently stopped working once before")
	}
	return items
}

func declaredNavRoutes(t *testing.T) []string {
	t.Helper()
	var routes []string
	for _, n := range declaredNavItems(t) {
		routes = append(routes, n.Route)
	}
	return routes
}

// chiGetCall matches one chi `.Get("/route", ...)` registration in router.go's source text.
// Navigation items are always link destinations (a GET), so Get is the only verb this checks.
var chiGetCall = regexp.MustCompile(`\.Get\(\s*"([^"]+)"`)

// TestNavigationRoutesAreRegistered closes the gap domain.NavigationItem's own doc comment names:
// "Route existence is therefore not cross-checked against anything at load time... routes are a
// runtime/internal/web concern, not a metadata one." metadata/workspaces/default.yaml can declare a route no
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
		// A declared route may carry a query (nav_my_documents is /approval-inbox?tab=mine): two
		// navigation items pointing at one handler that reads the query to decide which of its two
		// lists it composes. chi routes on path alone, so that is what this matches -- the query is
		// the handler's input, not a second registration. A route whose *path* has no handler still
		// fails exactly as before.
		if path, _, _ := strings.Cut(route, "?"); !registered[path] {
			t.Errorf("metadata/workspaces/default.yaml declares navigation route %q, but internal/web/router.go registers no GET handler for path %q", route, path)
		}
	}
}

// runtimeLevelRoutes are the only literal href="/..." (or Go string literal) values allowed to
// equal a metadata-declared navigation route: destinations that exist regardless of which
// Application is configured -- wired as fixed literals in internal/web/router.go itself
// (pr.Get("/home", ...), etc.), not something metadata *defines* so much as *labels* for the
// topbar. An Application's own screens (/dashboard, /approval-inbox, /documents/new, ...) have no
// such exemption: those routes only exist because this Application's metadata says so, so a page
// linking to one of its own siblings must call routeByID(id) (internal/rendering/machine.templ),
// never retype the route -- exactly the drift this session found twice (workspacehome.templ's
// /approval-inbox and /documents/new; then approvalinbox.templ's /documents/new,
// documentsubmit.templ's /approval-inbox, sprintdashboard.templ's /team-capacity) before this
// gate covered every .templ/.go file instead of only Workspace-level ones.
var runtimeLevelRoutes = map[string]bool{
	"/home": true,
	// Pre-auth screens, registered as fixed literals in router.go (r.Get("/login", ...) and
	// friends) and reachable before any Application context exists at all -- the purest form of
	// this map's own criterion. They are declared by no navigation list because there is nothing
	// to declare them in: a signed-out visitor has no Workspace yet.
	"/login":               true,
	"/register":            true,
	"/forgot-password":     true,
	"/resend-verification": true,
	// "/" is All Machines (nav_machines), Workspace-level navigation since Fase 3 and registered
	// as a fixed literal in router.go exactly like /home -- so it meets this map's own criterion
	// and belongs here. It was missed when All Machines moved up a level, and the cost showed up
	// immediately: internal/web/currentapp.go had to rewrite a plain strings.Cut(rest, "/") as
	// IndexByte with a rune to get past this gate, because a path *separator* is the same string
	// as this route. That is a false positive, not the gate working -- "/" carries no Application
	// meaning, and every future http.Redirect(w, r, "/", ...) or strings.Split(p, "/") would trip
	// the same wire and get contorted the same way, until someone weakened the gate instead.
	"/":                  true,
	"/workspace-members": true,
	// Same criterion as /workspace-members directly above: a Workspace-level destination that
	// exists regardless of which Application is configured, registered as a fixed literal in
	// router.go. Its handlers redirect to it by that literal for the same reason theirs do.
	"/workspace-groups": true,
	"/switch-workspace": true,
	"/choose-workspace": true,
	// /switch-workspace's own "Add workspace" row (ChooseWorkspacePage's canCreateWorkspace,
	// 2026-09-21) -- same criterion as the two routes directly above, a fixed router.go literal
	// reachable regardless of which Application is configured.
	"/create-workspace": true,
	// Account menu destinations (internal/rendering/appshell.templ's accountMenu, Account menu
	// port 2026-09-21): the signed-in identity's own settings, same criterion as
	// /workspace-members above -- they exist regardless of which Applications a Workspace
	// configures, registered as fixed literals in router.go, declared by no navigation list.
	"/account-profile":  true,
	"/account-security": true,
}

var templHref = regexp.MustCompile(`href="(/[^"{]*)"`)

// TestRenderingHasNoHardcodedApplicationRoute is the gate: no internal/rendering/*.templ file may
// contain a literal href equal to a metadata/workspaces/default.yaml navigation route, unless that route is
// runtime-level (runtimeLevelRoutes) -- an Application's own route must come from routeByID or an
// equivalent Go value, never a retyped literal, so it follows metadata instead of silently going
// stale next to it. Universal across every .templ file on purpose: an earlier version of this
// test only checked Workspace-level pages, reasoning that an Application-level page's own
// cross-links to its sibling screens were "normal same-Application navigation" and therefore
// exempt -- checking the actual data proved that reasoning wrong (three real instances found, all
// fixed by routeByID, zero false positives from the routes that are genuinely runtime-level), so
// the narrower version was replaced rather than kept alongside this one.
func TestRenderingHasNoHardcodedApplicationRoute(t *testing.T) {
	navRoutes := make(map[string]bool)
	for _, route := range declaredNavRoutes(t) {
		if !runtimeLevelRoutes[route] {
			navRoutes[route] = true
		}
	}

	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range templHref.FindAllStringSubmatch(string(src), -1) {
			if navRoutes[m[1]] {
				t.Errorf("%s hardcodes href=%q, a route metadata/workspaces/default.yaml already declares -- link to it with routeByID(...) instead", path, m[1])
			}
		}
	}
}

// staticAssetHref matches an href ending in a file extension -- /css/app.css, /manifest.json,
// /icons/icon-180.png. Those are files the browser fetches, not destinations a person navigates
// to, so they are outside every question this file asks about routes.
var staticAssetHref = regexp.MustCompile(`\.[a-z0-9]{2,5}$`)

// TestRenderingLinksOnlyToDeclaredRoutes points the opposite way to the two hardcoding gates
// above, and closes the blind spot they share. Those ask "is this literal equal to a route
// metadata declares?" -- so a route that *should* be declared and simply isn't passes both in
// silence. Fase 4 shipped /workspace-groups hardcoded in four places in groups.templ and declared
// in no navigation list, and neither gate said a word (found by the session that wrote it, not by
// this suite).
//
// The rule here is narrow on purpose: a literal href in a .templ is, by definition, a destination
// someone navigates to, so it must be a destination metadata knows about. Combined with the gate
// above, the two say something simpler than either does alone -- a literal app-path href is always
// wrong. It either names a declared route, and then routeByID should have been used, or it names
// an undeclared screen, and then the screen should be declared.
//
// Three exemptions, each with a reason rather than a convenience:
//
//   - Static assets (staticAssetHref): files, not routes.
//   - /machines/... : writing-guide.md's own rule, "a generic Machine's own page is always
//     reachable at /machines/{id} even with no navigation entry at all". Demanding a nav item for
//     Lists and Labels would contradict documented behaviour, not enforce it.
//   - runtimeLevelRoutes: destinations that exist regardless of which Application is configured,
//     including the pre-auth screens, which no navigation list can declare because a signed-out
//     visitor has no Workspace yet.
//
// A declared route is skipped rather than reported, so a violation is named once by whichever
// gate actually applies to it instead of twice by both.
//
// undeclaredScreenRatchet is the frozen violation set, the same shape projectionRatchet uses and
// for the same reason: this gate states an invariant that does not hold yet, so it freezes what
// is broken rather than failing the build. The list may only shrink.
// applicationSubScreens are Application screens reached from a parent screen's own primary action
// rather than from any menu -- deliberately undeclared, and therefore deliberately linked by
// literal. This is a third category the gate's own doc comment above did not anticipate: not a
// runtime-level route, and not debt either, which is why it is separate from the ratchet below (a
// ratchet may only shrink; this is a standing exemption that shrinks when a *capability* lands).
//
// The missing capability is a Page declaring its own primary action -- 007 §12.3's ActionBar
// component, still unbuilt. Until it exists, "the Approval Inbox has a + New Document button
// pointing at the submit form" is sayable only in a .templ. The Review screen
// (/machines/{id}/records/{id}/review) is the same category and has always been a literal, in
// internal/composition rather than a .templ, which is the only reason this gate never saw it.
//
// Re-check at each ROADMAP.md phase close (CLAUDE.md step 4): if ActionBar or an equivalent lands,
// these become declarable and the entry leaves. Also listed in writing-guide.md's "What comes free
// vs. what's hardcoded today" table.
var applicationSubScreens = map[string]string{
	"/documents/new": "Approval Inbox's + New Document action; nav_new_approval deleted 2026-09-21 (owner) -- a submit form is not a menu destination. Leaves when 007 §12.3 ActionBar lands.",
}

var undeclaredScreenRatchet = map[string]string{
	// Empty since Fase 6a, and that is the intended end state rather than a gap: /workspace-groups
	// was this list's only entry, and declaring nav_workspace_groups (metadata/workspaces/default.yaml) closed it
	// along with the viewer-level hiding that made declaring it safe. The map stays so the gate
	// keeps its shape -- a future undeclared screen gets frozen here rather than failing a build
	// mid-port -- but per the ratchet rule it may only shrink, so nothing should ever be added
	// without the same kind of note explaining when it leaves.
}

func TestRenderingLinksOnlyToDeclaredRoutes(t *testing.T) {
	declared := make(map[string]bool)
	for _, route := range declaredNavRoutes(t) {
		declared[route] = true
	}

	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range templHref.FindAllStringSubmatch(string(src), -1) {
			href := m[1]
			switch {
			case staticAssetHref.MatchString(href),
				strings.HasPrefix(href, "/machines/"),
				runtimeLevelRoutes[href],
				declared[href]:
				continue
			}
			if _, exempt := applicationSubScreens[href]; exempt {
				continue
			}
			seen[href] = true
			if _, allowed := undeclaredScreenRatchet[href]; allowed {
				continue
			}
			t.Errorf("%s links to %q, which no navigation list declares -- a screen someone can reach should be a destination metadata knows about, or the link should point somewhere that is", path, href)
		}
	}

	for href := range undeclaredScreenRatchet {
		if !seen[href] {
			t.Errorf("undeclaredScreenRatchet still lists %q, but nothing links to it undeclared any more -- delete the entry so the list keeps measuring real remaining debt", href)
		}
	}
}

// TestHandlersHaveNoHardcodedApplicationRoute is TestRenderingHasNoHardcodedApplicationRoute's
// Go-side counterpart, over every internal/web/*.go file (not just ones calling a particular
// page): a handler could otherwise satisfy the .templ-side check by hardcoding the route itself
// and merely passing a clean-looking variable into the page, pushing the violation down one layer
// instead of removing it. Parsed with go/ast, not a text regexp, so a route mentioned in a
// comment doesn't false-positive -- the same rigor boundary_test.go/handlersize_test.go already
// use for their own Go-source checks.
func TestHandlersHaveNoHardcodedApplicationRoute(t *testing.T) {
	navRoutes := make(map[string]bool)
	for _, route := range declaredNavRoutes(t) {
		if !runtimeLevelRoutes[route] {
			navRoutes[route] = true
		}
	}

	dir := filepath.Join(repoRoot(), "internal", "web")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		// router.go is the routing table itself -- the one place a route string is the
		// registration, not a duplicate of one (it's also TestNavigationRoutesAreRegistered's own
		// source of truth for "is this route registered").
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") || e.Name() == "router.go" {
			continue
		}
		path := filepath.Join(dir, e.Name())
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
				t.Errorf("%s hardcodes %q, a route metadata/workspaces/default.yaml already declares -- it must come from a domain.Application field threaded through web.Deps, not be retyped here", fset.Position(lit.Pos()), value)
			}
			return true
		})
	}
}

func declaredNavLabels(t *testing.T) []string {
	t.Helper()
	var labels []string
	for _, n := range declaredNavItems(t) {
		labels = append(labels, n.Label)
	}
	return labels
}

// templPageShellTitle matches a `pageShell("...", ...)` call's title argument -- the canonical
// site a Page names its own title, and (with detailBackLink/workspacehome's card text) one of the
// three real shapes TestRenderingHasNoHardcodedApplicationLabel found hardcoded before this gate
// existed (documentsubmit.templ, approvalinbox.templ).
var templPageShellTitle = regexp.MustCompile(`pageShell\(\s*"([^"]*)"`)

// templTagText matches a run of Title-Case words between two tags -- `<h1>Approval Inbox</h1>`,
// `← Approval Inbox` inside an `<a>`, a card's plain-text body -- covering the rendered-text shape
// pageShell's title argument doesn't (workspacehome.templ's card subtitle, detail.templ's back
// link). Every declared label in metadata/workspaces/default.yaml today is itself a Title-Case phrase, so this is
// scoped to that shape rather than matching arbitrary text -- exactly the same "match the specific
// shape that's actually at risk" posture templHref already takes for hrefs, not a blanket
// substring search (which false-positives on prose, per capabilities.md's own
// TestWritingGuideMachinesMatchMetadata history in menata-app-document's development-history.md).
var templTagText = regexp.MustCompile(`>[^<{]*?([A-Z][a-zA-Z]+(?:\s[A-Z][a-zA-Z]+)*)\s*<`)

// TestRenderingHasNoHardcodedApplicationLabel is TestRenderingHasNoHardcodedApplicationRoute's
// label-side counterpart: no internal/rendering/*.templ file may render a metadata-declared
// navigation label as a literal -- it must come from rendering.labelByID(id) instead, so a label
// edit in metadata/workspaces/default.yaml can't go stale next to a page that still shows the old wording. Found
// three real instances before this test existed (label text sitting on the very same line as an
// already-correct routeByID href): approvalinbox.templ and documentsubmit.templ's own pageShell
// title + <h1>, and workspacehome.templ's "Approval Inbox" card subtitle -- all three fixed via
// labelByID as this gate's own first commit.
func TestRenderingHasNoHardcodedApplicationLabel(t *testing.T) {
	navLabels := make(map[string]bool)
	for _, label := range declaredNavLabels(t) {
		navLabels[label] = true
	}

	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		found := make(map[string]bool)
		for _, m := range templPageShellTitle.FindAllStringSubmatch(string(src), -1) {
			found[m[1]] = true
		}
		for _, m := range templTagText.FindAllStringSubmatch(string(src), -1) {
			found[m[1]] = true
		}
		for literal := range found {
			if navLabels[literal] {
				t.Errorf("%s hardcodes %q, a label metadata/workspaces/default.yaml already declares -- render it with labelByID(...) instead", path, literal)
			}
		}
	}
}

// TestHandlersHaveNoHardcodedApplicationLabel is TestHandlersHaveNoHardcodedApplicationRoute's
// label-side counterpart, over every internal/web/*.go file, parsed with go/ast for the same
// comment-safety reason the route version already is.
func TestHandlersHaveNoHardcodedApplicationLabel(t *testing.T) {
	navLabels := make(map[string]bool)
	for _, label := range declaredNavLabels(t) {
		navLabels[label] = true
	}

	dir := filepath.Join(repoRoot(), "internal", "web")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") || e.Name() == "router.go" {
			continue
		}
		path := filepath.Join(dir, e.Name())
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
			if navLabels[value] {
				t.Errorf("%s hardcodes %q, a label metadata/workspaces/default.yaml already declares -- it must come from a domain.Application field or rendering.labelByID, not be retyped here", fset.Position(lit.Pos()), value)
			}
			return true
		})
	}
}

// composedScreenDatasets names every Dataset (and the Measures within it) that a composed screen
// in internal/composition looks up by id, mapped to the Machine file expected to declare it.
// Stated explicitly here, the same posture behavior_gate_test.go's mutatingWriteGuards takes,
// rather than parsed out of the Go source: this table *is* the contract, and writing it down is
// what makes a silent break impossible.
//
// Without this, a Dataset renamed or removed in YAML compiles fine, passes every unit test (whose
// fixtures mirror metadata rather than reading it), and only surfaces as a runtime error the first
// time someone opens the page -- exactly the class of metadata/code drift
// TestNavigationRoutesAreRegistered already closes for routes.
var composedScreenDatasets = map[string]map[string][]string{
	"task.yaml": {
		"ds_task_workload":   {"msr_total", "msr_total_open"}, // Team Capacity, and Sprint Dashboard's workload column
		"ds_task_by_project": {"msr_total", "msr_total_open"}, // Dashboard's Project rollup
		"ds_task_by_status":  {"msr_total"},                   // Sprint Dashboard's headline counts
	},
	"user.yaml":     {"ds_user_capacity": {"msr_total_capacity"}},
	"document.yaml": {"ds_document_by_status": {"msr_total"}},
}

func TestComposedScreenDatasetsAreDeclared(t *testing.T) {
	for file, datasets := range composedScreenDatasets {
		m, err := metadata.Load(filepath.Join(repoRoot(), "metadata", file))
		if err != nil {
			t.Fatalf("load %s: %v", file, err)
		}
		for datasetID, measureIDs := range datasets {
			ds, ok := m.DatasetByID(datasetID)
			if !ok {
				t.Errorf("internal/composition reads dataset %q, but metadata/%s declares no such dataset -- a composed screen naming a dataset metadata dropped renders nothing but zeroes", datasetID, file)
				continue
			}
			declared := make(map[string]bool, len(ds.Measures))
			for _, ms := range ds.Measures {
				declared[ms.ID] = true
			}
			for _, measureID := range measureIDs {
				if !declared[measureID] {
					t.Errorf("internal/composition reads measure %q of dataset %q, but metadata/%s declares no such measure", measureID, datasetID, file)
				}
			}
		}
	}
}

// TestApprovalStepDeclaresStatusRollup guards the one declaration the Document-approval flow's own
// correctness now rests on. A Document's status follows its Approval Steps by metadata
// (evt_step_decision_rollup) rather than by the hardcoded recompute that used to live in
// internal/web -- so if that Event is renamed or dropped, decisions still succeed and the
// Document's status silently stops moving, which no other test would notice: internal/web's
// approval tests build a synthetic Machine fixture that mirrors this declaration rather than
// reading it (the same fixture-drift gap TestComposedScreenDatasetsAreDeclared closes for
// Datasets).
func TestApprovalStepDeclaresStatusRollup(t *testing.T) {
	m, err := metadata.Load(filepath.Join(repoRoot(), "metadata", "approval_step.yaml"))
	if err != nil {
		t.Fatalf("load approval_step.yaml: %v", err)
	}
	for _, e := range m.Events {
		if e.Then.Name != domain.ServiceRollupParentStatus {
			continue
		}
		if e.Then.Rollup == nil {
			t.Fatalf("event %q declares %s with no rollup configuration", e.ID, domain.ServiceRollupParentStatus)
		}
		if e.On != "fld_decision" {
			t.Errorf("rollup event %q watches %q, want fld_decision -- the Document's status follows a step's decision", e.ID, e.On)
		}
		return
	}
	t.Errorf("metadata/approval_step.yaml declares no %s event -- a Document's status would stop following its own Approval Steps, silently, while every decision still succeeds", domain.ServiceRollupParentStatus)
}

// TestApprovalStepDeclaresSequencing is TestApprovalStepDeclaresStatusRollup's counterpart for the
// other rule the approval flow's correctness rests on, and it guards a failure that is worse than
// the rollup's: sequencing is now opt-in per Machine (behavior.CanAct returns true when a Machine
// declares none), so dropping this block does not break a build or fail a request -- it silently
// unlocks every approval step at once, letting a later approver decide before an earlier one.
// internal/composition's own tests supply a fixture mirroring this rather than reading it, so
// nothing else would notice.
func TestApprovalStepDeclaresSequencing(t *testing.T) {
	m, err := metadata.Load(filepath.Join(repoRoot(), "metadata", "approval_step.yaml"))
	if err != nil {
		t.Fatalf("load approval_step.yaml: %v", err)
	}
	if m.Sequencing == nil {
		t.Fatal("metadata/approval_step.yaml declares no sequencing: block -- every approval step would become actionable at once, with no error anywhere")
	}
	if m.Sequencing.OpenValue == "" || m.Sequencing.SequentialValue == "" {
		t.Errorf("sequencing declares open_value=%q sequential_value=%q -- both are what make ordering apply at all", m.Sequencing.OpenValue, m.Sequencing.SequentialValue)
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

// TestCapabilitiesComponentsTableCitesPromotionGuide keeps the "Shared rendering components"
// table's own admission criteria from silently rotting into an unwritten house opinion again --
// the same failure mode 007 §40 already flags for the wider composable-runtime target (PROPOSED
// claims need a citation, not just an assertion). The actual checklist lives in the
// `menata-app-document` companion repo (`guides/ui-composition-decomposition-criteria.md`,
// grounded in `references/ui-composition-decomposition-frameworks.md`'s world-class survey), a
// separate repo this test cannot read -- so this only checks that capabilities.md still names it,
// the same cross-repo citation-by-name posture CLAUDE.md already uses for `ui-sample/*.html`.
func TestCapabilitiesComponentsTableCitesPromotionGuide(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(repoRoot(), "capabilities.md"))
	if err != nil {
		t.Fatalf("read capabilities.md: %v", err)
	}
	section := markdownSection(t, doc, "### Shared rendering components")
	for _, line := range section {
		if strings.Contains(line, "ui-composition-decomposition-criteria.md") {
			return
		}
	}
	t.Error(`capabilities.md's "Shared rendering components" section no longer cites guides/ui-composition-decomposition-criteria.md (menata-app-document) -- the promotion criteria became an unwritten house opinion again; restore the citation or replace it with wherever the criteria moved to`)
}

// TestDynamicActorGateIsDeclaredAndResolvable checks the real metadata, not a fixture: CAP-F24's
// gate is declared on the Permission that governs deciding, and every Field it names exists on
// that Machine with the type the resolver needs.
//
// Load-time validation (internal/metadata.validateDynamicActor) already refuses a malformed gate,
// so this is not a second copy of that. It answers the *other* question, the one that has burned
// this repo three times in a day: **is the thing declared at all?** A gate that is simply absent
// passes every validator ever written, because there is nothing to validate — exactly how two
// route conformance gates ran against an empty list while reporting `ok`, and how Projection sat
// wired end to end with zero declarations. The capability is only real if the metadata says so.
func TestDynamicActorGateIsDeclaredAndResolvable(t *testing.T) {
	app, err := metadata.LoadApplication(filepath.Join(repoRoot(), "metadata", "workspaces", "default.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication: %v", err)
	}
	const stepMachineID = "mch_approval_step"
	var step *domain.Machine
	for _, m := range app.Machines {
		if m.ID == stepMachineID {
			step = m
			break
		}
	}
	if step == nil {
		t.Fatalf("%s is not declared in the real manifest", stepMachineID)
	}

	perms := step.PermissionsFor(domain.ActionDecide)
	if len(perms) == 0 {
		t.Fatalf("%s declares no Permission governing %q -- deciding would be unrestricted", step.ID, domain.ActionDecide)
	}
	gated := 0
	for _, p := range perms {
		if p.DynamicActor == nil {
			continue
		}
		gated++
		fields := map[string]domain.FieldType{
			p.DynamicActor.ActorTypeField:  domain.FieldTypeStatus,
			p.DynamicActor.ActorGroupField: domain.FieldTypeGroup,
		}
		for id, want := range fields {
			f, ok := fieldByID(step, id)
			if !ok {
				t.Errorf("permission %q names %q, which %s does not declare", p.ID, id, step.ID)
				continue
			}
			if f.Type != want {
				t.Errorf("permission %q: %q is %q, want %q", p.ID, id, f.Type, want)
			}
		}
		if f, ok := fieldByID(step, p.DynamicActor.ActorUserField); !ok || !f.IsReference() {
			t.Errorf("permission %q: actor_user_field %q must be a declared identity field", p.ID, p.DynamicActor.ActorUserField)
		}
		// The fallback is the property that made adoption safe; a gate declared without one would
		// leave every pre-CAP-F24 record ungoverned.
		if p.ActorField == "" {
			t.Errorf("permission %q declares a dynamic gate but no actor_field fallback -- every record written before the gate existed would stop being governed", p.ID)
		}
	}
	if gated == 0 {
		t.Errorf("no Permission on %s declares a dynamic actor gate -- CAP-F24 is built in code and unreachable from metadata, which is the state this suite exists to catch", step.ID)
	}
}

func fieldByID(m *domain.Machine, id string) (domain.Field, bool) {
	for _, f := range m.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return domain.Field{}, false
}

// TestApprovalStepDeclaresItsTransitions checks the real manifest, not a fixture, for the same
// reason TestDynamicActorGateIsDeclaredAndResolvable and TestApprovalStepDeclaresSequencing do:
// load-time validation refuses a *malformed* declaration, and has nothing at all to say about one
// that is simply absent.
//
// An absent transitions: block here is not a cosmetic gap. behavior.CheckTransitions reads an
// undeclared state model as "this Machine restricts nothing" (Principle #6), so deleting these
// two lines silently reopens two things at once: an already-decided step becomes decidable again
// on a parallel Document, and fld_decision becomes writable straight through the generic edit
// route -- which is what internal/web's allowsDecisionChange used to stop by hand before this
// declaration replaced it. Both would pass every other test in this repo.
func TestApprovalStepDeclaresItsTransitions(t *testing.T) {
	step := machineFromManifest(t, "mch_approval_step")

	if len(step.Transitions) == 0 {
		t.Fatal("mch_approval_step declares no transitions: -- a decision would become reversible and directly editable, with nothing else failing")
	}
	for _, want := range []struct{ from, to string }{{"pending", "approved"}, {"pending", "rejected"}} {
		tr, ok := step.TransitionFor("fld_decision", want.from, want.to)
		if !ok {
			t.Errorf("mch_approval_step declares no fld_decision transition %q -> %q", want.from, want.to)
			continue
		}
		if tr.Action != domain.ActionDecide {
			t.Errorf("transition %q: action = %q, want %q -- reserving it for decide is what keeps the generic edit route from being a bypass", tr.ID, tr.Action, domain.ActionDecide)
		}
	}
	// The other half of the rule, and the one that is easy to lose by "just adding an edge":
	// nothing may leave a decided state.
	for _, from := range []string{"approved", "rejected"} {
		if got := step.TransitionsFrom("fld_decision", from); len(got) > 0 {
			t.Errorf("mch_approval_step declares %d transition(s) leaving %q -- a decision is final, and composition.canStillDecide reads exactly this to stop offering the bar", len(got), from)
		}
	}
}

// TestDocumentStatusIsDerivedNotSettable holds the second thing Fase 7's transitions declare: a
// Document's status is computed from its steps (evt_step_decision_rollup), so no Action performs
// any of its edges.
//
// The failure this guards is the one that was live until Fase 7 and that no test noticed: the
// generic update route rewrites a record from whatever the form submits, so any authenticated
// member could set fld_status to "approved" directly -- skipping every step, the sequencing rule
// and the PDF compositing at once. Giving one of these edges an action: would restore it.
func TestDocumentStatusIsDerivedNotSettable(t *testing.T) {
	document := machineFromManifest(t, "mch_document")

	if len(document.Transitions) == 0 {
		t.Fatal("mch_document declares no transitions: -- fld_status becomes directly writable through the generic update route again")
	}
	for _, tr := range document.Transitions {
		if tr.Field != "fld_status" {
			continue
		}
		if tr.Action != "" {
			t.Errorf("transition %q: action = %q, want none -- a Document's status is derived from its Approval Steps, so declaring an action makes it settable by hand", tr.ID, tr.Action)
		}
	}
}

// TestApprovalStepPermissionsCarryRoles is CAP-P01's own declaration gate, the role-side twin of
// the two above.
//
// authorization.holdsOneOf reads an empty roles: as "this Permission says nothing about roles",
// which is the right default and exactly what makes its absence invisible: dropping the arm from
// the manifest returns the app to Fase 6's behaviour -- anyone a submitter names as an approver
// may decide, whatever role they hold -- with every unit test still green, because the unit tests
// build their own Permissions. Only the real manifest can answer whether the rule is live.
func TestApprovalStepPermissionsCarryRoles(t *testing.T) {
	app, err := metadata.LoadApplication(filepath.Join(repoRoot(), "metadata", "workspaces", "default.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication: %v", err)
	}
	step := machineFromManifest(t, "mch_approval_step")

	if step.ApplicationID == "" {
		t.Fatal("mch_approval_step is claimed by no application -- a role-bearing permission on it could never be satisfied (domain.Actor.HasRole returns false for an empty application id)")
	}
	declared := map[string]bool{}
	for _, a := range app.Workspace.Applications {
		if a.ID == step.ApplicationID {
			for _, r := range a.Roles {
				declared[r] = true
			}
		}
	}

	for _, action := range []string{domain.ActionDecide, domain.ActionEdit, domain.ActionDelete} {
		perms := step.PermissionsFor(action)
		if len(perms) == 0 {
			t.Errorf("mch_approval_step declares no %q permission at all", action)
			continue
		}
		for _, p := range perms {
			if len(p.Roles) == 0 {
				t.Errorf("permission %q (%s) names no roles -- the role gate is off for this action and nothing else reports it", p.ID, action)
			}
			for _, role := range p.Roles {
				if !declared[role] {
					t.Errorf("permission %q names role %q, which application %q does not declare", p.ID, role, step.ApplicationID)
				}
			}
		}
	}
}

// machineFromManifest loads one Machine out of the real manifest, through the loader rather than
// by re-parsing the YAML -- the lesson declaredNavItems records above, applied here: a private
// copy of the shape cannot notice the shape changing. It goes through LoadApplication rather than
// Load so ApplicationID is stamped, which the role checks above depend on.
func machineFromManifest(t *testing.T, id string) *domain.Machine {
	t.Helper()
	app, err := metadata.LoadApplication(filepath.Join(repoRoot(), "metadata", "workspaces", "default.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication: %v", err)
	}
	for _, m := range app.Machines {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("%s is not declared in the real manifest", id)
	return nil
}

// TestKnownIconsAreAllDrawn closes the one seam domain.KnownIcons cannot close by itself: it is a
// list of *names*, and a name is only worth declaring if something draws it. internal/rendering's
// icon templ deliberately does not panic on an unknown name (an icon is decoration beside a label
// that still reads), so a name added to the set and never drawn would ship as an empty 24x24 box
// in a bottom-bar tab or an Application tile, with no error anywhere -- the same
// invisible-at-runtime failure shape as a colour token with no appIconClasses branch, which is
// exactly why KnownApplicationColors' own doc comment warns it "must be extended here in step".
//
// The check reads the switch's `case` labels out of icons.templ rather than rendering each icon,
// because what can go wrong is a missing branch, not a wrong path: a typo'd `d=` attribute is a
// drawing bug a test like this could never see, while a missing case is exactly what it catches.
//
// One direction only, matching TestCapabilitiesComponentsTableMatchesTempl's own reasoning:
// a drawn icon that KnownIcons does not list is unreachable from metadata but perfectly usable by
// the chrome (chevron-right, home, grid and more are drawn for appShell and named by no manifest),
// so requiring the reverse would forbid the runtime from having icons of its own.
func TestKnownIconsAreAllDrawn(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "rendering", "icons.templ"))
	if err != nil {
		t.Fatalf("read icons.templ: %v", err)
	}
	drawn := make(map[string]bool)
	for _, m := range regexp.MustCompile(`(?m)^\s*case "([a-z-]+)":`).FindAllStringSubmatch(string(src), -1) {
		drawn[m[1]] = true
	}
	if len(drawn) == 0 {
		t.Fatal("internal/rendering/icons.templ declares no `case \"...\":` branches -- the icon switch is gone or changed shape")
	}
	for name := range domain.KnownIcons {
		if !drawn[name] {
			t.Errorf("domain.KnownIcons declares %q, but internal/rendering/icons.templ draws no case for it -- metadata could name an icon that renders an empty box", name)
		}
	}
}
