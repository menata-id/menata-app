package action

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// stepMachine and docMachine are the two-Machine cast every case here needs: a step declaring all
// nine derivations its role owes, and the Document it points at.
func explainFixtures() (*domain.Machine, *domain.Machine) {
	doc := &domain.Machine{
		ID:             "mch_surat",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleDocument,
		Fields:         []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus}},
		Transitions: []domain.Transition{
			{Field: "fld_status", From: "draft", To: "in_review"},
		},
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
			{ID: "fld_jenis", Type: domain.FieldTypeText},
			{ID: "fld_grup", Type: domain.FieldTypeRelation, RelatedMachine: "mch_group"},
			{ID: "fld_ttd", Type: domain.FieldTypeFile},
			{ID: "fld_hlm", Type: domain.FieldTypeNumber},
			{ID: "fld_x", Type: domain.FieldTypeNumber},
			{ID: "fld_y", Type: domain.FieldTypeNumber},
			{ID: "fld_lebar", Type: domain.FieldTypeNumber},
		},
		Transitions: []domain.Transition{
			{Field: "fld_putusan", From: "menunggu", To: "disetujui", Action: domain.ActionDecide},
			{Field: "fld_putusan", From: "menunggu", To: "ditolak", Action: domain.ActionDecide},
		},
		Sequencing: &domain.Sequencing{OrderField: "fld_urutan"},
		Permissions: []domain.Permission{{
			ID:         "prm_decide",
			Action:     domain.ActionDecide,
			ActorField: "fld_petugas",
			DynamicActor: &domain.DynamicActorGate{
				ActorTypeField:  "fld_jenis",
				ActorGroupField: "fld_grup",
			},
		}},
		SignaturePlacement: &domain.SignaturePlacement{
			ImageField: "fld_ttd", PageField: "fld_hlm", XField: "fld_x", YField: "fld_y", WidthField: "fld_lebar",
		},
		Events: []domain.Event{{
			Then: domain.Service{
				Name:      domain.ServiceCompositeSignedDocument,
				Composite: &domain.Composite{SourceField: "fld_berkas"},
			},
		}},
	}
	return step, doc
}

func explainWorkspace(machines ...*domain.Machine) domain.Workspace {
	roles := map[string]string{}
	for _, m := range machines {
		if m.WorkflowRole != "" {
			roles[m.WorkflowRole] = m.ID
		}
	}
	return domain.Workspace{
		Machines: machines,
		Applications: []domain.Application{{
			ID:       "app_persetujuan",
			Workflow: &domain.Workflow{Engine: domain.WorkflowEngineDocumentApproval, Roles: roles},
		}},
	}
}

