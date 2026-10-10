package composition

import (
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func reviewDoc() *data.Record {
	d := doc("doc_1", "Vendor Contract Q3", "sequential", "2026-09-20")
	d.Values["fld_file"] = "abc123__vendor-contract-q3.pdf"
	return d
}

// docMachineForTest is mch_document reduced to what buildReview reads: which Field carries the
// SLA. Declared rather than assumed, the same posture stepMachineForTest takes for sequencing.
//
// It carries the workflow binding for the same reason stepMachineForTest does (2026-09-28): the
// loader stamps WorkflowEngine/WorkflowRole from the Application's own workflow: block, and a
// fixture naming only the id would be some Workspace's Machine of that name rather than an approval
// engine's document.
func docMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: action.DocumentMachineID, Name: "Document",
		// The Fields its own transitions, sla_field and actions: block name. Absent until 2026-09-29,
		// when this fixture was run through metadata.Validate: declaring behaviour over Fields the
		// Machine does not have describes something the loader would refuse to load.
		Fields: []domain.Field{
			{ID: action.FieldDocumentStatus, Name: "Status", Type: domain.FieldTypeStatus, Options: []string{action.DocumentStatusDraft, action.DocumentStatusInReview, action.DocumentStatusApproved, action.DocumentStatusRejected}},
			{ID: action.FieldDocumentSubmittedBy, Name: "Submitted By", Type: domain.FieldTypePerson},
			{ID: "fld_due_date", Name: "Due", Type: domain.FieldTypeDate},
		},
		// Its own state model, so StatusField can answer which Field a Document's status lives in --
		// the six edges name no Action (that status is derived from its steps), which is exactly why
		// ActionField cannot answer it and this fixture has to carry them.
		Transitions: []domain.Transition{
			{ID: "trn_document_approved", Name: "Approved", Field: action.FieldDocumentStatus, From: action.DocumentStatusInReview, To: action.DocumentStatusApproved},
			{ID: "trn_document_rejected", Name: "Rejected", Field: action.FieldDocumentStatus, From: action.DocumentStatusInReview, To: action.DocumentStatusRejected},
		},
		ApplicationID:  "app_document_approval",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleDocument,
		SLAField:       "fld_due_date",
		// The rule blocks metadata/document.yaml declares, added when
		// TestFixturesMirrorTheRealMachines was written (2026-09-28). This fixture is otherwise a
		// deliberate reduction -- it carries no Fields, Views, Datasets or card_fields, and the gate
		// does not ask it to: structure and presentation are where a reduction is legitimate. A
		// missing *rule* is not, because MayPlaceSignature already reads one of these (the create
		// Permission's actor_field, since Stage D) and buildReview would answer differently without it.
		Permissions: []domain.Permission{
			{ID: "prm_create_own_document", Action: domain.ActionCreate, Roles: []string{"approver", "submitter"}, ActorField: action.FieldDocumentSubmittedBy},
			{ID: "prm_edit_document_not_reviewer", Action: domain.ActionEdit, Roles: []string{"approver", "submitter"}},
			{ID: "prm_delete_document_not_reviewer", Action: domain.ActionDelete, Roles: []string{"approver", "submitter"}},
		},
		ActionEffects: []domain.ActionEffect{
			{Action: domain.ActionRevise, Writes: []domain.FieldWrite{{Field: action.FieldDocumentStatus, Value: action.DocumentStatusDraft}}},
			{Action: domain.ActionCreate, Writes: []domain.FieldWrite{{Field: action.FieldDocumentStatus, Value: action.DocumentStatusInReview}}},
		},
		Events: []domain.Event{{
			ID: "evt_document_submitted", OnCreate: true,
			Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "\"{fld_title}\" submitted"},
		}},
	}
}

