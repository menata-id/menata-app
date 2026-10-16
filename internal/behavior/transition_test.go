package behavior

import (
	"math"
	"strings"
	"testing"
	"time"

	"menata.app/internal/domain"
)

// stepMachine mirrors metadata/approval_step.yaml's own shape: one status Field governed by two
// decide-only edges, plus a second status Field nothing governs.
//
// The ungoverned Field is not filler. It is the assertion that declaring a state model for one
// Field leaves every other one alone -- without it, this primitive would silently freeze every
// status Field in the manifest the moment a single Machine declared a transition.
func stepMachine() *domain.Machine {
	return &domain.Machine{
		ID: "mch_approval_step", Name: "Approval Step",
		Fields: []domain.Field{
			{ID: "fld_decision", Type: domain.FieldTypeStatus, Options: []string{"pending", "approved", "rejected"}},
			{ID: "fld_approver_type", Type: domain.FieldTypeStatus, Options: []string{"User", "Group"}},
		},
		Transitions: []domain.Transition{
			{ID: "trn_step_approve", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionDecide},
			{ID: "trn_step_reject", Name: "Reject", Field: "fld_decision", From: "pending", To: "rejected", Action: domain.ActionDecide},
		},
	}
}

func TestCheckTransitions(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		current map[string]any
		next    map[string]any
		wantErr bool
	}{
		{
			name:    "a declared edge through its own action passes",
			action:  domain.ActionDecide,
			current: map[string]any{"fld_decision": "pending"},
			next:    map[string]any{"fld_decision": "approved"},
		},
		{
			name:    "the same edge through another action is refused",
			action:  domain.ActionEdit,
			current: map[string]any{"fld_decision": "pending"},
			next:    map[string]any{"fld_decision": "approved"},
			wantErr: true,
		},
		{
			// The hole sequencing never covered: on a parallel Document nothing stopped an
			// already-approved step being decided again.
			name:    "an edge leaving a final value is not declared, so it is refused",
			action:  domain.ActionDecide,
			current: map[string]any{"fld_decision": "approved"},
			next:    map[string]any{"fld_decision": "rejected"},
			wantErr: true,
		},
		{
			// The generic update route rewrites a whole record, so every status Field is present
			// on every write. If an unchanged value counted as a transition, no edit would ever
			// succeed on a Machine that declares one.
			name:    "a value that does not change is not a transition",
			action:  domain.ActionEdit,
			current: map[string]any{"fld_decision": "approved"},
			next:    map[string]any{"fld_decision": "approved"},
		},
		{
			name:    "a status field no transition mentions still moves freely",
			action:  domain.ActionEdit,
			current: map[string]any{"fld_decision": "pending", "fld_approver_type": "User"},
			next:    map[string]any{"fld_decision": "pending", "fld_approver_type": "Group"},
		},
		{
			name:    "a write that does not mention the field at all is untouched",
			action:  domain.ActionEdit,
			current: map[string]any{"fld_decision": "pending"},
			next:    map[string]any{"fld_approver_type": "Group"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckTransitions(stepMachine(), tt.action, tt.current, tt.next)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckTransitions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// A Machine declaring no transitions restricts nothing -- metadata describes exceptions, not
// defaults (001 Principle #6). This is what lets the gate run on every write in the manifest
// without any Machine but one noticing.
func TestCheckTransitions_undeclaredModelRestrictsNothing(t *testing.T) {
	m := stepMachine()
	m.Transitions = nil
	err := CheckTransitions(m, domain.ActionEdit,
		map[string]any{"fld_decision": "approved"}, map[string]any{"fld_decision": "pending"})
	if err != nil {
		t.Errorf("CheckTransitions() = %v, want nil for a machine that declares no state model", err)
	}
}

// A derived Field's edges name no Action, so no route may perform them -- the rule that stops the
// generic update route writing a Document straight to `approved` with no step decided.
func TestCheckTransitions_actionlessEdgeIsRefusedThroughEveryAction(t *testing.T) {
	m := &domain.Machine{
		ID:          "mch_document",
		Fields:      []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"in_review", "approved"}}},
		Transitions: []domain.Transition{{ID: "trn_document_approved", Field: "fld_status", From: "in_review", To: "approved"}},
	}
	for _, act := range []string{domain.ActionEdit, domain.ActionDecide, domain.ActionDelete} {
		if err := CheckTransitions(m, act, map[string]any{"fld_status": "in_review"}, map[string]any{"fld_status": "approved"}); err == nil {
			t.Errorf("CheckTransitions(%s) = nil, want a refusal: a derived field is set by no action", act)
		}
	}
	// ...and the runtime's own write does not go through this gate at all (internal/web's
	// rollUpParentStatus writes through the store directly), which is what keeps the rollup
	// working while the route is closed.
}

