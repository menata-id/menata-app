package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func placementFixture(editable bool) PlacementView {
	return PlacementView{
		DocumentID: "doc_1", DocumentTitle: "Vendor Contract Q3", Page: 1, TotalPages: 6,
		Steps: []PlacementStep{{
			StepID: "stp_2", Index: 2, StepName: "Legal Review",
			Approver: "Legal Group", ApproverKind: domain.ActorKindGroup,
			Placed: true, Page: 1, X: 20, Y: 84, Width: 25,
			Editable: editable,
			CarryFields: []CarryField{
				{Name: "fld_document", Value: "doc_1"},
				{Name: "fld_approver_type", Value: domain.ActorKindGroup},
				{Name: "fld_approver_group", Value: "grp_legal"},
				{Name: "fld_signature_image", Value: "sig_key"},
			},
		}},
	}
}

func renderPlacement(t *testing.T, v PlacementView) string {
	t.Helper()
	t.Cleanup(func() { ConfigureWorkspace(domain.Workspace{}) })
	ConfigureWorkspace(domain.Workspace{Navigation: []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
	}})
	var buf bytes.Buffer
	if err := SignaturePlacementPage(v, "Dokter Kecil", "AP", "admin", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// Every carried Field must reach the form as a hidden input, whatever it is.
//
// This asserts the *list it was handed*, not a set of names this test knows — which is the whole
// difference from the version Fase 6c-2 shipped. That one checked three specific names, so it
// passed against a hand-maintained list that had already forgotten fld_signature_image. A loop
// over CarryFields cannot forget, and this test cannot pass by coincidence.
func TestSignaturePlacementPage_rendersEveryCarriedField(t *testing.T) {
	v := placementFixture(true)
	html := renderPlacement(t, v)
	for _, f := range v.Steps[0].CarryFields {
		if !strings.Contains(html, `name="`+f.Name+`"`) {
			t.Errorf("no hidden input for %s -- the next drag would erase it, silently", f.Name)
		}
		if f.Value != "" && !strings.Contains(html, `value="`+f.Value+`"`) {
			t.Errorf("%s is present but does not carry %q forward", f.Name, f.Value)
		}
	}
	// fld_signature_image is called out by name because it is the one Fase 6c-2's hand-written
	// list missed: a one-time signature captured at Approve time was still being erased by the
	// next marker drag after that fix shipped.
	if !strings.Contains(html, `name="fld_signature_image"`) {
		t.Error("fld_signature_image must be carried -- it was the Field the hand-maintained list forgot")
	}
}

// The positional CSS is behaviour, not decoration: the drag script writes the pointer percentage
// straight into left/top and depends on the centring transform, on absolute-against-relative, and
// on touch-action to survive a touch drag. They are asserted as utilities on the element because
// a class name does not carry styling across a shell boundary — which is how board 10's signature
// canvas silently lost touch-action when it moved to appShell in Fase 6b.
func TestSignaturePlacementPage_markerCarriesItsPositioningUtilities(t *testing.T) {
	html := renderPlacement(t, placementFixture(true))
	for _, want := range []string{"-translate-x-1/2", "-translate-y-1/2", "touch-none", "absolute", "relative"} {
		if !strings.Contains(html, want) {
			t.Errorf("marker/canvas is missing %q -- the drag maths depends on it, and losing it moves every stored placement", want)
		}
	}
	if !strings.Contains(html, "left:20.0%;top:84.0%") {
		t.Error("the marker must be positioned by inline style: the numbers are per-record, so a built class name would never reach app.css")
	}
}

// A viewer who may not edit gets a marker with no form around it. That is also what the drag
// script now checks before it starts dragging — before Fase 6c-3 it matched the marker by class,
// found no form, and threw on the first pointermove.
func TestSignaturePlacementPage_nonEditableStepHasNoForm(t *testing.T) {
	html := renderPlacement(t, placementFixture(false))
	if strings.Contains(html, "hx-put") {
		t.Error("a step this viewer may not edit must render no form at all")
	}
	if !strings.Contains(html, "sig-marker") {
		t.Error("...but its marker is still drawn, so the page shows where the signature will land")
	}
	if !strings.Contains(html, "!marker.closest(\"form\")") {
		t.Error("the drag script must refuse to drag a marker with no form, rather than throwing on pointermove")
	}
}

// Board 09's User/Group badge. An empty kind renders nothing rather than guessing, because the
// wizard deliberately stores no approver type on a User row and Composition resolves that absence
// itself — an empty value reaching here means something genuinely unknown.
func TestApproverKindBadge_rendersNothingForAnUnknownKind(t *testing.T) {
	var buf bytes.Buffer
	if err := approverKindBadge("").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("empty kind rendered %q, want nothing", buf.String())
	}
}