// The decision bar is offered to exactly one person: this step's own assignee, on a step that is
// still pending and not locked behind an earlier one. Every other viewer sees the same page
// read-only. This is the half of the Fase 6b move that is easy to get wrong -- the bar used to be
// gated inside a .templ that had the record in hand, and the gate now lives one plane away.
func TestBuildReview_DecisionBarIsOfferedToItsOwnAssigneeOnly(t *testing.T) {
	steps := []*data.Record{
		step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1),
		step("stp_2", "doc_1", "usr_ana", action.DecisionPending, 2),
	}
	document := reviewDoc()

	// usr_budi holds step 1 and nothing is ahead of it.
	got := buildReview(steps[0], document, steps, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_budi"), 6, true, at(10), nil)
	if !got.CanDecide {
		t.Error("step 1's own assignee must be offered the decision bar")
	}

	// usr_ana holds step 2, which sequential mode locks behind step 1.
	if got := buildReview(steps[1], document, steps, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10), nil); got.CanDecide {
		t.Error("a step locked behind an earlier one must not be offered the bar; the server would refuse the POST")
	}

	// Somebody else entirely, looking at step 1.
	if got := buildReview(steps[0], document, steps, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10), nil); got.CanDecide {
		t.Error("a viewer who is not this step's assignee must not be offered the bar")
	}

	// Already decided: there is nothing left to offer, whoever is looking.
	decided := step("stp_1", "doc_1", "usr_budi", action.DecisionApproved, 1)
	if got := buildReview(decided, document, []*data.Record{decided}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_budi"), 6, true, at(10), nil); got.CanDecide {
		t.Error("a decided step must not offer the bar again -- action.CanDecide's semantics are one-way")
	}
}

// TestBuildReview_StepLabelIsTheAssignee: a step is identified by who holds it.
//
// This test used to assert a fallback -- the assignee's name "while fld_step_name is empty", then the
// declared name once set. That Field was deleted on 2026-09-29: board 08's own model is "each step is
// one person or a whole group", nothing filled the Field in 23 records, and what boards 09 and 10 draw
// in its place is the approver's job title, which is identity data rather than a Field on a step. So
// there is no fallback any more -- there is one answer.
func TestBuildReview_StepLabelIsTheAssignee(t *testing.T) {
	s := step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)
	got := buildReview(s, reviewDoc(), []*data.Record{s}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10), nil)
	if got.StepLabel != "Ana Putri" {
		t.Errorf("StepLabel = %q, want the assignee's name", got.StepLabel)
	}
	if got.Steps[0].Label != "Ana Putri" {
		t.Errorf("Steps[0].Label = %q, want the same label the progress list renders", got.Steps[0].Label)
	}
}

// IsYou is what lets board 10 badge one row "You". It is the only thing distinguishing this
// viewer's row from anyone else's, so an empty viewer must not accidentally match an unassigned
// step -- both are the empty string in storage.
func TestBuildReview_MarksOnlyTheViewersOwnStep(t *testing.T) {
	steps := []*data.Record{
		step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1),
		step("stp_2", "doc_1", "usr_ana", action.DecisionPending, 2),
		step("stp_3", "doc_1", "", action.DecisionPending, 3),
	}
	got := buildReview(steps[1], reviewDoc(), steps, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10), nil)
	if len(got.Steps) != 3 {
		t.Fatalf("want one row per step, got %d", len(got.Steps))
	}
	if got.Steps[0].IsYou || !got.Steps[1].IsYou || got.Steps[2].IsYou {
		t.Errorf("IsYou = %v/%v/%v, want only the viewer's own step marked",
			got.Steps[0].IsYou, got.Steps[1].IsYou, got.Steps[2].IsYou)
	}

	// An anonymous viewer marks nothing -- including the unassigned step 3.
	got = buildReview(steps[1], reviewDoc(), steps, personNames, stepMachineForTest(), docMachineForTest(), domain.Actor{}, 0, true, at(10), nil)
	for i, s := range got.Steps {
		if s.IsYou {
			t.Errorf("step %d marked IsYou for an empty viewer; an unassigned step is not everyone's", i+1)
		}
	}
}

