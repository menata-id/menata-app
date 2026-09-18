// Package action implements exactly one workflow shape: sequential/parallel multi-step document
// approval (ROADMAP.md Phase 12, Case 3's core mechanism) -- Approve/Reject as an Action that
// writes a decision onto one Approval Step and, in sequential mode, is only allowed once every
// earlier step is decided.
//
// This is hardcoded to mch_document/mch_approval_step's own field ids, not a generic
// metadata-driven engine, matching this roadmap's own Method: generalize on a second real case
// that needs something similar, never on the first. The design question this phase asked before
// writing any code -- does "sequential" reduce to Phase 4's single-hop Constraint, or does it
// need new Action semantics -- resolves to the latter: Constraint can only check another Machine's
// records pointing at this one, not compare ordinal position among sibling records of the same
// Machine, which is exactly what sequencing needs.
package action

import (
	"menata.app/internal/data"
)

const (
	DocumentMachineID = "mch_document"
	StepMachineID     = "mch_approval_step"

	FieldDocumentMode   = "fld_mode"
	FieldDocumentStatus = "fld_status"
	FieldDocumentFile   = "fld_file"
	FieldStepDocument   = "fld_document"
	FieldStepSequence   = "fld_sequence"
	FieldStepAssignee   = "fld_assignee"
	FieldStepDecision   = "fld_decision"

	// Signature placement fields (ROADMAP.md Phase 15 Step 3/4) -- plain, percentage-based number
	// Fields, not a new Field type. Origin is the top-left of the rendered page image: X grows
	// right, Y grows down, matching (clientX-rect.left)/rect.width the placement screen's own drag
	// handler computes.
	FieldStepSignaturePage = "fld_signature_page"
	FieldStepSignatureX    = "fld_signature_x"
	FieldStepSignatureY    = "fld_signature_y"

	ModeSequential = "sequential"

	DecisionPending  = "pending"
	DecisionApproved = "approved"
	DecisionRejected = "rejected"

	DocumentStatusInReview = "in_review"
	DocumentStatusApproved = "approved"
	DocumentStatusRejected = "rejected"
)

// CanDecide reports whether step is unlocked for a decision yet. In any mode other than
// ModeSequential, every step is always unlocked. In sequential mode, step is locked while any
// sibling step (same FieldStepDocument, lower FieldStepSequence) is still pending.
func CanDecide(mode string, step *data.Record, siblings []*data.Record) bool {
	if mode != ModeSequential {
		return true
	}
	mySeq := sequenceOf(step)
	for _, s := range siblings {
		if s.ID == step.ID {
			continue
		}
		if sequenceOf(s) < mySeq && decisionOf(s) == DecisionPending {
			return false
		}
	}
	return true
}

// DocumentStatus derives a Document's aggregate status from all its Approval Steps: rejected if
// any step is rejected, approved once every step is approved, in_review otherwise (including
// when there are no steps at all yet).
func DocumentStatus(steps []*data.Record) string {
	if len(steps) == 0 {
		return DocumentStatusInReview
	}
	allApproved := true
	for _, s := range steps {
		switch decisionOf(s) {
		case DecisionRejected:
			return DocumentStatusRejected
		case DecisionApproved:
		default:
			allApproved = false
		}
	}
	if allApproved {
		return DocumentStatusApproved
	}
	return DocumentStatusInReview
}

func sequenceOf(r *data.Record) float64 {
	v, _ := r.Values[FieldStepSequence].(float64)
	return v
}

func decisionOf(r *data.Record) string {
	v, _ := r.Values[FieldStepDecision].(string)
	return v
}
