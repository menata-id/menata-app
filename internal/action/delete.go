package action

import "menata.app/internal/data"

// CanDeleteApprovalStep reports whether an Approval Step may still be deleted through the
// generic delete route. A decided step (approved or rejected) is the audit trail Phase 16
// (Permission) and Phase 17 (signature compositing) exist to protect -- deleting it would erase
// the record of who actually approved or rejected the document, so it is blocked outright rather
// than left to the generic route's own no-op authorization.
func CanDeleteApprovalStep(values map[string]any) (ok bool, reason string) {
	if decision, _ := values[FieldStepDecision].(string); decision != DecisionPending {
		return false, "cannot delete an approval step that has already been decided"
	}
	return true, ""
}

// CanDeleteDocument reports whether a Document may still be deleted through the generic delete
// route: not once it has reached DocumentStatusApproved (a signed, composited PDF may already
// exist for it, Phase 17), and not while any of its own Approval Steps has a real decision on it
// -- same reasoning as CanDeleteApprovalStep, checked here too since a Document delete cascades
// to its steps (data.Store.DeleteRecord does not enforce this on its own).
func CanDeleteDocument(status string, steps []*data.Record) (ok bool, reason string) {
	if status == DocumentStatusApproved {
		return false, "cannot delete an approved document"
	}
	for _, s := range steps {
		if decisionOf(s) != DecisionPending {
			return false, "cannot delete a document with at least one decided approval step"
		}
	}
	return true, ""
}
