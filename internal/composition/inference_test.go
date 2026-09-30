package composition

import (
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/registry"
	"menata.app/internal/rendering"
)

// This file exists because mutation testing found the gap it fills. The rendering tests construct
// InferenceRow values with an explicit Tone, so they never exercise toneFor -- mapping
// `not applicable` to PillBad (the loudest tone, over the 52-of-65 majority that is correct) passed
// every test in the package. The grouping had no test either, which matters more than it sounds:
// Inference stores indexes into block.Roles precisely because storing pointers there is invalidated
// by the next append, and nothing would have caught that regression.

func inferenceCast(t *testing.T) domain.Workspace {
	t.Helper()
	doc := &domain.Machine{
		ID:             "mch_surat",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleDocument,
		Fields:         []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus}},
		Transitions:    []domain.Transition{{Field: "fld_status", From: "draft", To: "in_review"}},
	}
	step := &domain.Machine{
		ID:             "mch_langkah",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleStep,
		Fields: []domain.Field{
			{ID: "fld_putusan", Type: domain.FieldTypeStatus},
			{ID: "fld_surat", Type: domain.FieldTypeRelation, RelatedMachine: "mch_surat"},
			{ID: "fld_urutan", Type: domain.FieldTypeNumber},
			{ID: "fld_petugas", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		Transitions: []domain.Transition{
			{Field: "fld_putusan", From: "menunggu", To: "disetujui", Action: domain.ActionDecide},
		},
		Sequencing:  &domain.Sequencing{OrderField: "fld_urutan"},
		Permissions: []domain.Permission{{ID: "prm_decide", Action: domain.ActionDecide, ActorField: "fld_petugas"}},
	}
	return domain.Workspace{
		Machines: []*domain.Machine{step, doc},
		Applications: []domain.Application{{
			ID: "app_persetujuan",
			Workflow: &domain.Workflow{
				Engine: domain.WorkflowEngineDocumentApproval,
				Roles: map[string]string{
					domain.WorkflowRoleStep:     step.ID,
					domain.WorkflowRoleDocument: doc.ID,
				},
			},
		}},
	}
}

func rowFor(t *testing.T, v rendering.InferenceView, role, derivation string) rendering.InferenceRow {
	t.Helper()
	for _, e := range v.Engines {
		for _, r := range e.Roles {
			if r.Role != role {
				continue
			}
			for _, row := range r.Rows {
				if row.Derivation == derivation {
					return row
				}
			}
		}
	}
	t.Fatalf("no row for %s.%s in the view", role, derivation)
	return rendering.InferenceRow{}
}

// TestInference_toneMatchesTheStatusItReports is the assertion the mutation exposed as missing. The
// tone is not decoration: `not applicable` is the majority and correct, so giving it the same weight as
// a defect buries the two statuses that mean something -- the failure that killed two earlier attempts
// at triaging these numbers.
func TestInference_toneMatchesTheStatusItReports(t *testing.T) {
	v := Inference(inferenceCast(t))

	resolved := rowFor(t, v, "step", "decision")
	if resolved.Tone != rendering.PillGood || resolved.IsDefect {
		t.Errorf("a resolved derivation has tone %q, defect=%v; want %q and not a defect",
			resolved.Tone, resolved.IsDefect, rendering.PillGood)
	}

	// A Document is not decided -- its steps are. Correct, common, and must stay quiet.
	notApplicable := rowFor(t, v, "document", "decision")
	if notApplicable.Tone != rendering.PillMuted {
		t.Errorf("a not-applicable derivation has tone %q, want %q -- it is 52 of 65 rows and every one "+
			"of them is right, so rendering it as loudly as a defect hides the ones that matter",
			notApplicable.Tone, rendering.PillMuted)
	}
	if notApplicable.IsDefect {
		t.Error("a not-applicable derivation is reported as a defect")
	}
	if notApplicable.Value != "" {
		t.Errorf("a not-applicable derivation resolved to %q; it must not be read at all", notApplicable.Value)
	}
}

// TestInference_inputUnavailableIsWarnedAndCounted covers the /review 404's shape end to end through
// Composition: classified, toned, and counted into the summary the page leads with.
func TestInference_inputUnavailableIsWarnedAndCounted(t *testing.T) {
	ws := inferenceCast(t)
	// Drop the document role, leaving the step's relation Field declared with nothing to resolve against.
	ws.Applications[0].Workflow.Roles = map[string]string{domain.WorkflowRoleStep: "mch_langkah"}
	ws.Machines = ws.Machines[:1]

	v := Inference(ws)
	parent := rowFor(t, v, "step", "parent")
	if parent.Tone != rendering.PillWarn {
		t.Errorf("step.parent tone = %q, want %q", parent.Tone, rendering.PillWarn)
	}
	if !parent.IsDefect {
		t.Error("step.parent is not counted as a defect -- it produced a silent 404 in production for a day")
	}
	if v.Defects == 0 {
		t.Error("the view's own defect count is zero, so the page would lead with \"nothing to act on\"")
	}
}

// TestInference_groupsEveryRowUnderItsOwnRole is the regression test for the pointer-into-a-slice bug
// this function was written with and fixed before it shipped: &block.Roles[len-1] is invalidated by the
// next append, so rows would be appended into a stale backing array and disappear. On a page whose only
// job is showing what is missing, rows going missing would have been a memorable way to fail.
func TestInference_groupsEveryRowUnderItsOwnRole(t *testing.T) {
	v := Inference(inferenceCast(t))

	if len(v.Engines) != 1 {
		t.Fatalf("got %d engine blocks, want 1", len(v.Engines))
	}
	spec := registry.KnownWorkflowEngines[domain.WorkflowEngineDocumentApproval]
	if got, want := len(v.Engines[0].Roles), len(spec.Roles()); got != want {
		t.Errorf("got %d role groups, want %d (every role in the cast, including the uncast optional ones)", got, want)
	}
	for _, r := range v.Engines[0].Roles {
		if len(r.Rows) == 0 {
			t.Errorf("role %q has no rows -- a group that lost its rows is the slice-pointer bug", r.Role)
		}
		for _, row := range r.Rows {
			if row.Derivation == "" {
				t.Errorf("role %q has a row with no derivation name", r.Role)
			}
			if row.Status == "" {
				t.Errorf("role %q's %q has no status, so the page cannot tone it", r.Role, row.Derivation)
			}
		}
	}
}

// TestInference_uncastOptionalRoleIsNotADefect: a Workspace that installed no signature store is a
// smaller legitimate installation, and the page must say so without claiming something is wrong.
func TestInference_uncastOptionalRoleIsNotADefect(t *testing.T) {
	v := Inference(inferenceCast(t))
	for _, e := range v.Engines {
		for _, r := range e.Roles {
			if r.Role != domain.WorkflowRoleSignature {
				continue
			}
			if r.MachineID != "" {
				t.Fatalf("the fixture casts a signature Machine (%s); this test needs it uncast", r.MachineID)
			}
			if r.Defects != 0 {
				t.Errorf("an uncast optional role reports %d defects", r.Defects)
			}
			if r.Required {
				t.Error("the signature role is reported as required")
			}
			return
		}
	}
	t.Fatal("no signature role group in the view -- an uncast role must still be stated")
}

// TestInference_skipsAnEngineNoApplicationBinds: a table of "not applicable" for a feature the
// Workspace never installed is noise of exactly the kind Resolution.Status exists to suppress.
func TestInference_skipsAnEngineNoApplicationBinds(t *testing.T) {
	v := Inference(domain.Workspace{})
	if len(v.Engines) != 0 {
		t.Errorf("got %d engine blocks for a Workspace with nothing installed, want 0", len(v.Engines))
	}
	if v.Defects != 0 {
		t.Errorf("got %d defects for a Workspace with nothing installed", v.Defects)
	}
}
