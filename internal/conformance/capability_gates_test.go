package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

// Package capability_gates_test.go holds the three gates that keep a *new* capability from
// arriving as Go the runtime alone can reach (owner request, 2026-09-28, after the audit in
// menata-app-document's audits/2026-09-28-kajian-metadata-based-document-approval.md).
//
// What a gate can and cannot do here is worth stating once, because the distinction decides which
// gates exist. "This capability should have been metadata" is not decidable -- every runtime
// capability *is* Go, and only its seam is metadata. A gate can only recognise a named shape. So
// these three name shapes rather than intent:
//
//   - a closed registry member the loader would reject (registry and validator disagreeing);
//   - a closed registry member no metadata ever activates (a capability reachable only from Go);
//   - growth in the amount of code coupled to one Application's own Machines.
//
// The third is a ratchet, which is this repo's answer to everything the first two cannot see: it
// cannot tell a good new reference from a bad one, but it can refuse to let the population grow,
// and that turns "do not hardcode for one application" into something a machine checks without
// judging anyone's intent.
//
// One sequencing rule learned the same day, recorded because it is the thing that makes gates
// useful rather than obstructive: **a gate locks in progress, it does not create it.** Forbidding
// a shape before the declarative alternative exists forbids the only way anyone has. The gap the
// audit calls A -- an Action cannot declare which Fields it writes -- is therefore deliberately
// *not* gated here: until that primitive exists, writing it in Go is correct.
// TestNoBareMachineIDIdentityChecks could only be written on 2026-09-27 after all 23 call sites
// were converted, and this file follows the same order.

// TestClosedRegistryMembersAreAcceptedByTheLoader: every Action and Service the runtime declares
// must be one the loader actually accepts in metadata.
//
// These are two lists that can drift apart silently: a registry of names, and a `switch` in
// internal/metadata with a `default` that rejects the unknown and never consults that registry. Adding a
// member without adding its case leaves the runtime advertising something the loader refuses, and the
// failure surfaces only when someone writes it into a manifest.
//
// Asserted behaviourally rather than by reading source: build the smallest metadata that names
// each member and confirm the loader does not reject it *for being unknown*. Other complaints
// (a member's own required keys) are expected and ignored -- this gate is about recognition, not
// configuration.
//
// domain.KnownWorkflowEngines is deliberately absent, because it cannot have this failure: its
// validator (internal/metadata.validateWorkflowBinding) reads the registry itself rather than
// repeating its members in a switch, so there is no second list to drift from.
//
// **Services left this gate on 2026-09-30 by taking exactly that shape, which the paragraph above used to
// name as the condition for retiring their half.** internal/registry.Services now maps each Service name
// to its own validator and internal/metadata calls registry.ValidateService, so the registry *is* the
// validation -- there is no second list, and a probe asserting the loader accepts what the registry
// declares would be asserting that a map contains its own keys. domain.KnownServices is deleted.
//
// What remains here is KnownActions, which still has the two-list shape. The gate's own prediction coming
// true is the useful part: a gate that names the condition for its own retirement can be retired on
// evidence instead of on taste.
func TestClosedRegistryMembersAreAcceptedByTheLoader(t *testing.T) {
	for action := range domain.KnownActions {
		m := &domain.Machine{
			ID: "mch_gate_probe", Name: "Gate Probe",
			Fields:      []domain.Field{{ID: "fld_probe", Name: "Probe", Type: domain.FieldTypeText}},
			Permissions: []domain.Permission{{ID: "prm_probe", Action: action, ActorField: "fld_probe"}},
		}
		err := metadata.Validate(m)
		if err != nil && strings.Contains(err.Error(), "unknown action") {
			t.Errorf("domain.KnownActions declares %q, but the loader rejects it as unknown -- the registry and internal/metadata's own validation have drifted apart", action)
		}
	}
}

