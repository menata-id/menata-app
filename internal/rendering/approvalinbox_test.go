package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// pendingCardFixture is the minimal PendingApprovalCard needed to render pendingApprovalCard
// without panicking (parseSLA tolerates an empty/unparseable SLADue, so it's left unset here).
func pendingCardFixture(cardFields []ProjectedField) PendingApprovalCard {
	return PendingApprovalCard{
		Reference:    "DOC-0001",
		Title:        "Contract",
		DocumentType: "Contract",
		Mode:         "sequential",
		Approved:     0,
		TotalSteps:   1,
		Submitter:    "Ana Putri",
		Href:         "/machines/mch_approval_step/records/stp_1",
		CardFields:   cardFields,
	}
}

// TestPendingApprovalCard_rendersCardFields is the Fase 1 pilot's own proof: a card_fields entry
// resolved by internal/composition.ProjectCardFields shows up on the rendered card without this
// file (or pendingApprovalCard/pendingApprovalCardBody) knowing anything about the specific Field
// that produced it -- only its already-resolved Label/Role/Display.
func TestPendingApprovalCard_rendersCardFields(t *testing.T) {
	c := pendingCardFixture([]ProjectedField{
		{Label: "Department", Role: "title", Display: "Legal"},
	})

	var buf bytes.Buffer
	if err := pendingApprovalCard(c).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Department") || !strings.Contains(html, "Legal") {
		t.Errorf("rendered card missing projected field label/value; got:\n%s", html)
	}
}

// TestPendingApprovalCard_noCardFieldsRendersNoMetaRow is the regression guard: every Machine
// today declares no card_fields, so this must render exactly as it did before Fase 1 -- no
// new markup for a card that projects nothing.
func TestPendingApprovalCard_noCardFieldsRendersNoMetaRow(t *testing.T) {
	c := pendingCardFixture(nil)

	var buf bytes.Buffer
	if err := pendingApprovalCard(c).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "pending-card-meta") {
		t.Errorf("card with no CardFields rendered a pending-card-meta block, want none")
	}
}
