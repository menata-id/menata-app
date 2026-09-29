package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
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
// is why domain.WorkflowEngineSpec.Answers exists at all.
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
		for engine := range domain.KnownWorkflowEngines {
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
	for engine, spec := range domain.KnownWorkflowEngines {
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

	for _, d := range all {
		if len(owed[d]) == 0 {
			t.Errorf("derivation %q is owed by no role in any engine -- it can only ever report "+
				"not-applicable, which makes it a question nothing answers and nobody sees", d)
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