// TestClosedRegistryMembersAreActivatedByMetadata: every Action and Service must be reachable by
// *declaring* it, not only by calling it from Go.
//
// This is the gate that encodes the owner's own requirement -- "dengan konfigurasi dan aktivasi
// dari metadata" -- as something checkable. A capability that exists in the registry, is accepted
// by the loader, and yet appears in no manifest anywhere is a capability only Go can start, which
// is 001 #3 inverted: the application would be evolving by changing source, not metadata.
//
// domain.KnownWorkflowEngines joined the two original registries on 2026-09-28 (Stage A), and is the
// clearest case of what this gate is for: the approval engine existed for weeks with *no* metadata
// seam at all -- it selected its own Machines by matching literals -- so there was nothing a manifest
// could have named. An engine back in that state would pass every other test in this repo.
//
// The audit's Gap B is exactly this shape caught early: signature/PDF compositing is invoked from
// flow code and named by no `events:` block, so when it becomes a Service it must arrive with the
// declaration that activates it rather than a fourth registry line and another direct call.
//
// Read from the real installed Workspaces, so a member kept alive only by a fixture does not count.
func TestClosedRegistryMembersAreActivatedByMetadata(t *testing.T) {
	installed, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}

	usedActions, usedServices := map[string]bool{}, map[string]bool{}
	for _, ws := range installed {
		for _, m := range ws.Machines {
			for _, p := range m.Permissions {
				usedActions[p.Action] = true
			}
			for _, tr := range m.Transitions {
				if tr.Action != "" {
					usedActions[tr.Action] = true
				}
			}
			for _, e := range m.Events {
				usedServices[e.Then.Name] = true
			}
		}
	}

	usedEngines := map[string]bool{}
	for _, ws := range installed {
		for _, app := range ws.Workspace.Applications {
			if app.Workflow != nil {
				usedEngines[app.Workflow.Engine] = true
			}
		}
	}
	for engine := range domain.KnownWorkflowEngines {
		if !usedEngines[engine] {
			t.Errorf("domain.KnownWorkflowEngines declares %q, but no installed Workspace's Application binds it in a workflow: block -- an engine no Application can name is one only Go can start, which is the shape Stage A removed (see workflow_binding_test.go). Declare the binding where the engine is meant to run, or remove it", engine)
		}
	}

	for action := range domain.KnownActions {
		if !usedActions[action] {
			t.Errorf("domain.KnownActions declares %q, but no installed Workspace's metadata names it in a permission or transition -- a capability only Go can reach is not activated by metadata (001 #3). Declare it where it is meant to be used, or remove it", action)
		}
	}
	for service := range registry.Services {
		if !usedServices[service] {
			t.Errorf("registry.Services declares %q, but no installed Workspace's metadata names it in an event -- a Service invoked only from flow code is not a declared Service (006 Behavioral Model). Declare the event that triggers it, or remove it", service)
		}
	}
}

// documentApprovalCoupling is the frozen population for the ratchet below: how many times each
// file outside internal/action names one of Document Approval's own Machine-id constants.
//
// **The numbers may only go down.** A count that is too high fails (something new is coupled to
// this one Application); a count that is too low also fails, so a file that improved gets its
// number lowered here instead of quietly leaving room to regress. Removing a file's last reference
// means removing its entry.
//
// Counted per file rather than per line, and per reference rather than per file-presence, on
// purpose: a per-line budget breaks on unrelated refactors, and a per-file boolean cannot see a
// new hardcoded reference added to a file that already had one -- which is how this kind of
// coupling actually grows.
//
// Frozen 2026-09-28 at 67 references across 18 files, the same day the audit measured 411 lines of
// metadata against 3,366 of Application-specific Go and templ -- **and emptied the same day**, by
// the slice after Stage A that made a Machine resolvable by the role its Application casts it in.
// The map stays declared, because an empty ratchet is an ordinary gate rather than a retired one:
// the next file to name one of these constants fails, which is the whole point.
//
// Two lessons from that emptying, both worth keeping:
//
// Comment lines are skipped (as TestNoBareMachineIDIdentityChecks already does). Five references
// survive in comments that explain the shape they replaced -- exactly what CLAUDE.md asks an author
// to leave behind -- and counting those would have made the gate reward deleting the explanation.
//
// It measures *naming* coupling, not identity coupling. Stage A removed the latter and moved this
// number not at all, which is correct: identity had already been funnelled into IsDocument/IsStep on
// 2026-09-27, where a different gate holds it. Ask which kind a change addresses before predicting
// this number.
var documentApprovalCoupling = map[string]int{}