func TestMovedTransitions(t *testing.T) {
	m := stepMachine()
	if got := MovedTransitions(m, map[string]any{"fld_decision": "pending"}, map[string]any{"fld_decision": "approved"}); len(got) != 1 || got[0].ID != "trn_step_approve" {
		t.Errorf("a declared move = %+v, want trn_step_approve", got)
	}
	for name, next := range map[string]map[string]any{
		"a value that does not change": {"fld_decision": "pending"},
		"a Field the write omits":      {},
		"an undeclared move":           {"fld_decision": "pending-again"},
		"an ungoverned Field":          {"fld_approver_type": "Group"},
	} {
		if got := MovedTransitions(m, map[string]any{"fld_decision": "pending", "fld_approver_type": "User"}, next); len(got) != 0 {
			t.Errorf("%s: MovedTransitions = %+v, want none", name, got)
		}
	}
}

func TestApplyTransitionSets(t *testing.T) {
	m := &domain.Machine{
		Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"open", "done"}}, {ID: "fld_when", Type: domain.FieldTypeDate}},
		Transitions: []domain.Transition{
			{ID: "trn_done", Field: "fld_status", From: "open", To: "done", Action: domain.ActionEdit, Sets: map[string]string{"fld_when": domain.TransitionSourceToday}},
			{ID: "trn_reopen", Field: "fld_status", From: "done", To: "open", Action: domain.ActionEdit},
		},
	}
	now := time.Date(2026, 10, 16, 23, 59, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		current, next map[string]any
		want          any
	}{
		"the edge is taken":             {map[string]any{"fld_status": "open"}, map[string]any{"fld_status": "done"}, "2026-10-16"},
		"a typed date is overridden":    {map[string]any{"fld_status": "open"}, map[string]any{"fld_status": "done", "fld_when": "1999-01-01"}, "2026-10-16"},
		"the status is left alone":      {map[string]any{"fld_status": "open"}, map[string]any{"fld_status": "open"}, nil},
		"an edge that sets nothing":     {map[string]any{"fld_status": "done", "fld_when": "2026-01-01"}, map[string]any{"fld_status": "open", "fld_when": "2026-01-01"}, "2026-01-01"},
		"an edit that omits the status": {map[string]any{"fld_status": "open"}, map[string]any{}, nil},
	} {
		ApplyTransitionSets(m, tc.current, tc.next, now)
		if got := tc.next["fld_when"]; got != tc.want {
			t.Errorf("%s: fld_when = %v, want %v", name, got, tc.want)
		}
	}
	if !HasTransitionSets(m) || HasTransitionSets(&domain.Machine{Transitions: []domain.Transition{{ID: "t"}}}) {
		t.Error("HasTransitionSets misreads the edges")
	}
}

func TestPlanGenerated(t *testing.T) {
	g := &domain.GenerateSchedule{Machine: "mch_inst", CountField: "fld_months", LinkField: "fld_loan", DueField: "fld_due", DueEveryDays: 30, AmountField: "fld_amount", AmountFrom: "fld_total"}
	m := &domain.Machine{
		Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"approved", "paid_out"}}},
		Transitions: []domain.Transition{
			{ID: "trn_pay", Name: "Pay out", Field: "fld_status", From: "approved", To: "paid_out", Action: domain.ActionEdit, Generates: g},
		},
	}
	now := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	cur := map[string]any{"fld_status": "approved"}
	take := func(months, total any) map[string]any {
		return map[string]any{"fld_status": "paid_out", "fld_months": months, "fld_total": total}
	}

	b, err := PlanGenerated(m, "rec_loan", cur, take(3.0, 1000.0), now)
	if err != nil || b == nil || b.Machine != "mch_inst" || b.LinkField != "fld_loan" || len(b.Rows) != 3 {
		t.Fatalf("PlanGenerated = %+v, %v", b, err)
	}
	var sum float64
	for i, row := range b.Rows {
		if row["fld_loan"] != "rec_loan" {
			t.Errorf("row %d points at %v, not the loan", i+1, row["fld_loan"])
		}
		sum += row["fld_amount"].(float64)
	}
	if b.Rows[0]["fld_amount"] != 333.33 || b.Rows[1]["fld_amount"] != 333.33 || b.Rows[2]["fld_amount"] != 333.34 {
		t.Errorf("amounts = %v %v %v, want 333.33 333.33 333.34 (the last takes the remainder)", b.Rows[0]["fld_amount"], b.Rows[1]["fld_amount"], b.Rows[2]["fld_amount"])
	}
	if math.Abs(sum-1000) > 1e-9 {
		t.Errorf("the shares add up to %v, want exactly 1000", sum)
	}
	if got := []any{b.Rows[0]["fld_due"], b.Rows[1]["fld_due"], b.Rows[2]["fld_due"]}; got[0] != "2026-03-02" || got[1] != "2026-04-01" || got[2] != "2026-05-01" {
		t.Errorf("due dates = %v, want 30 days apart counted from the day the edge is taken, first one a period on", got)
	}
	if b, err := PlanGenerated(m, "r", cur, map[string]any{"fld_status": "approved"}, now); b != nil || err != nil {
		t.Errorf("a write that takes no edge planned %+v, %v", b, err)
	}
	if b, err := PlanGenerated(m, "r", map[string]any{"fld_status": "paid_out"}, take(3.0, 9.0), now); b != nil || err != nil {
		t.Errorf("a write that does not move the status planned %+v, %v", b, err)
	}
	for name, in := range map[string]map[string]any{
		"no count":           take(nil, 10.0),
		"a fractional count": take(2.5, 10.0),
		"a zero count":       take(0.0, 10.0),
		"too many":           take(float64(domain.MaxGeneratedRecords+1), 10.0),
		"a text count":       take("3", 10.0),
		"no amount":          take(3.0, nil),
		"a text amount":      take(3.0, "10"),
	} {
		if b, err := PlanGenerated(m, "r", cur, in, now); err == nil || b != nil {
			t.Errorf("%s: planned %+v, %v; want a refusal", name, b, err)
		}
	}
	if b, err := PlanGenerated(m, "r", cur, take(float64(domain.MaxGeneratedRecords), 1200.0), now); err != nil || len(b.Rows) != domain.MaxGeneratedRecords {
		t.Errorf("the maximum count was refused: %v", err)
	}
	if !HasGenerates(m) || HasGenerates(&domain.Machine{Transitions: []domain.Transition{{ID: "t"}}}) {
		t.Error("HasGenerates misreads the edges")
	}
}

