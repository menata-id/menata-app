package action

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func TestCanDeleteApprovalStep(t *testing.T) {
	if ok, _ := CanDeleteApprovalStep(map[string]any{FieldStepDecision: DecisionPending}); !ok {
		t.Error("CanDeleteApprovalStep(pending) = false, want true")
	}
	if ok, reason := CanDeleteApprovalStep(map[string]any{FieldStepDecision: DecisionApproved}); ok || reason == "" {
		t.Errorf("CanDeleteApprovalStep(approved) = (%v, %q), want (false, non-empty reason)", ok, reason)
	}
	if ok, reason := CanDeleteApprovalStep(map[string]any{FieldStepDecision: DecisionRejected}); ok || reason == "" {
		t.Errorf("CanDeleteApprovalStep(rejected) = (%v, %q), want (false, non-empty reason)", ok, reason)
	}
}

func TestCanDeleteDocument(t *testing.T) {
	if ok, _ := CanDeleteDocument(DocumentStatusInReview, nil); !ok {
		t.Error("CanDeleteDocument(in_review, no steps) = false, want true")
	}
	if ok, _ := CanDeleteDocument(DocumentStatusInReview, []*data.Record{
		step("rec_1", 1, DecisionPending),
		step("rec_2", 2, DecisionPending),
	}); !ok {
		t.Error("CanDeleteDocument(in_review, all pending steps) = false, want true")
	}
	if ok, reason := CanDeleteDocument(DocumentStatusApproved, nil); ok || reason == "" {
		t.Errorf("CanDeleteDocument(approved) = (%v, %q), want (false, non-empty reason)", ok, reason)
	}
	if ok, reason := CanDeleteDocument(DocumentStatusInReview, []*data.Record{
		step("rec_1", 1, DecisionApproved),
	}); ok || reason == "" {
		t.Errorf("CanDeleteDocument(in_review, one decided step) = (%v, %q), want (false, non-empty reason)", ok, reason)
	}
}

// bound is a Machine cast in one of the approval engine's roles, the way the loader stamps it from
// an Application's own workflow: block. The Application is deliberately *not* called
// app_document_approval -- the engine engages on the declaration, and a fixture using the familiar
// name could not tell that apart from matching it.
func bound(id, role string) *domain.Machine {
	return &domain.Machine{
		ID:             id,
		ApplicationID:  "app_persetujuan",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   role,
	}
}

func TestCanDelete(t *testing.T) {
	stepM := bound(StepMachineID, domain.WorkflowRoleStep)
	docM := bound(DocumentMachineID, domain.WorkflowRoleDocument)

	if ok, reason := CanDelete(stepM, map[string]any{FieldStepDecision: DecisionApproved}, nil); ok || reason == "" {
		t.Errorf("CanDelete(decided step) = (%v, %q), want (false, non-empty reason) -- must dispatch to CanDeleteApprovalStep", ok, reason)
	}
	if ok, reason := CanDelete(docM, map[string]any{FieldDocumentStatus: DocumentStatusApproved}, nil); ok || reason == "" {
		t.Errorf("CanDelete(approved document) = (%v, %q), want (false, non-empty reason) -- must dispatch to CanDeleteDocument", ok, reason)
	}
	if ok, _ := CanDelete(&domain.Machine{ID: "mch_task"}, nil, nil); !ok {
		t.Error("CanDelete(a Machine neither check governs) = false, want true: unrestricted is the default, same as the generic route always was")
	}
	if ok, _ := CanDelete(nil, map[string]any{FieldStepDecision: DecisionApproved}, nil); !ok {
		t.Error("CanDelete(nil) = false, want true -- no Machine means no engine rule to apply")
	}

	// The reason this takes a Machine at all: a Workspace may legitimately hold its own Machine named
	// mch_approval_step, bound to nothing, and this engine's delete rule must not reach its records.
	// Before 2026-09-28 CanDelete switched on the bare id and did exactly that -- and the decided-step
	// rule refuses a delete, so the leak *blocked* real deletions rather than allowing them.
	unbound := &domain.Machine{ID: StepMachineID}
	if ok, reason := CanDelete(unbound, map[string]any{FieldStepDecision: DecisionApproved}, nil); !ok {
		t.Errorf("CanDelete(a Machine merely named %s) = (false, %q), want true -- its Application casts it in no role, so this engine has no rule about it", StepMachineID, reason)
	}
}
