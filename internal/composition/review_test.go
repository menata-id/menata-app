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
func docMachineForTest() *domain.Machine {
	return &domain.Machine{ID: action.DocumentMachineID, SLAField: "fld_due_date"}
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
	got := buildReview(steps[0], document, steps, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_budi"), 6, true, at(10))
	if !got.CanDecide {
		t.Error("step 1's own assignee must be offered the decision bar")
	}

	// usr_ana holds step 2, which sequential mode locks behind step 1.
	if got := buildReview(steps[1], document, steps, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10)); got.CanDecide {
		t.Error("a step locked behind an earlier one must not be offered the bar; the server would refuse the POST")
	}

	// Somebody else entirely, looking at step 1.
	if got := buildReview(steps[0], document, steps, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10)); got.CanDecide {
		t.Error("a viewer who is not this step's assignee must not be offered the bar")
	}

	// Already decided: there is nothing left to offer, whoever is looking.
	decided := step("stp_1", "doc_1", "usr_budi", action.DecisionApproved, 1)
	if got := buildReview(decided, document, []*data.Record{decided}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_budi"), 6, true, at(10)); got.CanDecide {
		t.Error("a decided step must not offer the bar again -- action.CanDecide's semantics are one-way")
	}
}

// Fase 6b declared fld_step_name but nothing writes it until board 08's wizard (6c), so the
// fallback is the live path, not the edge case: a step with no declared name is titled with its
// assignee, exactly as approvalStepRow has always titled it.
func TestBuildReview_StepLabelFallsBackToAssignee(t *testing.T) {
	s := step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)
	got := buildReview(s, reviewDoc(), []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10))
	if got.StepLabel != "Ana Putri" {
		t.Errorf("StepLabel = %q, want the assignee's name while fld_step_name is empty", got.StepLabel)
	}

	s.Values[action.FieldStepName] = "Legal Review"
	got = buildReview(s, reviewDoc(), []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10))
	if got.StepLabel != "Legal Review" {
		t.Errorf("StepLabel = %q, want the declared fld_step_name once it exists", got.StepLabel)
	}
	if got.Steps[0].Label != "Legal Review" {
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
	got := buildReview(steps[1], reviewDoc(), steps, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10))
	if len(got.Steps) != 3 {
		t.Fatalf("want one row per step, got %d", len(got.Steps))
	}
	if got.Steps[0].IsYou || !got.Steps[1].IsYou || got.Steps[2].IsYou {
		t.Errorf("IsYou = %v/%v/%v, want only the viewer's own step marked",
			got.Steps[0].IsYou, got.Steps[1].IsYou, got.Steps[2].IsYou)
	}

	// An anonymous viewer marks nothing -- including the unassigned step 3.
	got = buildReview(steps[1], reviewDoc(), steps, nil, personNames, stepMachineForTest(), docMachineForTest(), domain.Actor{}, 0, true, at(10))
	for i, s := range got.Steps {
		if s.IsYou {
			t.Errorf("step %d marked IsYou for an empty viewer; an unassigned step is not everyone's", i+1)
		}
	}
}

// A placement exists only when a page was chosen. Board 10's right column is the difference
// between "here is where your signature lands" and "you haven't placed one yet", so reading a
// half-written step as placed would draw a marker at 0,0 over page 0.
func TestBuildReview_PlacementNeedsAPage(t *testing.T) {
	s := step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)
	got := buildReview(s, reviewDoc(), []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10))
	if got.Placement != nil {
		t.Error("a step with no fld_signature_page has no placement to draw")
	}

	s.Values[action.FieldStepSignaturePage] = float64(6)
	s.Values[action.FieldStepSignatureX] = float64(50)
	s.Values[action.FieldStepSignatureY] = float64(84)
	got = buildReview(s, reviewDoc(), []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10))
	if got.Placement == nil {
		t.Fatal("a step with a page has a placement")
	}
	if got.Placement.Page != 6 || got.Placement.X != 50 || got.Placement.Y != 84 {
		t.Errorf("Placement = %+v, want page 6 at 50,84", got.Placement)
	}
	// The width default matches what widthControl offers, so a step placed before widths existed
	// renders at the same size the placement screen would show it.
	if got.Placement.Width != 20 {
		t.Errorf("Placement.Width = %v, want the same 20%% default the placement screen uses", got.Placement.Width)
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
			got := buildReview(s, document, []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10))
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
	got := buildReview(s, reviewDoc(), []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 6, true, at(10))
	if got.FileName != "vendor-contract-q3.pdf" || got.FileHref != "/uploads/abc123__vendor-contract-q3.pdf" {
		t.Errorf("File = %q / %q, want the stored key's display name and its upload URL", got.FileName, got.FileHref)
	}
	if got.PDFPages != 6 {
		t.Errorf("PDFPages = %d, want the count the caller supplied", got.PDFPages)
	}

	bare := reviewDoc()
	delete(bare.Values, "fld_file")
	if got := buildReview(s, bare, []*data.Record{s}, nil, personNames, stepMachineForTest(), docMachineForTest(), approverActor("usr_ana"), 0, true, at(10)); got.FileHref != "" {
		t.Errorf("FileHref = %q, want empty when no file is attached", got.FileHref)
	}
}
