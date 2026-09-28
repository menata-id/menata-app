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
	// The Document Machine is here because the submitter arm reads the Field its *own* create
	// Permission already names (Stage D) rather than a constant -- prm_create_own_document's
	// actor_field, exactly as metadata/document.yaml declares it.
	docM := &domain.Machine{
		ID:            action.DocumentMachineID,
		ApplicationID: "app_document_approval",
		Fields:        []domain.Field{{ID: action.FieldDocumentSubmittedBy, Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID}},
		Permissions: []domain.Permission{{
			ID: "prm_create_own_document", Action: domain.ActionCreate,
			Roles: []string{"approver", "submitter"}, ActorField: action.FieldDocumentSubmittedBy,
		}},
	}
	document := &data.Record{ID: "doc_1", Values: map[string]any{action.FieldDocumentSubmittedBy: "usr_nana"}}
	step := &data.Record{ID: "stp_1", Values: map[string]any{action.FieldStepAssignee: "usr_rina"}}

	// The submitter, holding no approver role and named on no step: this is the case the wizard
	// actually produces, and the one that was broken.
	submitter := domain.Actor{ID: "usr_nana", Roles: map[string][]string{"app_document_approval": {"submitter"}}}
	if !MayPlaceSignature(stepM, docM, document, step, submitter) {
		t.Error("the document's own submitter lays the boxes out -- board 09 is step 2 of 3 of submitting")
	}

	// The step's own approver, on someone else's document.
	approver := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_document_approval": {"approver"}}}
	if !MayPlaceSignature(stepM, docM, document, step, approver) {
		t.Error("a step's own approver may still place its signature")
	}

	// Neither: a real member of the Application who is not this document's submitter and holds no
	// step here.
	bystander := domain.Actor{ID: "usr_budi", Roles: map[string][]string{"app_document_approval": {"approver"}}}
	if MayPlaceSignature(stepM, docM, document, step, bystander) {
		t.Error("someone who neither submitted the document nor holds the step must be refused")
	}
	if MayPlaceSignature(stepM, docM, document, step, domain.Actor{}) {
		t.Error("an unidentified caller places nothing")
	}
	// A Document with no submitter recorded -- every one written before fld_submitted_by existed
	// -- falls back to the step's own approver rather than opening up.
	legacy := &data.Record{ID: "doc_0", Values: map[string]any{}}
	if MayPlaceSignature(stepM, docM, legacy, step, submitter) {
		t.Error("an empty fld_submitted_by must grant nobody, the same way an empty actor field does")
	}
}

// The approver label resolves through whichever arm the record chose, and the kind is carried
// rather than inferred -- a person whose mch_user record has gone missing must not read as a Group.
func TestApproverOf_resolvesEitherArm(t *testing.T) {
	relations := rendering.RelationOptions{domain.UserMachineID: {{ID: "usr_rina", Label: "Rina Nur"}}}
	groups := rendering.GroupOptions{{ID: "grp_legal", Label: "Legal Group"}}

	// The Field ids come from the step Machine's own Permission, the way the real screen derives them.
	f := action.DeclaredFields(stepMachineForTest(), nil)

	person := &data.Record{Values: map[string]any{action.FieldStepAssignee: "usr_rina"}}
	if kind, label := approverOf(person, relations, groups, f); kind != domain.ActorKindUser || label != "Rina Nur" {
		t.Errorf("person arm = %q/%q, want User/Rina Nur", kind, label)
	}

	group := &data.Record{Values: map[string]any{
		action.FieldStepApproverType:  domain.ActorKindGroup,
		action.FieldStepApproverGroup: "grp_legal",
	}}
	if kind, label := approverOf(group, relations, groups, f); kind != domain.ActorKindGroup || label != "Legal Group" {
		t.Errorf("group arm = %q/%q, want Group/Legal Group", kind, label)
	}

	// A step with no approver at all is still the person arm, with an empty name -- not a Group.
	if kind, _ := approverOf(&data.Record{Values: map[string]any{}}, relations, groups, f); kind != domain.ActorKindUser {
		t.Errorf("an unassigned step's kind = %q, want User -- absence must not read as a Group", kind)
	}
}