var documentApprovalConstant = regexp.MustCompile(`action\.(Document|Step|Signature|Template|TemplateStep)MachineID`)

// documentApprovalFieldCoupling is the second frozen population, and the one Stage B earned the right
// to gate: how many times each file outside internal/action names one of Document Approval's own
// *Field* ids.
//
// It exists because the map above could not see the thing Stage B removed. That one counts Machine-id
// constants; an Action's effect is about Field ids, so "Stage B will make the ratchet drop" -- which
// this repo's own ROADMAP claimed -- was false as the gate stood. Extending the regex instead of
// asserting the claim is what makes it checkable.
//
// Frozen 2026-09-28 at 129 references across 13 files, immediately after `decide`, `revise` and the
// submit wizard's own writes became declared; 126 across 14 when Stage C moved the compositing operation
// out of internal/web (a relocation adds a file and subtracts from another, which is why the sweep below
// carries a total as well as per-file numbers); and **78 across 11 by the end of that day**, when the
// behaviour planes stopped naming Fields that metadata already declares elsewhere and started deriving
// them (action.DeclaredFields).
//
// **43 across 7 files** since Stage D the same day, which closed the second of the two shapes this
// comment listed as "not a derivation away". That entry read: *the signature store's own shape (the
// placement Fields, fld_owner/fld_image on mch_signature) -- nothing declares these, so removing them
// is a capability question rather than a reading one.* It was right, and the answer was to build the
// capability: `signature_placement:` and `signature_store:` (domain.SignaturePlacement), two
// Machine-level blocks naming which Fields hold a signature and where it sits. Five files left the map
// entirely.
//
// **Stage D also moved 12 write-side bindings**, which is worth stating precisely because the other
// shape above is still open. `signatureplacement.templ`'s four `name={ action.Field… }` attributes now
// render ids Composition resolved from that declaration -- so those particular names are declared. The
// *general* primitive is not built: a form Field declared in metadata (007 §11.3 Binding) still does
// not exist, and its trigger is still a third bespoke write screen (ROADMAP.md's deferral table). A
// screen can still leave this map with every binding hand-typed, which is exactly what the remaining
// entries are.
//
// **The numbers may only go down**, on the same terms as the map above: too high fails, too low fails,
// a new file fails, and comment lines are skipped.
//
// **29 across 7 files** since Stage E1 the same day, which took the derivable half of the submit
// wizard: the Document's status Field from its own state model, the ordering mode from the step
// Machine's `sequencing:`, the actor and its gate from the Permission, the relation from
// DeclaredFields, the PDF from the compositing Event's `source_field`.
//
// **The prediction was 16 and the measurement is 20**, and the four are the interesting part. The
// wizard shares stepRowValues between a real Approval Step and a *saved flow template* step, and
// mch_approval_flow_template_step declares no `decide` Permission, no actor gate and no `sequencing:`
// -- it is a template, nothing decides it -- so DeclaredFields returns every id empty for it. Passing
// that through would have written four values under the empty key and produced a flow template with no
// approvers; checked with a probe rather than reasoned about. So the template path now names its own
// Fields explicitly (`templateStepFields`) where it used to borrow the real step's constants by
// coincidence -- four references that did not exist before, and a more honest file.
//
// That is the flow-template shape gap, and it is Stage D's exactly: the roles are cast
// (`flow_template`/`flow_template_step`), the Fields they hold are declared nowhere. ROADMAP.md's
// Stage E2.
//
// **11 across 7 files** since Stage E2 (2026-09-29), which closed exactly that gap:
// `flow_template:`/`flow_template_step:` (domain.FlowTemplate) declare the Fields of a saved approval
// flow, and `web/document.go` went 20 -> 2.
//
// **Its six row keys are not a second source of truth**, which is the stage's own lesson. They ask what
// `sequencing:` and the `decide` Permission answer for the *live* step Machine -- but a template is
// never decided, so it declares neither, and the probe above is why we know deriving from them returns
// every id empty. Two Machines answering for themselves is not duplication; declaring `sequencing:` on
// a template to make one derivation serve both would assert locking behaviour it does not have.
//
// **9 across 5 files since 2026-09-29**, and the drop is worth reading carefully because an earlier
// version of this comment was wrong about what held the number.
//
// It claimed the three hand-written steps-by-document correlations were holding it, and predicted the
// Relation primitive would move it. Relation shipped (007 §7.5, all three sites) and the number did
// **not** move: the correlations were never Field-id coupling. Measured properly, the 11 were a step's
// label (5), a Document's file (2) and status (1), and three form bindings.
//
// What moved it was two accessors that already existed. `composition/review.go` reads the Document's
// file Field from the compositing Event's own `source_field` (action.CompositeFields) and
// `composition/pages.go` reads its status Field from the Machine's own transitions
// (docMachine.StatusField()) -- both derivations available for a week, neither used here. **Finding a
// declaration that already answers the question beat building a new primitive**, which is the order 001
// #8 asks for and the order this entry got backwards.
//
// **4 across 2 files since 2026-09-29**, and the last drop was a *deletion* rather than a derivation.
//
// A step's label held 5 of the 9, and the question had been "where should the declaration live".
// Measuring the Field answered a different question: `fld_step_name` was empty in **0 of 23 records**,
// including a real submission made that morning with the wizard's own "(optional)" input on screen.
// Board 08 -- the wizard's own mockup -- collects no step name and says why ("Each step is one person
// or a whole group"), and what boards 09 and 10 draw in that column is the approver's job title
// ("Director", "Finance") or a Group's size ("4 members"). The Field was a stand-in for identity data,
// invented here rather than asked for by the design, so it was deleted along with both constants,
// `stepLabel`, the wizard input, and the flow-template's own copy of it. No data migration: no record
// held the key.
//
// `card_fields` with a `title` role had been investigated as its home first and rejected -- the
// vocabulary is right (007 §7.6) but this runtime wires `card_fields` to a chip renderer, and the label
// was already rendered. That investigation is what led to measuring the Field, which is the order that
// worked: **the declaration nobody could place turned out to be a Field nobody filled.**
//
// **1 in 1 file since 2026-09-29**, and the 3 that left were the entry above being wrong about them.
//
// It called `documentsubmit.templ`'s three `name=` attributes "007 §11.3 Binding, primitive unbuilt",
// which read as a fair deferral and was not: the *read* side of that same form
// (`internal/web.parseStepInputs`) had derived those three names from `action.DeclaredFields` since the
// day it was written, and `composition.buildPlacement` was already handing board 09 its four written
// names as `rendering.PlacementFields`. So the primitive existed, twice, and the shape to copy was in
// the neighbouring file -- what was missing was a `StepFields` struct and a parameter.
//
// Worse than a stale deferral: a comment in `internal/web/document.go` asserted the two ends of this
// form read one declaration, and an edit that same morning strengthened it to "**Every** input name".
// Both were false while three attributes were `action.FieldStep*` literals, and the gate could not see
// it because a `.templ` naming a constant is what this map *counts*, not what it forbids. The number
// being frozen is what made the prose look settled.
//
// **A deferral is a measurement with an expiry date.** "Primitive unbuilt" is checkable -- grep for the
// accessor -- and this one was checked only when someone asked whether the remainder could be finished.
//
// What is left is `rendering/detail.templ` (1): it compares a Field id to decide whether to draw a page
// thumbnail, and the derived Field would have to be threaded into an already ten-parameter page
// signature. That is a props decision, not a missing primitive -- said that way so the next reader does
// not inherit the same "unbuilt" framing this entry just had to retract.
var documentApprovalFieldCoupling = map[string]int{
	"rendering/detail.templ": 1,
}

