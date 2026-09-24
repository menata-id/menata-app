package composition

import (
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/data"
)

// groupStep is step's own group-held counterpart: fld_approver_type: Group, no fld_assignee --
// CAP-F24's other arm, the one stepBelongsTo's own doc comment names.
func groupStep(id, docID, decision string, seq float64, group string) *data.Record {
	return rec(id, map[string]any{
		action.FieldStepDocument:      docID,
		action.FieldStepDecision:      decision,
		action.FieldStepSequence:      seq,
		action.FieldStepApproverType:  "Group",
		action.FieldStepApproverGroup: group,
	})
}

// A sequential Document locks step 2 behind step 1 the same way it does for the Inbox
// (TestBuildInbox_SequentialLocksLaterSteps) -- but unlike the Inbox, the locked step still
// belongs on this screen: it is "not yet your turn", not absent.
func TestBuildAssigned_SequentialStepIsNotYetYourTurn(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "sequential", "")}
	steps := []*data.Record{
		step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1),
		step("stp_2", "doc_1", "usr_ana", action.DecisionPending, 2),
	}

	got := buildAssigned(steps, docs, nil, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 1 {
		t.Fatalf("usr_ana's own step must appear even though it is locked; got %d row(s)", len(got.Rows))
	}
	if got.Rows[0].DecisionKey != AssignedNotYet {
		t.Errorf("DecisionKey = %q, want %q", got.Rows[0].DecisionKey, AssignedNotYet)
	}
	if got.NotYetCount != 1 || got.WaitingCount != 0 {
		t.Errorf("counts = {NotYet:%d Waiting:%d}, want {1 0}", got.NotYetCount, got.WaitingCount)
	}
}

// The same step, actionable now (parallel, or sequential with nothing ahead of it), is "waiting
// for your decision" instead.
func TestBuildAssigned_ActionableStepIsWaiting(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{
		step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1),
		step("stp_2", "doc_1", "usr_ana", action.DecisionPending, 2),
	}

	got := buildAssigned(steps, docs, nil, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 1 || got.Rows[0].DecisionKey != AssignedWaiting {
		t.Fatalf("got %+v, want one row keyed %q", got.Rows, AssignedWaiting)
	}
	if got.WaitingCount != 1 {
		t.Errorf("WaitingCount = %d, want 1", got.WaitingCount)
	}
	if got.Rows[0].DecisionLabel != "Waiting for your decision" {
		t.Errorf("DecisionLabel = %q", got.Rows[0].DecisionLabel)
	}
}

// Approved and rejected steps stay on the screen -- Assigned to me is a record of everything that
// has ever asked, not a worklist of what still needs deciding (that is ApprovalInbox's own job).
func TestBuildAssigned_DecidedStepsStay(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{
		step("stp_ok", "doc_1", "usr_ana", action.DecisionApproved, 1),
		step("stp_no", "doc_1", "usr_ana", action.DecisionRejected, 2),
	}

	got := buildAssigned(steps, docs, nil, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 2 {
		t.Fatalf("got %d row(s), want 2 -- a decided step must not disappear", len(got.Rows))
	}
	if got.ApprovedCount != 1 || got.RejectedCount != 1 {
		t.Errorf("counts = {Approved:%d Rejected:%d}, want {1 1}", got.ApprovedCount, got.RejectedCount)
	}
}

// A step held by a Group the viewer belongs to is theirs -- CAP-F24's second arm, and the reason
// stepBelongsTo exists rather than a bare fld_assignee comparison. The row names the Group.
func TestBuildAssigned_GroupHeldStepIsMineWhenIAmAMember(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "sequential", "")}
	steps := []*data.Record{
		step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1),
		groupStep("stp_2", "doc_1", action.DecisionPending, 2, "grp_legal"),
	}
	myGroups := map[string]string{"grp_legal": "Legal Group"}

	got := buildAssigned(steps, docs, nil, personNames, myGroups, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 1 {
		t.Fatalf("got %d row(s), want 1 (the Group-held step)", len(got.Rows))
	}
	if got.Rows[0].DecisionKey != AssignedNotYet {
		t.Fatalf("DecisionKey = %q, want %q (locked behind step 1)", got.Rows[0].DecisionKey, AssignedNotYet)
	}
	if want := "Not yet your turn — step 2 of 2 · via Legal Group"; got.Rows[0].DecisionLabel != want {
		t.Errorf("DecisionLabel = %q, want %q", got.Rows[0].DecisionLabel, want)
	}
}