// An unplaced step still renders: Composition hands the page the defaults the place/width controls
// offer, so the .templ never decides a number itself.
func TestBuildPlacement_unplacedStepGetsRenderableDefaults(t *testing.T) {
	m := &domain.Machine{ID: action.StepMachineID}
	doc := &data.Record{ID: "doc_1", Values: map[string]any{"fld_title": "Contract"}}
	s := &data.Record{ID: "stp_1", Values: map[string]any{}}

	v := buildPlacement(m, nil, doc, []*data.Record{s}, nil, nil, 1, 6, domain.Actor{})
	if len(v.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(v.Steps))
	}
	step := v.Steps[0]
	if step.Placed {
		t.Error("a step with no declared page value is not placed")
	}
	if step.X != 50 || step.Y != 50 || step.Width != 20 {
		t.Errorf("defaults = %v/%v/%v, want 50/50/20 -- the same the place and width controls offer", step.X, step.Y, step.Width)
	}
}

// TestBuildPlacement_overAMachineThatNamesItsPlacementDifferently is Stage D's own property, on the
// screen that both reads and writes these Fields: a step Machine whose coordinates are called
// fld_hal/fld_x/fld_y/fld_lebar renders its markers in the right places *and* gives its forms the
// right input names, with none of Document Approval's own Field ids reaching the page.
//
// The write half matters as much as the read half here. Until Stage D the four `name=` attributes on
// board 09 were action.Field* constants in the .templ, so this Machine would have rendered its
// markers correctly and then submitted every drag under names it does not have -- a screen that looks
// composable and silently writes nothing.
func TestBuildPlacement_overAMachineThatNamesItsPlacementDifferently(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_langkah",
		SignaturePlacement: &domain.SignaturePlacement{
			ImageField: "fld_gambar",
			PageField:  "fld_hal",
			XField:     "fld_x",
			YField:     "fld_y",
			WidthField: "fld_lebar",
		},
	}
	doc := &data.Record{ID: "doc_1", Values: map[string]any{"fld_title": "Kontrak"}}
	s := &data.Record{ID: "stp_1", Values: map[string]any{
		"fld_hal": float64(2), "fld_x": float64(30), "fld_y": float64(70), "fld_lebar": float64(25),
	}}

	v := buildPlacement(m, nil, doc, []*data.Record{s}, nil, nil, 2, 6, domain.Actor{})
	step := v.Steps[0]
	if !step.Placed || step.Page != 2 || step.X != 30 || step.Y != 70 || step.Width != 25 {
		t.Errorf("read side = placed %v at page %d %v/%v width %v, want page 2 at 30/70 width 25",
			step.Placed, step.Page, step.X, step.Y, step.Width)
	}
	want := rendering.PlacementFields{Page: "fld_hal", X: "fld_x", Y: "fld_y", Width: "fld_lebar"}
	if step.Fields != want {
		t.Errorf("write side = %+v, want %+v -- the form must submit under the names this Machine declares", step.Fields, want)
	}
}

// A step Machine declaring no signature_placement has no placement and no form names, rather than
// falling back to internal/action's constants -- proved with a record that does hold them.
func TestBuildPlacement_undeclaredPlacementReadsAndWritesNothing(t *testing.T) {
	m := &domain.Machine{ID: "mch_langkah"}
	doc := &data.Record{ID: "doc_1", Values: map[string]any{"fld_title": "Kontrak"}}
	s := &data.Record{ID: "stp_1", Values: map[string]any{
		action.FieldStepSignaturePage:  float64(2),
		action.FieldStepSignatureX:     float64(30),
		action.FieldStepSignatureY:     float64(70),
		action.FieldStepSignatureWidth: float64(25),
	}}

	step := buildPlacement(m, nil, doc, []*data.Record{s}, nil, nil, 2, 6, domain.Actor{}).Steps[0]
	if step.Placed {
		t.Error("a Machine declaring no signature_placement must not read a placement out of this package's own constants")
	}
	if (step.Fields != rendering.PlacementFields{}) {
		t.Errorf("form names = %+v, want empty -- there is no declared name to submit under", step.Fields)
	}
}
