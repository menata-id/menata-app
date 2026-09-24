package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestRecordRow_deleteRespectsBusinessState is the regression test for a code-review finding
// (2026-09-19): RecordRow's Delete button used to check only authorization.AllowsAction, never
// action.CanDelete's own business-state rule (an already-decided Approval Step, an approved
// Document) -- detail.templ's canDeleteInView already enforced both, so a decided step reached
// via the generic list/board (or a Document's own child-collection view, which reuses RecordRow)
// offered a Delete button internal/web's deleteAllowed would then reject server-side.
func TestRecordRow_deleteRespectsBusinessState(t *testing.T) {
	m := &domain.Machine{
		ID:   action.StepMachineID,
		Name: "Approval Step",
		Fields: []domain.Field{
			{ID: action.FieldStepAssignee, Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
			{ID: action.FieldStepDecision, Name: "Decision", Type: domain.FieldTypeStatus, Options: []string{action.DecisionPending, action.DecisionApproved, action.DecisionRejected}},
		},
		// No declared Permission -- AllowsAction(ActionDelete) is unconditionally true, isolating
		// this test to action.CanDelete's own business-state check.
	}
	r := &data.Record{
		ID: "rec_step1",
		Values: map[string]any{
			action.FieldStepAssignee: "rec_user1",
			action.FieldStepDecision: action.DecisionApproved, // already decided -- CanDelete must refuse
		},
	}

	var buf bytes.Buffer
	if err := RecordRow(m, r, nil, nil, domain.Actor{ID: "rec_user1"}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "hx-delete") {
		t.Error(`RecordRow rendered a Delete button for an already-decided Approval Step -- action.CanDelete should have refused it regardless of AllowsAction`)
	}
}

func docMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_document",
		Name: "Document",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"in_review", "approved"}},
		},
		CardFields: []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}},
		Views: []domain.View{
			{ID: "vw_document_table", Name: "Table", Type: domain.ViewTable},
			{ID: "vw_document_cards", Name: "Cards", Type: domain.ViewCards},
		},
	}
}

// A cards View renders its Machine's projected card_fields, and nothing else -- the first time
// Projection's output actually reaches a screen and varies per record.
func TestMachineBody_cardsViewRendersProjectedFields(t *testing.T) {
	m := docMachine()
	records := []*data.Record{{ID: "doc_1", Values: map[string]any{"fld_title": "Kontrak A"}}}
	cards := []RecordCard{{
		Record: records[0],
		Fields: []ProjectedField{{Label: "Status", Role: "status", Display: "in_review"}},
	}}

	var buf bytes.Buffer
	if err := MachineBody(m, m.Views[1], records, nil, nil, nil, cards, domain.Actor{ID: "usr_ana"}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	// Asserts on the link every card carries, not on a class name: the class it used to check
	// ("summary-card-list") belonged to the hand-written stylesheet deleted on 2026-09-24, and a
	// test pinned to a class name passes or fails on restyling rather than on rendering.
	if !strings.Contains(got, `href="/machines/mch_document/records/doc_1"`) {
		t.Error("cards view rendered no card linking to its record")
	}
	for _, want := range []string{"Kontrak A", "Status", "in_review"} {
		if !strings.Contains(got, want) {
			t.Errorf("cards view is missing %q -- the projection never reached the page", want)
		}
	}
}

// The switcher labels each arrangement by its declared name:, never by its type -- "table"/"cards"
// is the runtime engine's own vocabulary and not a string a viewer should be shown.
func TestMachineBody_viewSwitcherUsesDeclaredNames(t *testing.T) {
	m := docMachine()

	var buf bytes.Buffer
	if err := MachineBody(m, m.Views[0], nil, nil, nil, nil, nil, domain.Actor{ID: "usr_ana"}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, ">Table<") || !strings.Contains(got, ">Cards<") {
		t.Errorf("switcher did not render both declared names:\n%s", got)
	}
	if !strings.Contains(got, "/machines/mch_document?view=vw_document_cards") {
		t.Error("switcher did not link to the other view by query parameter")
	}
}

// A Machine declaring one view (or none) renders no switcher at all, so it looks exactly as it
// did before views: existed.
func TestMachineBody_noSwitcherBelowTwoViews(t *testing.T) {
	m := docMachine()
	m.Views = m.Views[:1]

	var buf bytes.Buffer
	if err := MachineBody(m, m.Views[0], nil, nil, nil, nil, nil, domain.Actor{ID: "usr_ana"}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "view-switcher") {
		t.Error("a single-view Machine rendered a switcher, want none")
	}
}