// A signature box exists only when a page was chosen. The Signature positions panel's difference
// between "here is where your signature lands" and "you haven't placed one yet" is SignaturePage
// being 0, so reading a half-written step as placed would draw a marker at 0,0 over page 0.
func TestBuildReview_SignatureBoxNeedsAPage(t *testing.T) {
	s := step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)
	got := buildReview(s, reviewDoc(), []*data.Record{s}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10), nil)
	if got.SignaturePage != 0 || len(got.SignatureBoxes) != 0 {
		t.Errorf("SignaturePage/SignatureBoxes = %d/%+v, want no page and no boxes when no step has placed one",
			got.SignaturePage, got.SignatureBoxes)
	}

	s.Values[action.FieldStepSignaturePage] = float64(6)
	s.Values[action.FieldStepSignatureX] = float64(50)
	s.Values[action.FieldStepSignatureY] = float64(84)
	got = buildReview(s, reviewDoc(), []*data.Record{s}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10), nil)
	if got.SignaturePage != 6 {
		t.Fatalf("SignaturePage = %d, want 6 once a step has placed one", got.SignaturePage)
	}
	if len(got.SignatureBoxes) != 1 {
		t.Fatalf("SignatureBoxes = %+v, want the one placed step's own box", got.SignatureBoxes)
	}
	box := got.SignatureBoxes[0]
	if box.X != 50 || box.Y != 84 {
		t.Errorf("box = %+v, want 50,84", box)
	}
	// The width default matches what widthControl offers, so a step placed before widths existed
	// renders at the same size the placement screen would show it.
	if box.Width != 20 {
		t.Errorf("box.Width = %v, want the same 20%% default the placement screen uses", box.Width)
	}
	// The lone step is this viewer's own and still pending: "yours", the panel's own vocabulary
	// for "not yet, but it will be stamped here when you approve".
	if box.Kind != "yours" {
		t.Errorf("box.Kind = %q, want %q for the viewer's own still-pending step", box.Kind, "yours")
	}
	if got.SignaturePreviewHref != "/machines/mch_document/records/doc_1/pdf-preview?page=6" {
		t.Errorf("SignaturePreviewHref = %q, want the pdf-preview route board 09 already serves", got.SignaturePreviewHref)
	}
	if got.AllPositionsHref != "/machines/mch_document/records/doc_1/signature-placement?page=6" {
		t.Errorf("AllPositionsHref = %q, want board 09's own screen", got.AllPositionsHref)
	}
}

// A "signed" box's own ImageHref prefers the step's one-time captured image and falls back to the
// approver's saved reusable signature only when that one-time image is empty -- the same
// precedence internal/execution.signatureImageForStep already composites onto the PDF with. Every
// other kind never shows an image at all, on file or not: "yours"/"waiting" describe a decision
// that has not happened yet, so there is nothing captured to show.
func TestBuildReview_SignatureBoxImagePrefersOneTimeThenSavedSignature(t *testing.T) {
	approved := step("stp_1", "doc_1", "usr_budi", action.DecisionApproved, 1)
	approved.Values[action.FieldStepSignaturePage] = float64(6)
	approved.Values[action.FieldStepSignatureX] = float64(50)
	approved.Values[action.FieldStepSignatureY] = float64(84)

	// No one-time image and no saved signature on file: the box still renders, just with nothing
	// to show but the plain checkmark.
	got := buildReview(approved, reviewDoc(), []*data.Record{approved}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10), nil)
	if len(got.SignatureBoxes) != 1 || got.SignatureBoxes[0].Kind != "signed" {
		t.Fatalf("SignatureBoxes = %+v, want one signed box", got.SignatureBoxes)
	}
	if got.SignatureBoxes[0].ImageHref != "" {
		t.Errorf("ImageHref = %q, want empty when neither image exists", got.SignatureBoxes[0].ImageHref)
	}

	// A saved reusable signature on file for this step's own assignee: the fallback.
	got = buildReview(approved, reviewDoc(), []*data.Record{approved}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10),
		map[string]string{"usr_budi": "mch_signature/fld_image/saved__sig.png"})
	if want := "/uploads/mch_signature/fld_image/saved__sig.png"; got.SignatureBoxes[0].ImageHref != want {
		t.Errorf("ImageHref = %q, want the saved signature's own upload URL %q", got.SignatureBoxes[0].ImageHref, want)
	}

	// The step's own one-time image, once it has one, wins over the saved fallback -- it is what
	// was actually captured at the moment this step was decided.
	approved.Values[action.FieldStepSignatureImage] = "mch_approval_step/fld_signature_image/one_time__sig.png"
	got = buildReview(approved, reviewDoc(), []*data.Record{approved}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10),
		map[string]string{"usr_budi": "mch_signature/fld_image/saved__sig.png"})
	if want := "/uploads/mch_approval_step/fld_signature_image/one_time__sig.png"; got.SignatureBoxes[0].ImageHref != want {
		t.Errorf("ImageHref = %q, want the one-time image to win over the saved fallback", got.SignatureBoxes[0].ImageHref)
	}
}

