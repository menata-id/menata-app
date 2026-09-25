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
	"fmt"

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
	// FieldStepDecidedByName is the signer's name as it stood when they decided the step -- a
	// snapshot the signed PDF reproduces, not a lookup (see metadata/approval_step.yaml).
	FieldStepDecidedByName = "fld_decided_by_name"
	// FieldStepApproverType selects which kind of actor gates this step, and FieldStepApproverGroup
	// names the Group when it is a Group (CAP-F24, Fase 6c-1). The User half is FieldStepAssignee
	// above -- deliberately not a second person Field, see metadata/approval_step.yaml.
	FieldStepApproverType  = "fld_approver_type"
	FieldStepApproverGroup = "fld_approver_group"
	// FieldStepName is what a step is *for* ("Finance Review"), independent of who holds it --
	// board 10's own step titles, Fase 6b. Optional and unwritten until board 08's wizard collects
	// it (6c); composition.stepLabel falls back to the assignee's name meanwhile.
	FieldStepName = "fld_step_name"

	// Signature placement fields (ROADMAP.md Phase 15 Step 3/4) -- plain, percentage-based number
	// Fields, not a new Field type. Origin is the top-left of the rendered page image: X grows
	// right, Y grows down, matching (clientX-rect.left)/rect.width the placement screen's own drag
	// handler computes.
	FieldStepSignaturePage  = "fld_signature_page"
	FieldStepSignatureX     = "fld_signature_x"
	FieldStepSignatureY     = "fld_signature_y"
	FieldStepSignatureWidth = "fld_signature_width"

	// FieldStepSignatureImage is a one-time signature image captured at Approve time, used only
	// when its assignee chose not to save it as their own reusable mch_signature (Phase 15 Step
	// 5's SignatureMachineID below). See metadata/approval_step.yaml's own doc comment for why
	// this is step-scoped rather than reusable.
	FieldStepSignatureImage = "fld_signature_image"

	// Phase 17: a person's own reusable signature image (Phase 15 Step 5's mch_signature, an
	// ordinary Machine) and the Document's own composited output.
	SignatureMachineID  = "mch_signature"
	FieldSignatureOwner = "fld_owner"
	// FieldDocumentSubmittedBy is who submitted a Document -- stamped from the session by the
	// wizard, and the actor field mch_document's own create Permission checks.
	FieldDocumentSubmittedBy = "fld_submitted_by"
	FieldSignatureImage      = "fld_image"
	FieldDocumentSignedFile  = "fld_signed_file"

	DecisionPending  = "pending"
	DecisionApproved = "approved"
	DecisionRejected = "rejected"

	DocumentStatusDraft    = "draft"
	DocumentStatusInReview = "in_review"
	DocumentStatusApproved = "approved"
	DocumentStatusRejected = "rejected"
)

// decisionOf reads a step's own decision value. Kept after CanDecide moved to
// behavior.CanAct (sequencing is declared metadata now) because delete.go still asks the same
// question for its own, unrelated rule.
func decisionOf(r *data.Record) string {
	v, _ := r.Values[FieldStepDecision].(string)
	return v
}

// DocumentReference is Case 3's own human-readable Document identity (ROADMAP.md's UI mockup
// conformance audit, 2026-09-19) -- reuses Phase 9's per-Machine sort_order rather than a new
// Field, per this roadmap's own admission question ("can an existing primitive express it?").
func DocumentReference(sortOrder int64) string {
	return fmt.Sprintf("DOC-%04d", sortOrder)
}
