package composition

import (
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// TestCarryForward_isDerivedFromTheMachineNotAList is the test that proves what Fase 6c-3 actually
// changed, rather than what it happened to produce.
//
// A test asserting today's field names would pass equally well against the hand-maintained list
// Fase 6c-2 shipped -- and that list had already forgotten fld_signature_image. So this declares a
// Field the code has never heard of and asserts it is carried anyway. That can only pass if the
// list is derived, which is the difference between "fixed" and "cannot recur".
func TestCarryForward_isDerivedFromTheMachineNotAList(t *testing.T) {
	m := &domain.Machine{ID: action.StepMachineID, Fields: []domain.Field{
		{ID: action.FieldStepDocument, Type: domain.FieldTypeRelation, RelatedMachine: action.DocumentMachineID},
		{ID: action.FieldStepSignatureX, Type: domain.FieldTypeNumber},
		{ID: "fld_invented_tomorrow", Type: domain.FieldTypeText},
	}}
	s := &data.Record{ID: "stp_1", Values: map[string]any{
		action.FieldStepDocument:   "doc_1",
		action.FieldStepSignatureX: float64(20),
		"fld_invented_tomorrow":    "still carried",
	}}

	got := carryForward(m, s)
	byName := map[string]string{}
	for _, f := range got {
		byName[f.Name] = f.Value
	}
	if v, ok := byName["fld_invented_tomorrow"]; !ok || v != "still carried" {
		t.Errorf("a Field nobody wrote into this code must still be carried; got %q, ok=%v", v, ok)
	}
	if _, ok := byName[action.FieldStepSignatureX]; ok {
		t.Error("a Field the placement forms submit themselves must NOT be carried, or the form would send it twice")
	}
	if _, ok := byName[action.FieldStepDocument]; !ok {
		t.Error("every other declared Field must be carried")
	}
}

// The approver label resolves through whichever arm the record chose, and the kind is carried
// rather than inferred -- a person whose mch_user record has gone missing must not read as a Group.
func TestApproverOf_resolvesEitherArm(t *testing.T) {
	relations := rendering.RelationOptions{domain.UserMachineID: {{ID: "usr_rina", Label: "Rina Nur"}}}
	groups := rendering.GroupOptions{{ID: "grp_legal", Label: "Legal Group"}}

	person := &data.Record{Values: map[string]any{action.FieldStepAssignee: "usr_rina"}}
	if kind, label := approverOf(person, relations, groups); kind != domain.ActorKindUser || label != "Rina Nur" {
		t.Errorf("person arm = %q/%q, want User/Rina Nur", kind, label)
	}

	group := &data.Record{Values: map[string]any{
		action.FieldStepApproverType:  domain.ActorKindGroup,
		action.FieldStepApproverGroup: "grp_legal",
	}}
	if kind, label := approverOf(group, relations, groups); kind != domain.ActorKindGroup || label != "Legal Group" {
		t.Errorf("group arm = %q/%q, want Group/Legal Group", kind, label)
	}

	// A step with no approver at all is still the person arm, with an empty name -- not a Group.
	if kind, _ := approverOf(&data.Record{Values: map[string]any{}}, relations, groups); kind != domain.ActorKindUser {
		t.Errorf("an unassigned step's kind = %q, want User -- absence must not read as a Group", kind)
	}
}

// An unplaced step still renders: Composition hands the page the defaults the place/width controls
// offer, so the .templ never decides a number itself.
func TestBuildPlacement_unplacedStepGetsRenderableDefaults(t *testing.T) {
	m := &domain.Machine{ID: action.StepMachineID}
	doc := &data.Record{ID: "doc_1", Values: map[string]any{"fld_title": "Contract"}}
	s := &data.Record{ID: "stp_1", Values: map[string]any{}}

	v := buildPlacement(m, doc, []*data.Record{s}, nil, nil, 1, 6, domain.Actor{})
	if len(v.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(v.Steps))
	}
	step := v.Steps[0]
	if step.Placed {
		t.Error("a step with no fld_signature_page is not placed")
	}
	if step.X != 50 || step.Y != 50 || step.Width != 20 {
		t.Errorf("defaults = %v/%v/%v, want 50/50/20 -- the same the place and width controls offer", step.X, step.Y, step.Width)
	}
}
