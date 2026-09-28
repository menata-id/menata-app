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
// These are two lists that can drift apart silently. domain.KnownServices is the registry; what
// validates a declared `service:` is a switch in internal/metadata with a `default` that rejects
// the unknown -- it never consults the registry. So adding a fourth member to KnownServices
// without adding its case leaves the runtime advertising a Service the loader refuses, and the
// failure surfaces only when someone writes it into a manifest. Same shape for KnownActions.
//
// Asserted behaviourally rather than by reading source: build the smallest metadata that names
// each member and confirm the loader does not reject it *for being unknown*. Other complaints
// (a Service's own required keys) are expected and ignored -- this gate is about recognition, not
// configuration.
//
// domain.KnownWorkflowEngines is deliberately absent, because it cannot have this failure: its
// validator (internal/metadata.validateWorkflowBinding) reads the registry itself rather than
// repeating its members in a switch, so there is no second list to drift from. That is the shape
// KnownActions/KnownServices would have to take to retire their half of this gate.
func TestClosedRegistryMembersAreAcceptedByTheLoader(t *testing.T) {
	for service := range domain.KnownServices {
		m := &domain.Machine{
			ID: "mch_gate_probe", Name: "Gate Probe",
			Fields: []domain.Field{{ID: "fld_probe", Name: "Probe", Type: domain.FieldTypeText}},
			Events: []domain.Event{{
				ID: "evt_probe", OnCreate: true,
				Then: domain.Service{Name: service},
			}},
		}
		err := metadata.Validate(m)
		if err != nil && strings.Contains(err.Error(), "is not a service this runtime realizes") {
			t.Errorf("domain.KnownServices declares %q, but the loader rejects it as unknown -- the registry and internal/metadata's own validation have drifted apart; add its case where the other services are handled", service)
		}
	}

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
	for service := range domain.KnownServices {
		if !usedServices[service] {
			t.Errorf("domain.KnownServices declares %q, but no installed Workspace's metadata names it in an event -- a Service invoked only from flow code is not a declared Service (006 Behavioral Model). Declare the event that triggers it, or remove it", service)
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
// submit wizard's own writes became declared. **The numbers may only go down**, on the same terms as
// the map above: too high fails, too low fails, a new file fails, and comment lines are skipped.
//
// What would move it next is the *read* side, which Stage B deliberately did not touch: signing filters
// steps by fld_decision, composition projects cards from named Fields, and both are the "a bound Machine
// must carry this vocabulary" limit that survives this stage. Stage C is the next slice with a claim on
// these numbers.
var documentApprovalFieldCoupling = map[string]int{
	"composition/approval.go":            15,
	"composition/assigned.go":            8,
	"composition/pages.go":               1,
	"composition/placement.go":           5,
	"composition/review.go":              15,
	"rendering/detail.templ":             6,
	"rendering/documentsubmit.templ":     4,
	"rendering/signatureplacement.templ": 12,
	"web/approval.go":                    10,
	"web/document.go":                    34,
	"web/record.go":                      1,
	"web/review.go":                      1,
	"web/signing.go":                     17,
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

	for _, file := range sortedFileKeys(found) {
		allowed, listed := budget[file]
		switch {
		case !listed:
			t.Errorf("%s now names Document Approval's own %s %d time(s) and is not in the frozen population -- adding an entry is not the way to pass this gate. A new capability coupled to one Application's declarations is the shape the audit's stages exist to remove", file, what, found[file])
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
