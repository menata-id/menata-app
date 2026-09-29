package behavior

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

func stepMachineWithRemovalBlock() *domain.Machine {
	return &domain.Machine{
		ID: "mch_approval_step", Name: "Approval Step",
		// The Fields the removal block below names. A block gating on Fields the Machine does not declare
		// is one the loader refuses -- found by running this fixture through metadata.Validate (2026-09-29).
		Fields: []domain.Field{
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson},
			{ID: "fld_decision", Name: "Decision", Type: domain.FieldTypeStatus, Options: []string{"pending", "approved", "rejected"}},
		},
		MemberRemovalBlocks: []domain.MemberRemovalBlock{
			{
				ID:         "blk_step_pending",
				ActorField: "fld_assignee",
				Condition:  expression.Comparison{Field: "fld_decision", Op: expression.OpEquals, Value: "pending"},
				Reason:     "has a pending approval step",
			},
		},
	}
}

func TestMatchedMemberRemovalBlocks_firesOnMatchingActorAndCondition(t *testing.T) {
	records := []*data.Record{
		{ID: "stp_1", Values: map[string]any{"fld_assignee": "usr_ana", "fld_decision": "pending"}},
	}
	got := MatchedMemberRemovalBlocks(stepMachineWithRemovalBlock(), "usr_ana", records)
	if len(got) != 1 || got[0].ID != "blk_step_pending" {
		t.Fatalf("MatchedMemberRemovalBlocks() = %v, want exactly blk_step_pending", got)
	}
}

func TestMatchedMemberRemovalBlocks_skipsDifferentActor(t *testing.T) {
	records := []*data.Record{
		{ID: "stp_1", Values: map[string]any{"fld_assignee": "usr_budi", "fld_decision": "pending"}},
	}
	if got := MatchedMemberRemovalBlocks(stepMachineWithRemovalBlock(), "usr_ana", records); len(got) != 0 {
		t.Errorf("MatchedMemberRemovalBlocks() = %v, want none: the step is assigned to someone else", got)
	}
}

func TestMatchedMemberRemovalBlocks_skipsWhenConditionDoesNotMatch(t *testing.T) {
	records := []*data.Record{
		{ID: "stp_1", Values: map[string]any{"fld_assignee": "usr_ana", "fld_decision": "approved"}},
	}
	if got := MatchedMemberRemovalBlocks(stepMachineWithRemovalBlock(), "usr_ana", records); len(got) != 0 {
		t.Errorf("MatchedMemberRemovalBlocks() = %v, want none: the step is already decided", got)
	}
}

func TestMatchedMemberRemovalBlocks_emptyWhenMachineDeclaresNone(t *testing.T) {
	m := &domain.Machine{ID: "mch_task"}
	records := []*data.Record{
		{ID: "tsk_1", Values: map[string]any{"fld_assignee": "usr_ana"}},
	}
	if got := MatchedMemberRemovalBlocks(m, "usr_ana", records); len(got) != 0 {
		t.Errorf("MatchedMemberRemovalBlocks() = %v, want none: this machine declares no blocks_member_removal", got)
	}
}

func TestMatchedMemberRemovalBlocks_firesOnceEvenWithSeveralMatchingRecords(t *testing.T) {
	records := []*data.Record{
		{ID: "stp_1", Values: map[string]any{"fld_assignee": "usr_ana", "fld_decision": "pending"}},
		{ID: "stp_2", Values: map[string]any{"fld_assignee": "usr_ana", "fld_decision": "pending"}},
	}
	got := MatchedMemberRemovalBlocks(stepMachineWithRemovalBlock(), "usr_ana", records)
	if len(got) != 1 {
		t.Fatalf("MatchedMemberRemovalBlocks() = %v, want exactly one block even with two matching records -- one reason is enough", got)
	}
}
