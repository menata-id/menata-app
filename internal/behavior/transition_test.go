package behavior

import (
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
