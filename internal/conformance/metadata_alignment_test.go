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
	path := filepath.Join(repoRoot(), "metadata", "app.yaml")
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
	"/switch-workspace":  true,
	"/choose-workspace":  true,
	"/login":             true,
}

var templHref = regexp.MustCompile(`href="(/[^"{]*)"`)

// TestRenderingHasNoHardcodedApplicationRoute is the gate: no internal/rendering/*.templ file may
// contain a literal href equal to a metadata/app.yaml navigation route, unless that route is
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
				t.Errorf("%s hardcodes href=%q, a route metadata/app.yaml already declares -- link to it with routeByID(...) instead", path, m[1])
			}
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
				t.Errorf("%s hardcodes %q, a route metadata/app.yaml already declares -- it must come from a domain.Application field threaded through web.Deps, not be retyped here", fset.Position(lit.Pos()), value)
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
// link). Every declared label in metadata/app.yaml today is itself a Title-Case phrase, so this is
// scoped to that shape rather than matching arbitrary text -- exactly the same "match the specific
// shape that's actually at risk" posture templHref already takes for hrefs, not a blanket
// substring search (which false-positives on prose, per capabilities.md's own
// TestWritingGuideMachinesMatchMetadata history in menata-app-document's development-history.md).
var templTagText = regexp.MustCompile(`>[^<{]*?([A-Z][a-zA-Z]+(?:\s[A-Z][a-zA-Z]+)*)\s*<`)

// TestRenderingHasNoHardcodedApplicationLabel is TestRenderingHasNoHardcodedApplicationRoute's
// label-side counterpart: no internal/rendering/*.templ file may render a metadata-declared
// navigation label as a literal -- it must come from rendering.labelByID(id) instead, so a label
// edit in metadata/app.yaml can't go stale next to a page that still shows the old wording. Found
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
				t.Errorf("%s hardcodes %q, a label metadata/app.yaml already declares -- render it with labelByID(...) instead", path, literal)
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
				t.Errorf("%s hardcodes %q, a label metadata/app.yaml already declares -- it must come from a domain.Application field or rendering.labelByID, not be retyped here", fset.Position(lit.Pos()), value)
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
