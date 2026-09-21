package composition

import (
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// MayPlaceSignature is the whole of who can move a box on board 09, and the submitter arm is the
// one that needed a function rather than a Permission: it reads fld_submitted_by on the *parent*
// Document, which no Permission arm in this runtime can reach.
func TestMayPlaceSignature(t *testing.T) {
	stepM := &domain.Machine{
		ID:            action.StepMachineID,
		ApplicationID: "app_document_approval",
		Fields:        []domain.Field{{ID: action.FieldStepAssignee, Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID}},
		Permissions: []domain.Permission{{
			ID: "prm_edit_own_step", Action: domain.ActionEdit,
			Roles: []string{"approver"}, ActorField: action.FieldStepAssignee,
		}},
	}
	document := &data.Record{ID: "doc_1", Values: map[string]any{action.FieldDocumentSubmittedBy: "usr_nana"}}
	step := &data.Record{ID: "stp_1", Values: map[string]any{action.FieldStepAssignee: "usr_rina"}}

	// The submitter, holding no approver role and named on no step: this is the case the wizard
	// actually produces, and the one that was broken.
	submitter := domain.Actor{ID: "usr_nana", Roles: map[string][]string{"app_document_approval": {"submitter"}}}
	if !MayPlaceSignature(stepM, document, step, submitter) {
		t.Error("the document's own submitter lays the boxes out -- board 09 is step 2 of 3 of submitting")
	}

	// The step's own approver, on someone else's document.
	approver := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_document_approval": {"approver"}}}
	if !MayPlaceSignature(stepM, document, step, approver) {
		t.Error("a step's own approver may still place its signature")
	}

	// Neither: a real member of the Application who is not this document's submitter and holds no
	// step here.
	bystander := domain.Actor{ID: "usr_budi", Roles: map[string][]string{"app_document_approval": {"approver"}}}
	if MayPlaceSignature(stepM, document, step, bystander) {
		t.Error("someone who neither submitted the document nor holds the step must be refused")
	}
	if MayPlaceSignature(stepM, document, step, domain.Actor{}) {
		t.Error("an unidentified caller places nothing")
	}
	// A Document with no submitter recorded -- every one written before fld_submitted_by existed
	// -- falls back to the step's own approver rather than opening up.
	legacy := &data.Record{ID: "doc_0", Values: map[string]any{}}
	if MayPlaceSignature(stepM, legacy, step, submitter) {
		t.Error("an empty fld_submitted_by must grant nobody, the same way an empty actor field does")
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