var documentApprovalFieldConstant = regexp.MustCompile(`action\.Field[A-Za-z]+`)

func TestDocumentApprovalCouplingOnlyShrinks(t *testing.T) {
	assertCouplingOnlyShrinks(t, documentApprovalConstant, documentApprovalCoupling, "Machine ids")
}

// TestDocumentApprovalFieldCouplingOnlyShrinks is the Field-id half, gateable only since Stage B gave
// an Action a way to declare what it writes -- build the primitive, migrate the uses, *then* gate.
func TestDocumentApprovalFieldCouplingOnlyShrinks(t *testing.T) {
	assertCouplingOnlyShrinks(t, documentApprovalFieldConstant, documentApprovalFieldCoupling, "Field ids")
}

// assertCouplingOnlyShrinks is the shared sweep. The map is authoritative in both directions -- a count
// that rose fails, a count that fell fails until it is lowered, a new file fails, a vanished file fails
// -- so every change to the population is a change to this file, which is what "locked in rather than
// left as room to regress" means.
//
// The **total** is carried alongside it for one reason, learned on 2026-09-28 when Stage C moved the
// compositing operation from internal/web into internal/execution: a relocation adds a file and removes
// references elsewhere, and the "a new file is not the way to pass" message is exactly wrong advice for
// it. The gate is no laxer -- a relocation still fails until the map is updated -- but when the total
// fell it says so, and says what to do. Growth still reads as growth.
func assertCouplingOnlyShrinks(t *testing.T, pattern *regexp.Regexp, budget map[string]int, what string) {
	t.Helper()
	found := map[string]int{}
	for _, pkg := range []string{"internal/web", "internal/composition", "internal/rendering", "internal/execution"} {
		dir := filepath.Join(repoRoot(), pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") {
				continue
			}
			if !strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".templ") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s/%s: %v", pkg, name, err)
			}
			n := 0
			for _, line := range strings.Split(string(src), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue // a comment may name the old shape while explaining it
				}
				n += len(pattern.FindAllString(line, -1))
			}
			if n > 0 {
				found[strings.TrimPrefix(pkg, "internal/")+"/"+name] = n
			}
		}
	}

	foundTotal, budgetTotal := 0, 0
	for _, n := range found {
		foundTotal += n
	}
	for _, n := range budget {
		budgetTotal += n
	}
	relocation := foundTotal < budgetTotal

	for _, file := range sortedFileKeys(found) {
		allowed, listed := budget[file]
		switch {
		case !listed && relocation:
			t.Errorf("%s now names Document Approval's own %s %d time(s) and is not in the frozen population -- the total fell (%d -> %d), so this looks like a relocation rather than new coupling: add this entry and lower the one it moved out of, in this change", file, what, found[file], budgetTotal, foundTotal)
		case !listed:
			t.Errorf("%s now names Document Approval's own %s %d time(s) and is not in the frozen population, and the total did not fall (%d -> %d) -- adding an entry is not the way to pass this gate. A new capability coupled to one Application's declarations is the shape the audit's stages exist to remove", file, what, found[file], budgetTotal, foundTotal)
		case found[file] > allowed:
			t.Errorf("%s names Document Approval's own %s %d time(s), up from %d -- this population may only shrink", file, what, found[file], allowed)
		case found[file] < allowed:
			t.Errorf("%s is down to %d reference(s) from %d -- lower its entry so the improvement is locked in rather than left as room to regress", file, found[file], allowed)
		}
	}
	for _, file := range sortedIntKeys(budget) {
		if _, still := found[file]; !still {
			t.Errorf("%s no longer names any of Document Approval's %s -- remove its entry from the frozen population", file, what)
		}
	}
}

func sortedFileKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedIntKeys(m map[string]int) []string { return sortedFileKeys(m) }
