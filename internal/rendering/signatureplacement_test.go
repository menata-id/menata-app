package rendering

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/action"
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
		}},
	}
}

func renderPlacement(t *testing.T, v PlacementView) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
		{ID: "nav_my_documents", Label: "My Documents", Route: "/approval-inbox?tab=mine"},
	}}, "Test Workspace", false)
	var buf bytes.Buffer
	if err := SignaturePlacementPage(v, "Dokter Kecil", Viewer{Initials: "AP"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// The forms must write through the placement route, not the generic record route -- and must send
// nothing but the four placement Fields.
//
// This replaces TestSignaturePlacementPage_rendersEveryCarriedField, which asserted the opposite
// shape: that every *other* Field on the step was echoed back as a hidden input, because the
// generic route rewrote a whole record from whatever was submitted and erased what was missing.
// That echo is gone. A route writing four named Fields cannot erase a fifth, so the property worth
// holding is now the absence: any hidden input naming a Field outside the four is a step back
// toward the bug (ROADMAP.md Fase 6c-2/6c-3, where the hand-written list forgot four Fields and
// silently destroyed a one-time signature on the next drag).
func TestSignaturePlacementPage_writesOnlyPlacementFields(t *testing.T) {
	html := renderPlacement(t, placementFixture(true))

	if !strings.Contains(html, "/signature-placement\"") {
		t.Error("the forms must PUT to the placement route, not the generic record route")
	}
	placement := map[string]bool{
		action.FieldStepSignaturePage:  true,
		action.FieldStepSignatureX:     true,
		action.FieldStepSignatureY:     true,
		action.FieldStepSignatureWidth: true,
	}
	for _, m := range regexp.MustCompile(`<input type="hidden" name="(fld_[a-z_]+)"`).FindAllStringSubmatch(html, -1) {
		if !placement[m[1]] {
			t.Errorf("%s is submitted as a hidden input -- this screen writes only the four placement fields", m[1])
		}
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