func find(t *testing.T, rs []domain.Resolution, name string) domain.Resolution {
	t.Helper()
	for _, r := range rs {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no resolution named %q in %d results", name, len(rs))
	return domain.Resolution{}
}

// TestExplainCast_resolvesEveryDerivationTheRoleOwes is the baseline: a complete cast, under names
// none of Document Approval's own, explains every question with a value and a source.
func TestExplainCast_resolvesEveryDerivationTheRoleOwes(t *testing.T) {
	step, doc := explainFixtures()
	got := ExplainCast(explainWorkspace(step, doc), domain.WorkflowEngineDocumentApproval, "app_persetujuan")

	want := map[string]string{
		"step.decision":            "fld_putusan",
		"step.open_value":          "menunggu",
		"step.order":               "fld_urutan",
		"step.actor":               "fld_petugas",
		"step.actor_type":          "fld_jenis",
		"step.actor_group":         "fld_grup",
		"step.parent":              "fld_surat",
		"step.composite_source":    "fld_berkas",
		"document.document_status": "fld_status",
	}
	for name, value := range want {
		r := find(t, got, name)
		if r.Status != domain.StatusResolved || r.Value != value {
			t.Errorf("%s = %q (%s), want %q resolved", name, r.Value, r.Status, value)
		}
		if r.From == "" {
			t.Errorf("%s resolved to %q but names no declaration it was read from -- "+
				"the source is what makes an answer checkable rather than trusted (001 #6)", name, r.Value)
		}
	}

	placement := find(t, got, "step.signature_placement")
	if placement.Status != domain.StatusResolved || !strings.Contains(placement.Value, "fld_lebar") {
		t.Errorf("step.signature_placement = %q (%s), want all five Fields resolved", placement.Value, placement.Status)
	}
}

// TestExplainCast_distinguishesTheReviewDefectFromAnUndeclaredBlock is the test this whole slice
// exists for. Both of these resolve to "" and they are different defects; before Resolution.Status
// nothing could tell them apart, and each one shipped.
func TestExplainCast_distinguishesTheReviewDefectFromAnUndeclaredBlock(t *testing.T) {
	step, doc := explainFixtures()

	// The /review 404's shape: the step's relation Field exists and is declared, but the caller had
	// no document Machine to resolve it against. Reproduced by casting no document role at all.
	t.Run("input unavailable, not undeclared", func(t *testing.T) {
		ws := explainWorkspace(step) // doc deliberately absent from the cast
		got := ExplainCast(ws, domain.WorkflowEngineDocumentApproval, "app_persetujuan")

		parent := find(t, got, "step.parent")
		if parent.Status != domain.StatusInputUnavailable {
			t.Errorf("step.parent status = %s, want %s\n"+
				"  this is the /review 404: fld_surat IS declared, the caller had no document Machine.\n"+
				"  Reporting it as %s would send a reader to look for a missing declaration that is not missing",
				parent.Status, domain.StatusInputUnavailable, domain.StatusUndeclared)
		}
		if !parent.IsDefect() {
			t.Error("step.parent must count as a defect: it silently produced a 404 in production for a day")
		}
		if !strings.Contains(parent.From, "no document Machine") {
			t.Errorf("step.parent From = %q, want it to name the missing input", parent.From)
		}
	})

	// Stage E1's shape: the role is cast, the Machine is present, and it declares nothing for a
	// question its role owes.
	t.Run("undeclared, not not-applicable", func(t *testing.T) {
		bare := &domain.Machine{
			ID: "mch_template", WorkflowEngine: domain.WorkflowEngineDocumentApproval,
			WorkflowRole: domain.WorkflowRoleFlowTemplate,
		}
		ws := explainWorkspace(step, doc, bare)
		got := ExplainCast(ws, domain.WorkflowEngineDocumentApproval, "app_persetujuan")

		ft := find(t, got, "flow_template.flow_template")
		if ft.Status != domain.StatusUndeclared {
			t.Errorf("flow_template.flow_template status = %s, want %s\n"+
				"  this is Stage E1: the role is cast and the Machine declares no flow_template: block,\n"+
				"  which is what would have saved an approval flow with no approvers",
				ft.Status, domain.StatusUndeclared)
		}
		if !ft.IsDefect() {
			t.Error("an undeclared block on a cast role must count as a defect")
		}
	})
}

// TestExplainCast_marksAnotherRolesQuestionNotApplicable holds the larger half. 38 of 50 derivations
// on cast Machines are empty and correct; if they came back as defects this surface would be noise,
// which is what killed the first attempt at triaging these numbers.
func TestExplainCast_marksAnotherRolesQuestionNotApplicable(t *testing.T) {
	step, doc := explainFixtures()
	got := ExplainCast(explainWorkspace(step, doc), domain.WorkflowEngineDocumentApproval, "app_persetujuan")

	// A Document is not decided -- its steps are. Asking a Document for its decision Field is not a
	// gap in the Document.
	for _, name := range []string{"document.decision", "document.order", "document.actor", "document.open_value"} {
		r := find(t, got, name)
		if r.Status != domain.StatusNotApplicable {
			t.Errorf("%s status = %s, want %s: that question belongs to the step role", name, r.Status, domain.StatusNotApplicable)
		}
		if r.IsDefect() {
			t.Errorf("%s must not count as a defect", name)
		}
		if r.Value != "" {
			t.Errorf("%s = %q, want empty: a not-applicable question must not be read at all", name, r.Value)
		}
	}

	// An uncast optional role is a smaller, legitimate installation, not a defect.
	sigStore := find(t, got, "signature.signature_store")
	if sigStore.Status != domain.StatusNotApplicable || sigStore.IsDefect() {
		t.Errorf("signature.signature_store = %s, want %s and not a defect: casting no signature store is legitimate",
			sigStore.Status, domain.StatusNotApplicable)
	}
}

// TestExplainCast_reportsAPartlyDeclaredBlockAsUndeclared: a signature placement with an image and no
// coordinates would stamp at (0,0). Half an answer is not a smaller feature.
func TestExplainCast_reportsAPartlyDeclaredBlockAsUndeclared(t *testing.T) {
	step, doc := explainFixtures()
	step.SignaturePlacement = &domain.SignaturePlacement{ImageField: "fld_ttd", PageField: "fld_hlm"}

	got := ExplainCast(explainWorkspace(step, doc), domain.WorkflowEngineDocumentApproval, "app_persetujuan")
	r := find(t, got, "step.signature_placement")
	if r.Status != domain.StatusUndeclared {
		t.Errorf("a placement missing x/y/width = %s (%q), want %s: reporting it resolved because two "+
			"Fields answered is the half-truth this type exists to remove", r.Status, r.Value, domain.StatusUndeclared)
	}
}

// TestExplainCast_unknownEngineExplainsNothing -- an engine outside the closed registry has no cast to
// explain, and inventing one would be the silent fallback every accessor here refuses.
func TestExplainCast_unknownEngineExplainsNothing(t *testing.T) {
	step, doc := explainFixtures()
	if got := ExplainCast(explainWorkspace(step, doc), "tidak_ada", ""); got != nil {
		t.Errorf("ExplainCast over an unknown engine returned %d resolutions, want none", len(got))
	}
}

// TestExplainCast_isDeterministic holds 007 §4.6, which states it as a MUST: "Plan construction must
// not depend on map iteration order, incidental database ordering, renderer side effects, or
// non-deterministic capability discovery."
//
// ExplainCast satisfies it *by construction* -- its output order comes from two slices,
// WorkflowEngineSpec.Roles() and allDerivations -- and until this test nothing held that. Both
// KnownWorkflowEngines and WorkflowEngineSpec.Answers are maps, so one refactor that ranges over
// either to build the output would pass every other test here while making a diagnostics page
// reorder itself between refreshes. On a surface whose entire job is being compared against itself
// (before a change, after a change, one Workspace against another) that is not cosmetic: a reader
// diffing two runs would see noise and stop trusting it.
func TestExplainCast_isDeterministic(t *testing.T) {
	step, doc := explainFixtures()
	ws := explainWorkspace(step, doc)

	first := ExplainCast(ws, domain.WorkflowEngineDocumentApproval, "app_persetujuan")
	if len(first) == 0 {
		t.Fatal("ExplainCast returned nothing -- this test would pass by measuring an empty slice")
	}

	// Repeated rather than compared against a frozen list: a hardcoded expectation would have to be
	// edited every time a derivation is added, and would then be testing the edit rather than the
	// property. Go randomizes map iteration per range, so a map-ordered implementation fails this
	// within a few rounds.
	for round := 0; round < 50; round++ {
		again := ExplainCast(ws, domain.WorkflowEngineDocumentApproval, "app_persetujuan")
		if len(again) != len(first) {
			t.Fatalf("round %d returned %d resolutions, first returned %d", round, len(again), len(first))
		}
		for i := range first {
			if again[i] != first[i] {
				t.Fatalf("round %d differs at index %d:\n  first: %+v\n  again: %+v\n"+
					"  007 §4.6 makes deterministic construction a MUST -- something here ranges over a map",
					round, i, first[i], again[i])
			}
		}
	}
}
