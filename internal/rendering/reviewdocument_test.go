package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// renderDecisionBar renders reviewDecisionBar alone -- the piece Fase 6b moved off the generic
// record-detail page -- so the two halves of that move can be asserted independently of the whole
// page's chrome.
func renderDecisionBar(t *testing.T, v ReviewView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := reviewDecisionBar(v).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// TestReviewDecisionBar_offersApproveRejectOnlyWhenCanDecide is one half of the Fase 6b move. The
// other half is TestRecordDetailView_noLongerCarriesTheDecisionBar below; together they say the
// bar exists in exactly one place, which is the whole point of giving board 10 its own screen
// rather than leaving two renderings of the same decision to diverge.
func TestReviewDecisionBar_offersApproveRejectOnlyWhenCanDecide(t *testing.T) {
	html := renderDecisionBar(t, ReviewView{StepID: "stp_1", Decision: "pending", CanDecide: true, HasSignature: true})
	if !strings.Contains(html, `value="approved"`) || !strings.Contains(html, `value="rejected"`) {
		t.Error("a viewer who may decide gets both buttons")
	}
	if !strings.Contains(html, "/machines/mch_approval_step/records/stp_1/decide") {
		t.Error("the form must post to this step's own decide route")
	}

	html = renderDecisionBar(t, ReviewView{StepID: "stp_1", Decision: "pending", CanDecide: false})
	if strings.Contains(html, `value="approved"`) {
		t.Error("a viewer who may not decide must not be offered Approve -- the server refuses it anyway, so offering it would only lie")
	}
	if !strings.Contains(html, "Awaiting") {
		t.Error("a pending step someone else holds says who it waits on, rather than rendering nothing")
	}

	// A decided step shows what was decided instead of offering it again.
	html = renderDecisionBar(t, ReviewView{StepID: "stp_1", Decision: "approved", CanDecide: false})
	if strings.Contains(html, `value="approved"`) || !strings.Contains(html, "approved") {
		t.Error("a decided step shows its decision, not the buttons")
	}
}

// The signature canvas appears only when the viewer has nothing on file. Both arms matter: with a
// saved signature Approve must submit directly, and without one it must open the dialog instead --
// the render-time half of a gate whose write-time half is decideStep's own applyApprovalSignature.
func TestReviewDecisionBar_opensTheSignatureModalOnlyWithoutOneOnFile(t *testing.T) {
	withSig := renderDecisionBar(t, ReviewView{StepID: "stp_1", Decision: "pending", CanDecide: true, HasSignature: true})
	if strings.Contains(withSig, "sig-approve-trigger") {
		t.Error("a viewer with a saved signature submits Approve directly")
	}

	withoutSig := renderDecisionBar(t, ReviewView{StepID: "stp_1", Decision: "pending", CanDecide: true, HasSignature: false})
	if !strings.Contains(withoutSig, "sig-approve-trigger") || !strings.Contains(withoutSig, "sig-modal-stp_1") {
		t.Error("a viewer with no signature on file must be sent to the canvas dialog first")
	}
}

// TestRecordDetailView_noLongerCarriesTheDecisionBar is the regression half of the move.
//
// Until Fase 6b an Approval Step's Approve/Reject bar lived on the *generic* record-detail page
// every Machine shares, which is what put detail.templ in internal/conformance's projection
// ratchet: it read fld_decision straight off the record. A later change that re-adds a decide
// control here would silently recreate two renderings of one decision and put that entry back, so
// this asserts the link is what an Approval Step's detail page offers now.
func TestRecordDetailView_noLongerCarriesTheDecisionBar(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_approval_step",
		Name:   "Approval Step",
		Fields: []domain.Field{{ID: "fld_sequence", Name: "Sequence", Type: domain.FieldTypeNumber}},
	}
	r := &data.Record{ID: "stp_1", Values: map[string]any{"fld_sequence": float64(1)}}

	var buf bytes.Buffer
	if err := RecordDetailView(m, r, nil, nil, "usr_ana", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	if strings.Contains(html, `name="decision"`) {
		t.Error("the generic detail page must not render a decide control; board 10's own screen does")
	}
	if !strings.Contains(html, "/machines/mch_approval_step/records/stp_1/review") {
		t.Error("it must link to the review screen instead, so the page is not a dead end")
	}
}

// TestReviewDocumentPage_rendersEndToEnd is a smoke test for the whole board, not one panel: it
// catches a nil Placement, an empty Steps slice or a missing nav fixture panicking at render time,
// which the per-component tests above cannot. It asserts the elements a reviewer actually needs to
// see rather than exact markup, so restyling the board does not break it.
func TestReviewDocumentPage_rendersEndToEnd(t *testing.T) {
	t.Cleanup(func() { ConfigureWorkspace(domain.Workspace{}) })
	ConfigureWorkspace(domain.Workspace{Navigation: []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
	}})

	v := ReviewView{
		StepID: "stp_2", DocumentID: "doc_1",
		Reference: "DOC-0091", Title: "Vendor Contract Q3", DocumentType: "Kontrak",
		Status: "in_review", SubmittedBy: "Budi Santoso", SubmittedAt: "6 Sep 2026",
		FileName: "vendor-contract-q3.pdf", FileHref: "/uploads/k__vendor-contract-q3.pdf",
		PDFPages: 6,
		Steps: []StepApprover{
			{Name: "Rina Nur", Initials: "RN", State: "done", Label: "Finance Review", Decided: "10:42"},
			{Name: "Ana Putri", Initials: "AP", State: "current", Label: "Legal Review", IsYou: true},
			{Name: "Maya Puspita", Initials: "MP", State: "waiting", Label: "Director"},
		},
		StepLabel: "Legal Review", Decision: "pending",
		SLALabel: "OVERDUE", SLAOverdue: true, CanDecide: true, HasSignature: true,
		Placement: &ReviewPlacement{Page: 6, X: 50, Y: 84, Width: 20, PreviewHref: "/machines/mch_document/records/doc_1/pdf-preview?page=6"},
	}

	var buf bytes.Buffer
	if err := ReviewDocumentPage(v, "Dokter Kecil", "AP", "admin").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"DOC-0091", "Vendor Contract Q3", "Budi Santoso", // who and what
		"Finance Review", "Legal Review", "Director", // step names, not assignee names
		"Rina Nur · Approved · 10:42",                   // the sub-line carries the person
		">You<",                                         // the viewer's own row is badged
		"vendor-contract-q3.pdf", "6 pages", "download", // the file card
		"pdf-preview?page=6",          // the placement panel draws the real page
		"OVERDUE", `value="approved"`, // the footer
		"/approval-inbox", // back to the inbox, via routeByID
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// A step with no placement renders the prompt instead of an <img> with an empty src.
	v.Placement = nil
	buf.Reset()
	if err := ReviewDocumentPage(v, "Dokter Kecil", "AP", "admin").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() without a placement error = %v", err)
	}
	if !strings.Contains(buf.String(), "haven't placed your signature") {
		t.Error("a reviewer with no placement must be told so, not shown an empty preview")
	}
}