// A step held by a Group the viewer does NOT belong to is not theirs -- the negative case, without
// which "is a member of *some* group" could pass this screen open.
func TestBuildAssigned_GroupHeldStepIsNotMineWhenIAmNotAMember(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{groupStep("stp_1", "doc_1", action.DecisionPending, 1, "grp_legal")}

	got := buildAssigned(steps, docs, nil, personNames, map[string]string{"grp_finance": "Finance"}, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 0 {
		t.Errorf("got %d row(s), want 0 -- usr_ana is not in grp_legal", len(got.Rows))
	}
}

// FROM/REQUESTED come from the submission activity, the same source buildInbox's own Submitter/
// SubmittedAt use (TestBuildInbox_SubmitterFromEarliestEvent) -- one resolver, not two that could
// name a different submitter for the same Document on two different screens.
func TestBuildAssigned_FromAndRequestedFromActivity(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_ana", action.DecisionPending, 1)}
	activities := []*data.Record{event("doc_1", "usr_budi", at(3))}

	got := buildAssigned(steps, docs, activities, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 1 {
		t.Fatalf("got %d row(s), want 1", len(got.Rows))
	}
	if got.Rows[0].From != "Budi" {
		t.Errorf("From = %q, want %q", got.Rows[0].From, "Budi")
	}
	if want := "3 Sep 2026"; got.Rows[0].RequestedAt != want {
		t.Errorf("RequestedAt = %q, want %q", got.Rows[0].RequestedAt, want)
	}
}

// "Newest request first" (board 07b's own subtitle): two Documents, two submission dates, and the
// later one must lead.
func TestBuildAssigned_NewestRequestFirst(t *testing.T) {
	docs := []*data.Record{
		doc("doc_old", "Older", "parallel", ""),
		doc("doc_new", "Newer", "parallel", ""),
	}
	steps := []*data.Record{
		step("stp_old", "doc_old", "usr_ana", action.DecisionPending, 1),
		step("stp_new", "doc_new", "usr_ana", action.DecisionPending, 1),
	}
	activities := []*data.Record{
		event("doc_old", "usr_budi", at(1)),
		event("doc_new", "usr_budi", at(9)),
	}

	got := buildAssigned(steps, docs, activities, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 2 {
		t.Fatalf("got %d row(s), want 2", len(got.Rows))
	}
	if got.Rows[0].Title != "Newer" || got.Rows[1].Title != "Older" {
		t.Errorf("order = [%q, %q], want [Newer, Older]", got.Rows[0].Title, got.Rows[1].Title)
	}
}

// An orphaned step -- its own Document missing from the fetched set -- is skipped rather than
// rendered with a blank title, the same defensive read buildInbox's own
// TestBuildInbox_OrphanStepIsSkipped already establishes for the Inbox.
func TestBuildAssigned_OrphanStepIsSkipped(t *testing.T) {
	steps := []*data.Record{step("stp_1", "doc_missing", "usr_ana", action.DecisionPending, 1)}

	got := buildAssigned(steps, nil, nil, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 0 {
		t.Errorf("got %d row(s), want 0 -- the step's own Document does not exist in this read", len(got.Rows))
	}
}

// A step belonging to someone else entirely (no direct match, no shared Group) must not appear --
// the baseline every positive case above is checked against.
func TestBuildAssigned_SkipsOtherPeoplesSteps(t *testing.T) {
	docs := []*data.Record{doc("doc_1", "Contract", "parallel", "")}
	steps := []*data.Record{step("stp_1", "doc_1", "usr_budi", action.DecisionPending, 1)}

	got := buildAssigned(steps, docs, nil, personNames, nil, "usr_ana", at(10), stepMachineForTest())
	if len(got.Rows) != 0 {
		t.Errorf("got %d row(s), want 0", len(got.Rows))
	}
}
