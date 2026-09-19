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
	if err := RecordRow(m, r, nil, "rec_user1").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "hx-delete") {
		t.Error(`RecordRow rendered a Delete button for an already-decided Approval Step -- action.CanDelete should have refused it regardless of AllowsAction`)
	}
}
