package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func explainFixture() []*domain.Machine {
	user := &domain.Machine{ID: domain.UserMachineID, Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}}}
	task := &domain.Machine{
		ID: "mch_task",
		Fields: []domain.Field{
			{ID: "fld_title", Type: domain.FieldTypeText},
			{ID: "fld_assignee", Type: domain.FieldTypePerson},
			{ID: "fld_project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"},
		},
		Views: []domain.View{{ID: "vw_board", Name: "Board", Type: domain.ViewBoard, GroupBy: "fld_title"}},
	}
	project := &domain.Machine{ID: "mch_project", Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}}}
	return []*domain.Machine{Normalize(user), Normalize(task), Normalize(project)}
}

func find(t *testing.T, rs []domain.Resolution, substr string) domain.Resolution {
	t.Helper()
	for _, r := range rs {
		if strings.Contains(r.Name, substr) {
			return r
		}
	}
	t.Fatalf("no resolution whose name contains %q in %d results", substr, len(rs))
	return domain.Resolution{}
}

// TestExplain_personTargetIsReportedAsInferred: nothing in the YAML says a person Field points at
// mch_user, so this is the one genuine inference of the three.
func TestExplain_personTargetIsReportedAsInferred(t *testing.T) {
	got := Explain(explainFixture())

	r := find(t, got, domain.DerivationPersonTarget+": mch_task.fld_assignee")
	if r.Status != domain.StatusResolved || r.Value != domain.UserMachineID {
		t.Errorf("person target = %q (%s), want %s resolved", r.Value, r.Status, domain.UserMachineID)
	}
	if !strings.Contains(r.From, "expand authoring conveniences") {
		t.Errorf("From = %q, want it to name the Phase 4 step", r.From)
	}
}

// TestExplain_anUnnormalisedPersonFieldIsADefect. This is the failure that produced
// metadata.Normalize: a Machine built in Go with a correct person Field failed Validate with a
// message about its *type*. The surface has to call it what it is.
func TestExplain_anUnnormalisedPersonFieldIsADefect(t *testing.T) {
	raw := &domain.Machine{ID: "mch_raw", Fields: []domain.Field{{ID: "fld_owner", Type: domain.FieldTypePerson}}}
	got := Explain([]*domain.Machine{raw}) // deliberately not Normalized

	r := find(t, got, domain.DerivationPersonTarget+": mch_raw.fld_owner")
	if r.Status != domain.StatusUndeclared {
		t.Errorf("an unnormalised person Field is %s, want %s -- it means normalization did not run",
			r.Status, domain.StatusUndeclared)
	}
	if !r.IsDefect() {
		t.Error("an unnormalised person Field must count as a defect")
	}
}

// TestExplain_childCollectionsNameTheirFieldAndTheChain. Per pair, not per Machine, and a pair that
// exists because of *another* derivation says so -- all eight of mch_user's real child collections
// come from person Fields, so if Normalize stops running mch_user silently loses every one.
func TestExplain_childCollectionsNameTheirFieldAndTheChain(t *testing.T) {
	got := Explain(explainFixture())

	viaPerson := find(t, got, domain.DerivationChildCollection+": "+domain.UserMachineID+" <- mch_task.fld_assignee")
	if viaPerson.Status != domain.StatusResolved {
		t.Errorf("mch_user's child collection via a person Field is %s, want resolved", viaPerson.Status)
	}
	if !strings.Contains(viaPerson.From, domain.DerivationPersonTarget) {
		t.Errorf("From = %q, want it to name the derivation this one depends on -- a person Field's "+
			"target is itself inferred, so this collection exists only because Normalize ran", viaPerson.From)
	}

	viaRelation := find(t, got, domain.DerivationChildCollection+": mch_project <- mch_task.fld_project")
	if strings.Contains(viaRelation.From, domain.DerivationPersonTarget) {
		t.Error("a declared relation's child collection claims to depend on the person inference")
	}
}

// TestExplain_aMachineNothingReferencesIsNotApplicable: 18 of 31 installed Machines are in this
// state. Omitting them would make a correct absence indistinguishable from a failed derivation.
func TestExplain_aMachineNothingReferencesIsNotApplicable(t *testing.T) {
	got := Explain(explainFixture())

	r := find(t, got, domain.DerivationChildCollection+": mch_task")
	if r.Status != domain.StatusNotApplicable || r.IsDefect() {
		t.Errorf("a Machine nothing references is %s (defect=%v), want %s and not a defect",
			r.Status, r.IsDefect(), domain.StatusNotApplicable)
	}
}

// TestExplain_defaultViewOnlyWhenNothingWasDeclared: saying "resolved: table" for a Machine whose
// first View is a board would be false, so a Machine declaring its own is NotApplicable.
func TestExplain_defaultViewOnlyWhenNothingWasDeclared(t *testing.T) {
	got := Explain(explainFixture())

	defaulted := find(t, got, domain.DerivationDefaultView+": mch_project")
	if defaulted.Status != domain.StatusResolved || defaulted.Value != string(domain.ViewTable) {
		t.Errorf("a Machine declaring no views is %q (%s), want table resolved", defaulted.Value, defaulted.Status)
	}

	declared := find(t, got, domain.DerivationDefaultView+": mch_task")
	if declared.Status != domain.StatusNotApplicable {
		t.Errorf("a Machine declaring its own views is %s, want %s -- nothing was defaulted",
			declared.Status, domain.StatusNotApplicable)
	}
	if declared.Value != "" {
		t.Errorf("a not-applicable default view reported %q; nothing was defaulted, so there is no value", declared.Value)
	}
}
