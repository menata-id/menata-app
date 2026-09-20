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

// TestSignaturePlacementPage_carriesEveryStepFieldForward is the regression test for a silent
// data-loss trap found while planning Fase 6c-2, before it could fire.
//
// The placement screen's marker/place/width forms PUT to the *generic* record route, which
// rewrites the record from the submitted form -- and data.ValuesFromForm drops a field that is
// absent. So every Field a step can hold must be carried forward as a hidden input, or the next
// drag erases it. Not with an error: nothing is invalid, the field simply stops existing.
//
// fld_step_name, fld_approver_type and fld_approver_group were declared in Fase 6b/6c-1 while
// nothing wrote them, so their absence here cost nothing and was invisible. Fase 6c-2's wizard is
// the first thing to fill them; without this fix, the first marker drag would have quietly turned
// a Group-held step back into an ungated one. There were no tests on this file at all, which is
// why the trap survived two phases.
func TestSignaturePlacementPage_carriesEveryStepFieldForward(t *testing.T) {
	step := &data.Record{ID: "stp_1", Values: map[string]any{
		action.FieldStepDocument:      "doc_1",
		action.FieldStepSequence:      float64(2),
		action.FieldStepName:          "Legal Review",
		action.FieldStepApproverType:  domain.ActorKindGroup,
		action.FieldStepApproverGroup: "grp_legal",
		action.FieldStepDecision:      action.DecisionPending,
	}}
	document := &data.Record{ID: "doc_1", Values: map[string]any{"fld_title": "Vendor Contract Q3"}}
	stepMachine := &domain.Machine{ID: action.StepMachineID, Name: "Approval Step"}

	var buf bytes.Buffer
	c := SignaturePlacementPage(document, []*data.Record{step}, nil, 1, 1, stepMachine, domain.Actor{ID: "usr_ana"})
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()

	// Every Field the wizard can write must appear as a hidden input carrying its current value.
	for field, value := range map[string]string{
		action.FieldStepName:          "Legal Review",
		action.FieldStepApproverType:  domain.ActorKindGroup,
		action.FieldStepApproverGroup: "grp_legal",
		action.FieldStepDocument:      "doc_1",
		action.FieldStepDecision:      action.DecisionPending,
	} {
		if !strings.Contains(html, `name="`+field+`"`) {
			t.Errorf("no hidden input for %s -- the next marker drag would erase it, silently", field)
			continue
		}
		if !strings.Contains(html, `value="`+value+`"`) {
			t.Errorf("%s is present but does not carry %q forward", field, value)
		}
	}
}

// The counterpart claim, stated as a test so the reason above is not just prose: a step with none
// of those Fields set still renders, with empty hidden inputs rather than a panic or a missing
// form. This is every Approval Step written before Fase 6c-2.
func TestSignaturePlacementPage_rendersAStepWithNoApproverFields(t *testing.T) {
	step := &data.Record{ID: "stp_1", Values: map[string]any{
		action.FieldStepDocument: "doc_1",
		action.FieldStepSequence: float64(1),
		action.FieldStepAssignee: "usr_rina",
		action.FieldStepDecision: action.DecisionPending,
	}}
	document := &data.Record{ID: "doc_1", Values: map[string]any{"fld_title": "Legacy Document"}}

	var buf bytes.Buffer
	c := SignaturePlacementPage(document, []*data.Record{step}, nil, 1, 1,
		&domain.Machine{ID: action.StepMachineID, Name: "Approval Step"}, domain.Actor{ID: "usr_rina"})
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), `name="`+action.FieldStepApproverType+`"`) {
		t.Error("the hidden input must be rendered even when unset, so the field's absence is carried as absence rather than dropped")
	}
}