func TestCheckBalance(t *testing.T) {
	rule := &domain.BalanceRule{Machine: "mch_line", Via: "fld_entry", Debit: "fld_debit", Credit: "fld_credit", Reason: "Not balanced."}
	line := func(d, c any) map[string]any { return map[string]any{"fld_debit": d, "fld_credit": c} }
	for name, tc := range map[string]struct {
		lines []map[string]any
		want  string // "" means balanced
	}{
		"equal totals":           {[]map[string]any{line(500000.0, 0.0), line(0.0, 500000.0)}, ""},
		"several lines a side":   {[]map[string]any{line(300.0, nil), line(200.0, nil), line(nil, 500.0)}, ""},
		"cents rounding":         {[]map[string]any{line(0.1, nil), line(0.2, nil), line(nil, 0.3)}, ""},
		"debit larger":           {[]map[string]any{line(500.0, 0.0), line(0.0, 400.0)}, "debit 500.00, credit 400.00"},
		"credit larger":          {[]map[string]any{line(400.0, 0.0), line(0.0, 500.0)}, "debit 400.00, credit 500.00"},
		"a cent apart":           {[]map[string]any{line(10.01, nil), line(nil, 10.0)}, "Not balanced."},
		"no lines at all":        {nil, "nothing to post"},
		"lines of zero":          {[]map[string]any{line(0.0, 0.0)}, "nothing to post"},
		"a non-number is a zero": {[]map[string]any{line("x", nil), line(nil, 5.0)}, "debit 0.00, credit 5.00"},
	} {
		err := CheckBalance(rule, tc.lines)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: refused: %v", name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
	if err := CheckBalance(&domain.BalanceRule{Debit: "fld_debit", Credit: "fld_credit"}, []map[string]any{line(1.0, nil)}); err == nil || !strings.Contains(err.Error(), "do not balance") {
		t.Errorf("a rule with no reason must still say why: %v", err)
	}
}

func TestBalancedEdgesAreOnlyTheOnesTheWriteTakes(t *testing.T) {
	m := &domain.Machine{ID: "mch_entry", Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"draft", "posted"}}},
		Transitions: []domain.Transition{{ID: "trn_post", Field: "fld_status", From: "draft", To: "posted", Action: domain.ActionEdit,
			Balanced: &domain.BalanceRule{Machine: "mch_line"}},
			{ID: "trn_void", Field: "fld_status", From: "posted", To: "void", Action: domain.ActionEdit}}}
	m.Fields[0].Options = append(m.Fields[0].Options, "void")
	if !HasBalanced(m) {
		t.Fatal("HasBalanced missed a balanced edge")
	}
	if got := BalancedEdges(m, map[string]any{"fld_status": "draft"}, map[string]any{"fld_status": "posted"}); len(got) != 1 {
		t.Errorf("taking the edge: %d balanced edges, want 1", len(got))
	}
	if got := BalancedEdges(m, map[string]any{"fld_status": "draft"}, map[string]any{"fld_status": "draft"}); len(got) != 0 {
		t.Errorf("an edit that leaves the status alone took %d balanced edges", len(got))
	}
	if got := BalancedEdges(m, map[string]any{"fld_status": "posted"}, map[string]any{"fld_status": "void"}); len(got) != 0 {
		t.Errorf("an edge declaring no balance was listed as balanced: %d", len(got))
	}
	if HasBalanced(&domain.Machine{}) {
		t.Error("a Machine with no edges reports a balanced one")
	}
}