// The SLA line is day-scale on purpose (board 10 asks for "Breached · 4 hours ago"; fld_due_date
// has no time component). This pins the two outcomes the footer actually branches on.
func TestBuildReview_SLAIsDayScale(t *testing.T) {
	s := step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)
	for _, tc := range []struct {
		name        string
		due         string
		wantLabel   string
		wantOverdue bool
	}{
		{"due today", "2026-09-10", "Due today", false},
		{"due yesterday", "2026-09-09", "OVERDUE", true},
		{"no due date", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := reviewDoc()
			document.Values["fld_due_date"] = tc.due
			got := buildReview(s, document, []*data.Record{s}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10), nil)
			if got.SLALabel != tc.wantLabel || got.SLAOverdue != tc.wantOverdue {
				t.Errorf("SLA = %q/%v, want %q/%v", got.SLALabel, got.SLAOverdue, tc.wantLabel, tc.wantOverdue)
			}
		})
	}
}

// The file card links to the upload, and says so only when there is one: a Document with no file
// attached renders an explicit absence rather than an href to /uploads/.
func TestBuildReview_FileCardHandlesAMissingFile(t *testing.T) {
	s := step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)
	got := buildReview(s, reviewDoc(), []*data.Record{s}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10), nil)
	if got.FileName != "vendor-contract-q3.pdf" || got.FileHref != "/uploads/abc123__vendor-contract-q3.pdf" {
		t.Errorf("File = %q / %q, want the stored key's display name and its upload URL", got.FileName, got.FileHref)
	}
	if got.PDFPages != 6 {
		t.Errorf("PDFPages = %d, want the count the caller supplied", got.PDFPages)
	}

	bare := reviewDoc()
	delete(bare.Values, "fld_file")
	if got := buildReview(s, bare, []*data.Record{s}, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10), nil); got.FileHref != "" {
		t.Errorf("FileHref = %q, want empty when no file is attached", got.FileHref)
	}
}

// TestBuildReview_resolvesTheFileFieldFromTheCompositeDeclaration is the renamed-Machine case for the
// one Field the review screen reads off the Document.
//
// Before 2026-09-29 that Field was action.FieldDocumentFile, a constant -- so a Workspace whose
// Document calls its upload anything else showed no file at all, silently, on a screen whose whole
// subject is the document. The declaration answering it already existed: the compositing Event's
// `source_field` (action.CompositeFields), which is asked of the *step* Machine because that is where
// the Event lives while the caller holds the Document -- exactly what that accessor's own comment says
// it is for.
func TestBuildReview_resolvesTheFileFieldFromTheCompositeDeclaration(t *testing.T) {
	stepMachine := stepMachineForTest()
	// Replaced, not appended: CompositeFields returns the first matching Event, so appending would have
	// left the fixture's own fld_file winning and this test asserting nothing. Caught by it failing.
	stepMachine.Events = []domain.Event{{
		ID: "evt_composite", On: "fld_decision",
		Then: domain.Service{
			Name:      domain.ServiceCompositeSignedDocument,
			Composite: &domain.Composite{SourceField: "fld_berkas", TargetField: "fld_signed"},
		},
	}}

	document := doc("doc_1", "Kontrak", "sequential", "2026-09-20")
	// The upload lives under a name this engine's own constants do not know.
	document.Values["fld_berkas"] = "xyz789__kontrak.pdf"

	steps := []*data.Record{step("stp_1", "doc_1", "usr_budi", "pending", 1)}
	got := buildReview(steps[0], document, steps, personNames, stepMachine, docMachineForTest(), approverActor("usr_budi"), 6, true, at(10), nil)

	if got.FileName == "" {
		t.Error("the review screen resolved no file: the Field was read from a constant rather than " +
			"from the compositing Event's own source_field, so a Document naming its upload anything " +
			"else shows nothing -- silently, on the screen whose subject is the document")
	}
	if got.FileHref != "/uploads/xyz789__kontrak.pdf" {
		t.Errorf("FileHref = %q, want the declared Field's value", got.FileHref)
	}
}
