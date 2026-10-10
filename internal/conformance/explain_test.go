package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

// TestInstalledCastsExplainWithoutDefects is the control over reality, the same job
// TestUnboundMachinesAreNotTheEngines does for the binding: every Workspace actually installed is
// run through action.ExplainCast, and no resolution may come back a defect.
//
// A defect here is one of exactly two things, and both have shipped (domain.Resolution's own
// comment): StatusUndeclared, a Machine declaring nothing for a question its role owes -- Stage E1's
// shape, one probe away from saving an approval flow with no approvers; or StatusInputUnavailable,
// a declaration that exists with no input to read it against -- the /review 404, invisible for a day
// in production because the empty result rendered as a legitimate-looking 404.
//
// **StatusNotApplicable is not a defect and this test must never treat it as one.** It is the
// majority: measured 2026-09-29, 38 of 50 derivations on cast Machines are empty because they belong
// to another role -- a Document is not decided, a signature store has no state model, nothing decides
// a template. An earlier attempt to gate these numbers without that distinction produced noise, which
// is why registry.WorkflowEngineSpec.Answers exists at all.
func TestInstalledCastsExplainWithoutDefects(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	if len(wss) == 0 {
		t.Fatal("no Workspace manifests found -- this gate would pass by measuring nothing")
	}

	slugs := make([]string, 0, len(wss))
	for slug := range wss {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	engaged := 0
	for _, slug := range slugs {
		ws := wss[slug].Workspace
		for engine := range registry.KnownWorkflowEngines {
			if len(ws.MachinesInWorkflowRole(engine, domain.WorkflowRoleStep)) == 0 {
				continue // this Workspace installs no Application binding this engine
			}
			engaged++

			counts := map[domain.ResolutionStatus]int{}
			for _, r := range action.ExplainCast(ws, engine, "") {
				counts[r.Status]++
				if r.IsDefect() {
					t.Errorf("workspace %q, engine %q: %s is %s\n"+
						"  read from: %s\n"+
						"  a derivation the runtime makes and cannot explain is 001 #6's own failure mode --\n"+
						"  either declare what the role owes, or give the caller the input it needs",
						slug, engine, r.Name, r.Status, r.From)
				}
			}
			t.Logf("%-14s %-18s resolved=%d not-applicable=%d undeclared=%d input-unavailable=%d",
				slug, engine, counts[domain.StatusResolved], counts[domain.StatusNotApplicable],
				counts[domain.StatusUndeclared], counts[domain.StatusInputUnavailable])
		}
	}

	if engaged == 0 {
		t.Error("no installed Workspace engages any known workflow engine -- this gate measured nothing")
	}
}

// TestEveryDerivationIsOwedBySomeRole is the two-lists-drift guard between domain.Derivation* and
// WorkflowEngineSpec.Answers. A derivation in the first list and no second is asked of nobody, so it
// can only ever report not-applicable -- silently, which is the invisibility this whole surface exists
// to remove. The reverse fails too: a role owing a derivation action.ExplainCast cannot resolve would
// report undeclared forever, blaming metadata for a gap in Go.
//
// **The constant list is read out of the source, not retyped here.** It was a hand-maintained slice
// until 2026-09-29 and proved the point within an hour: adding DerivationStatusTargets and
// DerivationActionWrites made this test fail with "not a domain.Derivation* constant" -- about two
// constants that are, in fact, domain.Derivation* constants. A gate against drift that keeps its own
// third copy of the list is one more thing to drift.
func TestEveryDerivationIsOwedBySomeRole(t *testing.T) {
	all := derivationConstants(t)
	if len(all) == 0 {
		t.Fatal("no Derivation* constants found -- this gate would pass by measuring nothing")
	}

	owed := map[string][]string{}
	for engine, spec := range registry.KnownWorkflowEngines {
		for role, answers := range spec.Answers {
			if !contains(spec.Roles(), role) {
				t.Errorf("engine %q: Answers names role %q, which is in neither Required nor Optional", engine, role)
			}
			for _, a := range answers {
				if !contains(all, a.Derivation) {
					t.Errorf("engine %q role %q owes %q, which is not a domain.Derivation* constant", engine, role, a.Derivation)
				}
				owed[a.Derivation] = append(owed[a.Derivation], engine+"."+role)
			}
		}
	}

	// Every constant found by AST must be claimed by exactly one side of the partition. That is what
	// keeps the drift guard alive across the split: a new Derivation* in neither list fails here,
	// before either of the checks below can quietly skip it.
	for _, d := range all {
		engine := contains(domain.EngineDerivations, d)
		normalization := contains(domain.NormalizationDerivations, d)
		switch {
		case engine && normalization:
			t.Errorf("derivation %q is in both EngineDerivations and NormalizationDerivations", d)
		case !engine && !normalization:
			t.Errorf("derivation %q is in neither EngineDerivations nor NormalizationDerivations -- "+
				"nothing decides which check applies to it, so it would be verified by neither", d)
		case engine && len(owed[d]) == 0:
			t.Errorf("engine derivation %q is owed by no role in any engine -- it can only ever report "+
				"not-applicable, which makes it a question nothing answers and nobody sees", d)
		}
	}

	// And the reverse for the normalization half: an entry there that no role owes is correct, but one
	// that a role *does* owe is filed on the wrong side.
	for _, d := range domain.NormalizationDerivations {
		if len(owed[d]) > 0 {
			t.Errorf("normalization derivation %q is owed by %v -- it belongs in EngineDerivations", d, owed[d])
		}
	}
}

// derivationConstants reads the *values* of domain's Derivation* constants straight out of
// resolution.go. Values rather than names, because Answers holds values.
func derivationConstants(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot(), "internal", "domain", "resolution.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "Derivation") || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			out = append(out, strings.Trim(lit.Value, `"`))
		}
		return true
	})
	sort.Strings(out)
	return out
}

// unexplainedDerivationAccessors are the accessors action.ExplainCast deliberately does not reach, each
// with the reason. A closed map with a stated reason rather than a sweep, because the question needs
// judgement and this repo has one pattern for that (internal/web's readPathWriters,
// applicationSubScreens): the judgement is recorded once and reviewed, never embedded in the scan.
//
// **Measured before this was built, and the measurement is why it is a map.** Of 16 exported accessors
// across the two files, 12 were reached by explain.go and 4 were not -- and the four were four
// different things. A naive "every accessor must appear in Explain" gate would have been right about
// one and a half of them, a ~50% false rate, which is exactly the shape of the static
// fixture-discovery gate that was built, measured at 40 false findings across 10 packages, and
// deleted. Two of the four turned out to be real and were fixed rather than excused (ActionTargets and
// EffectFor now have Resolutions; Machine.OpenValue was deleted outright -- zero callers, and it read
// the source action.EngineFields.Open's own comment names as the wrong one). These two are what is
// genuinely left.
var unexplainedDerivationAccessors = map[string]string{
	"FlowTemplateRowFields": "a remapping of flow_template_step: into EngineFields, not a new derivation -- " +
		"its inputs are already explained as flow_template.flow_template_step, and explaining the same " +
		"declaration a second time under a second name is 001 #8",
}

// TestEveryDerivationAccessorIsExplainedOrExcused is the other half of the drift guard, and the half
// that was missing until 2026-09-29.
//
// TestEveryDerivationIsOwedBySomeRole holds Derivation* against Answers. Nothing held the accessors:
// a new function in internal/domain/actioneffect.go or internal/action/fields.go that never reaches
// Explain becomes an inference the runtime makes and nothing can explain -- silently, which is the
// exact failure 001 #6's second clause exists to prevent, reintroduced one accessor at a time.
//
// It fails three ways, and all three are mutation-proved: an accessor in neither the explainer nor the
// map, a map entry naming a function that no longer exists, and a map entry naming one that *is* now
// explained (an excuse outliving its reason is how an allowlist becomes a place to hide).
func TestEveryDerivationAccessorIsExplainedOrExcused(t *testing.T) {
	explainer := readFile(t, filepath.Join(repoRoot(), "internal", "action", "explain.go"))

	accessors := derivationAccessors(t)
	if len(accessors) < 10 {
		t.Fatalf("found only %d derivation accessors; the scan is measuring the wrong thing "+
			"(16 existed when this gate was written)", len(accessors))
	}

	for _, name := range accessors {
		explained := strings.Contains(explainer, "."+name+"(") || strings.Contains(explainer, name+"(m)") ||
			strings.Contains(explainer, name+"(stepMachine)") || strings.Contains(explainer, " "+name+"(")
		reason, excused := unexplainedDerivationAccessors[name]

		switch {
		case explained && excused:
			t.Errorf("%s is reached by internal/action/explain.go AND excused in "+
				"unexplainedDerivationAccessors (%q) -- remove the entry; an excuse that outlived its "+
				"reason is how an allowlist becomes a place to hide", name, reason)
		case !explained && !excused:
			t.Errorf("%s is a derivation accessor that internal/action/explain.go never reaches, and "+
				"unexplainedDerivationAccessors does not say why.\n"+
				"  An inference the runtime makes and nothing can explain is 001 #6's own failure mode:\n"+
				"  \"Hidden inference that cannot be explained is not an acceptable substitute for\n"+
				"  explicit configuration.\"\n"+
				"  Either give it a domain.Resolution, or add an entry here stating why it needs none.", name)
		}
	}

	for name := range unexplainedDerivationAccessors {
		if !contains(accessors, name) {
			t.Errorf("unexplainedDerivationAccessors names %s, which is not a derivation accessor any "+
				"more -- the entry protects nothing", name)
		}
	}
}

// derivationAccessors scans the two files that own this engine's derivations for exported functions
// and methods taking or receiving a Machine.
//
// Deliberately narrow, and the narrowness is the design: a wider scan catches helpers and turns the
// map above into a dumping ground, which is the failure mode of the deleted fixture gate. If this ever
// needs a long exclusion list to separate accessors from helpers, the discriminator is wrong and the
// answer is to stop and say so rather than to keep adding entries until it is green.
func derivationAccessors(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, rel := range [][]string{
		{"internal", "domain", "actioneffect.go"},
		{"internal", "action", "fields.go"},
	} {
		path := filepath.Join(append([]string{repoRoot()}, rel...)...)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			if !receivesOrTakesAMachine(fn) {
				continue
			}
			out = append(out, fn.Name.Name)
		}
	}
	sort.Strings(out)
	return out
}

// receivesOrTakesAMachine is the discriminator: a derivation accessor is asked *of a Machine*. A
// constructor, a pure helper over strings, or a type's own method is not one.
func receivesOrTakesAMachine(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			if strings.Contains(typeName(f.Type), "Machine") {
				return true
			}
		}
	}
	if fn.Type.Params == nil {
		return false
	}
	for _, f := range fn.Type.Params.List {
		if strings.Contains(typeName(f.Type), "Machine") {
			return true
		}
	}
	return false
}

func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// perViewerDatasets are `select: records` Datasets whose whole purpose is showing one identity its
// own records, mapped to the reason. Each must declare a $current_user predicate.
//
// **This exists because the scoping became declarative, and a declaration can be deleted.** Before
// 2026-09-29, composition.buildMyTasks filtered `fld_assignee != userID` in Go, where removing it
// would have failed a unit test. The predicate now lives in metadata/task.yaml's ds_my_tasks, and
// deleting it there is silent: the Dataset still loads, the screen still renders, and every viewer
// sees every assignee's Tasks bounded only by `limit: 200`. Measured by removing it -- the whole
// suite stayed green.
//
// A named list rather than a sweep, because "is this Dataset per-viewer" is a judgement about what a
// screen is for, which no scan can make. Same shape as readPathWriters and
// unexplainedDerivationAccessors: the judgement is recorded once and reviewed.
var perViewerDatasets = map[string]string{
	"ds_my_tasks": "My Tasks shows one identity its own assigned work; without the predicate every " +
		"viewer sees every assignee's Tasks",
}

func TestPerViewerDatasetsScopeByIdentity(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}

	found := map[string]bool{}
	for slug, app := range wss {
		for _, m := range app.Machines {
			for _, ds := range m.Datasets {
				reason, watched := perViewerDatasets[ds.ID]
				if !watched {
					continue
				}
				found[ds.ID] = true

				scoped := false
				for _, c := range ds.Where.Comparisons() {
					if c.Value == expression.SentinelCurrentUser {
						scoped = true
					}
				}
				if !scoped {
					t.Errorf("workspace %q: dataset %s declares no %s predicate.\n  %s.\n"+
						"  Its scoping is metadata now, so deleting the predicate is silent: the Dataset "+
						"still loads and the screen still renders.",
						slug, ds.ID, expression.SentinelCurrentUser, reason)
				}
			}
		}
	}

	for id := range perViewerDatasets {
		if !found[id] {
			t.Errorf("perViewerDatasets names %s, which no installed Workspace declares -- the entry "+
				"protects nothing", id)
		}
	}
}

// wholeMachineReadRatchet freezes how many times each file in internal/composition reads an entire
// Machine, and **it may only shrink** -- a higher count fails, a lower one fails too, and a new file
// fails, the same terms documentApprovalCoupling and projectionRatchet already carry.
//
// It measures the one thing declared record selection exists to fix. 007 objects to this shape in
// three places: §28 invariant 4 ("No full-record-by-default rule"), §4.4 ("avoid loading entire
// records when only a small projection is required") and §20, which names it with the word *never* --
// "query all data -> render -> trim unauthorized rows".
//
// **Measured 2026-09-29, after the first migrations rather than before them.** It started at 19;
// recentEvents took it to 18, PersonalTasks to 17, and the first declared Relation
// (ds_documents_with_steps, 007 §7.5) took all three approval screens' correlations to 11 -- the
// three sites Step 0 measured as one shape. A ratchet locks in a pattern and one migration
// is not a pattern, which is why this landed here and not with Slice A.
//
// **What it deliberately does not say**: a low count is not a composable Data Plane. Reading this
// number falling as "the work is nearly done" would be the same mistake as reading a green sweep as
// behaviour coverage.
//
// **This paragraph used to end "Ten of the remaining reads correlate two Machines ... and cannot shrink
// until 007 §7.5 Relation exists", and every clause of that was wrong by the time it was read
// (retracted 2026-09-30).** Relation had shipped -- the paragraph directly above says so, and says it
// took the count to 11 -- and `capabilities.md`'s own whole-Machine-read row had already corrected
// "ten join sites" to three by measurement. Two documents contradicted each other and the one wired to
// a gate held the stale half. That is 001 #8 in prose: the same number stated in two places is the same
// number gone stale in one.
//
// **What the remaining 11 actually are**, measured 2026-09-30 rather than inferred from the shape of
// the last sentence:
//
//   - 3 read the whole of mch_activity (approval.go, assigned.go, review.go), all feeding
//     submittersFromActivity;
//   - 1 reads the whole signature store to build an owner->image lookup (review.go, savedSignatureImages);
//   - 7 are id->scalar lookup maps over Projects, Tasks and Users (pages.go).
//
// None is a correlation, so none is Relation's to fix.
//
// **And the activity three are blocked by a declaration that is deliberately polymorphic**, which is the
// part worth carrying. submittersFromActivity needs two things: `fld_record_id IN (the Documents on
// screen)` -- set membership, where `where:` offers only equals/not_equals against a single value -- and
// the *earliest* event per record, which nothing expresses. Set membership exists in the store
// (ListRecordsByAny) but the only declarative route to it is `relations:`, and `relations:` requires
// `via` to be a reference Field pointing back, while mch_activity.fld_record_id is `text` **on purpose**:
// one Machine logs events for every Machine. Making it a `relation` is exactly what 007 §7.5 forbids --
// "reuse existing Machine reference semantics rather than inventing a second relationship identity".
//
// So: **this number will not fall again without a new primitive.** The map stays a gate against
// regression, and is not a target. `capabilities.md`'s whole-Machine-read row carries the same figures
// once; it is referenced here rather than restated, which is the lesson the retraction above cost.
// **The population was wrong until 2026-10-10 (K08), and a directive gate that under-reports is worse than none.**
// It scanned `internal/composition` only and excluded `loader.go` as "plumbing", so it reported 6 of 15 sites.
// `loader.go` holds four readers of an entire Machine (RelationOptions, BoardColumns, ConstraintRelatedRecords,
// AggregateDataset) -- the exclusion was right about the one line that *implements* the call and wrong about its
// four callers -- and the same shape sat in `internal/web` (4) and `internal/execution` (1), which nobody
// scanned. Keys are `<package>/<file>`. The 2026-09-29 and 2026-09-30 counts in the paragraphs above are composition alone.
//
// The new entries are frozen, not judged: nobody has read whether each `internal/web` site legitimately
// needs every record (a generic list or export would) or is a correlation waiting on a Dataset.
var wholeMachineReadRatchet = map[string]int{
	"composition/approval.go": 1,
	"composition/assigned.go": 1,
	"composition/loader.go":   4,
	"composition/pages.go":    2,
	"composition/review.go":   2,
	"execution/events.go":     1,
	"web/api.go":              1,
	"web/machine.go":          1,
	"web/record.go":           2,
}

// wholeMachineRead matches a call reading every record of a Machine through a Loader or a Store. The
// implementation line in composition/loader.go (`l.store.ListRecords`) is the one thing it must not count.
var wholeMachineRead = regexp.MustCompile(`\b\w+\.ListRecords\(`)

// wholeMachineReadPackages is the scanned population: every package above internal/data that may reach a
// Store or a Loader. internal/data itself defines the method and is not a caller.
var wholeMachineReadPackages = []string{"composition", "web", "execution"}

func TestWholeMachineReadsOnlyShrink(t *testing.T) {
	found := map[string]int{}
	for _, pkg := range wholeMachineReadPackages {
		dir := filepath.Join(repoRoot(), "internal", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			n := 0
			for _, line := range strings.Split(readFile(t, filepath.Join(dir, name)), "\n") {
				if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") || strings.Contains(line, "l.store.ListRecords(") {
					continue
				}
				n += len(wholeMachineRead.FindAllString(line, -1))
			}
			if n > 0 {
				found[pkg+"/"+name] = n
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no whole-Machine read found anywhere -- this gate would pass by measuring nothing")
	}

	total, budgetTotal := 0, 0
	for name, n := range found {
		total += n
		budget, listed := wholeMachineReadRatchet[name]
		switch {
		case !listed:
			t.Errorf("%s reads whole Machines %d times and is not in wholeMachineReadRatchet -- a new "+
				"file is not the way to pass. Declare a `select: records` Dataset instead (007 §7.7).", name, n)
		case n > budget:
			t.Errorf("%s: %d whole-Machine reads, up from %d. This ratchet only shrinks.", name, n, budget)
		case n < budget:
			t.Errorf("%s: %d whole-Machine reads, down from %d -- lower the entry to %d so the "+
				"improvement is locked in rather than left as room to regress.", name, n, budget, n)
		}
	}
	for name := range wholeMachineReadRatchet {
		if _, ok := found[name]; !ok {
			t.Errorf("wholeMachineReadRatchet names %s, which reads no Machine whole any more -- remove the entry", name)
		}
		budgetTotal += wholeMachineReadRatchet[name]
	}
	if total != budgetTotal {
		t.Errorf("total whole-Machine reads = %d, budgeted %d", total, budgetTotal)
	}
	t.Logf("whole-Machine reads: %d across %d files", total, len(found))
}

// TestInstalledMachinesExplainTheirNormalization is the corpus control for the Phase 4 half, the
// counterpart of TestInstalledCastsExplainWithoutDefects for the engine half.
//
// Its one defect status is StatusUndeclared on a person target, which means normalization did not run
// for that Machine -- the failure that produced metadata.Normalize in the first place.
//
// **That arm is unreachable through the loader, and mutation testing is how that was established
// rather than assumed.** Disabling Normalize in Parse does not produce an undeclared person target
// here: internal/metadata's own validation refuses the Machine first, with the three-branch message
// added 2026-09-29 ("is a person field with no related machine -- this Machine was not normalised").
// So for that derivation this gate is a second line behind a check that already holds, and the real
// coverage is metadata.TestExplain_anUnnormalisedPersonFieldIsADefect over a hand-built Machine --
// which is the only way the state can arise, since it cannot be loaded. Worth stating plainly: a
// green run here is not evidence that arm works.
//
// **StatusNotApplicable is the majority here too and must never be treated as a defect**: measured
// 2026-09-29, 18 of 31 installed Machines are referenced by nothing and 13 declare their own views:.
// Both are ordinary, correct states.
func TestInstalledMachinesExplainTheirNormalization(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	if len(wss) == 0 {
		t.Fatal("no Workspace manifests found -- this gate would pass by measuring nothing")
	}

	slugs := make([]string, 0, len(wss))
	for slug := range wss {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		counts := map[domain.ResolutionStatus]int{}
		explained := metadata.Explain(wss[slug].Machines)
		if len(explained) == 0 {
			t.Errorf("workspace %q explains no normalization at all", slug)
		}
		for _, r := range explained {
			counts[r.Status]++
			if r.IsDefect() {
				t.Errorf("workspace %q: %s is %s\n  read from: %s\n"+
					"  a normalization the runtime performs and cannot explain is 001 #6's own failure mode",
					slug, r.Name, r.Status, r.From)
			}
		}
		t.Logf("%-14s resolved=%d not-applicable=%d undeclared=%d",
			slug, counts[domain.StatusResolved], counts[domain.StatusNotApplicable], counts[domain.StatusUndeclared])
	}
}

// TestEveryNormalizationDerivationIsProduced is the other direction, and it is the one that would
// catch a constant declared and then never emitted -- a question nothing answers, which is exactly
// what the engine half's own gate exists to prevent on its side.
func TestEveryNormalizationDerivationIsProduced(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}

	// Both producers, because normalization's subjects are not all Machines. metadata.Explain answers for
	// Machines; ExplainNavigation answers for an Application's navigation, whose title inference lives on
	// domain.NavigationItem and has no Machine to hang from. Checking only the first made a correctly
	// emitted derivation fail this gate the day it was added (nav_title, 2026-10-02) -- the gate was
	// right that nobody could see it and wrong about why.
	seen := map[string]bool{}
	for _, app := range wss {
		for _, r := range append(metadata.Explain(app.Machines), metadata.ExplainNavigation(app.Workspace.Applications)...) {
			name, _, _ := strings.Cut(r.Name, ":")
			seen[strings.TrimSpace(name)] = true
		}
	}
	for _, d := range domain.NormalizationDerivations {
		if !seen[d] {
			t.Errorf("normalization derivation %q is declared but metadata.Explain never emits it over "+
				"any installed Workspace -- a question nothing answers and nobody sees", d)
		}
	}
}
