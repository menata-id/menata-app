package action

import (
	"testing"

	"menata.app/internal/data"
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
