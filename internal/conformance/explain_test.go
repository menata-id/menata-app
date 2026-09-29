package conformance

import (
	"path/filepath"
	"sort"
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

// TestEveryDerivationIsOwedBySomeRole is the two-lists-drift guard. domain.Derivation* names the
// questions and WorkflowEngineSpec.Answers says which role owes each one; a derivation in the first
// list and no second is asked of nobody, so it can only ever report not-applicable -- silently, which
// is precisely the invisibility this whole slice exists to remove.
//
// The reverse direction fails too: a role owing a derivation action.ExplainCast cannot resolve would
// report undeclared forever, blaming metadata for a gap in Go.
func TestEveryDerivationIsOwedBySomeRole(t *testing.T) {
	all := []string{
		domain.DerivationDecision, domain.DerivationOpenValue, domain.DerivationOrder,
		domain.DerivationActor, domain.DerivationActorType, domain.DerivationActorGroup,
		domain.DerivationParent, domain.DerivationDocumentStatus, domain.DerivationSignaturePlacement,
		domain.DerivationSignatureStore, domain.DerivationCompositeSource,
		domain.DerivationFlowTemplate, domain.DerivationFlowTemplateStep,
	}

	owed := map[string][]string{}
	for engine, spec := range domain.KnownWorkflowEngines {
		for role, answers := range spec.Answers {
			if !contains(spec.Roles(), role) {
				t.Errorf("engine %q: Answers names role %q, which is in neither Required nor Optional", engine, role)
			}
			for _, a := range answers {
				d := a.Derivation
				if !contains(all, d) {
					t.Errorf("engine %q role %q owes %q, which is not a domain.Derivation* constant", engine, role, d)
				}
				owed[d] = append(owed[d], engine+"."+role)
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
