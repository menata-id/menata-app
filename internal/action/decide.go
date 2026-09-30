// Package action implements exactly one workflow shape: sequential/parallel multi-step document
// approval (development-history.md Phase 12, Case 3's core mechanism) -- Approve/Reject as an Action that
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
	"menata.app/internal/domain"
)

const (
	// DocumentMachineID and StepMachineID are the ids the template library's own Document Approval
	// gives these two Machines (metadata/document.yaml, metadata/approval_step.yaml). They are what
	// this package's callers use to *name* a Machine -- look one up, query its records, build a URL
	// -- and deliberately no longer how anything decides whether a Machine is this engine's: that
	// question is IsDocument/IsStep below, which read the Application's own declared binding.
	//
	// Two corrections are worth keeping in view here, because each one removed an identity claim
	// these constants were carrying. Until Workspace isolation (2026-09-27) a Machine id was unique
	// across the process, so `m.ID == DocumentMachineID` really did mean "*the* Document Approval
	// Document" -- until a generated "Document Tracking" Application took the name the same night
	// and panicked its way through code that had taken the id as proof. The fix added the
	// Application id to the comparison, which held only while the Application was named
	// app_document_approval: renaming it, or installing a second approval Application beside it,
	// stopped the engine waking at all (audit Gap C). domain.Workflow is what answers both.
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

	// Signature placement fields (development-history.md Phase 15 Step 3/4) -- plain, percentage-based number
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

	// CAP-V28 (ROADMAP.md, 2026-09-27): a saved default approval flow per Document Type.
	// TemplateStepMachineID's own Field ids deliberately reuse StepMachineID's own strings --
	// ids are Machine-scoped, so there is no collision, and it keeps the parallel between a real
	// step and a template step legible.
	TemplateMachineID     = "mch_approval_flow_template"
	TemplateStepMachineID = "mch_approval_flow_template_step"

	FieldTemplateDocumentType      = "fld_document_type"
	FieldTemplateMode              = "fld_mode"
	FieldTemplateStepTemplate      = "fld_template"
	FieldTemplateStepSequence      = "fld_sequence"
	FieldTemplateStepApproverType  = "fld_approver_type"
	FieldTemplateStepAssignee      = "fld_assignee"
	FieldTemplateStepApproverGroup = "fld_approver_group"
)

// IsDocument and IsStep answer "is this Machine one this engine acts on, and in which role" -- the
// question every generic screen and handler needs before opting into approval behaviour, and the
// single seam all 29 of those call sites go through, which is why the answer could be rewritten
// here without touching one of them.
//
// What they read is the Application's own declaration (domain.Workflow, stamped onto the Machine at
// load): this Machine's Application says it runs the document_approval engine, and says this
// Machine is its document, or its step. Nothing here matches a name. An approval Application may
// therefore be called anything, its Machines may be called anything, and a Workspace may install a
// second one beside the first -- all three of which were impossible while these predicates matched
// the literals `app_document_approval` / `mch_document` / `mch_approval_step` (ROADMAP.md
// "Document Approval: closing the last three layers", Stage A).
//
// A Machine whose Application binds no engine -- every plain CRUD Application, and the
// Workspace-level mch_user/mch_activity/mch_notification that no Application claims at all -- has
// both fields empty and is correctly neither.
func IsDocument(m *domain.Machine) bool {
	return isWorkflowRole(m, domain.WorkflowRoleDocument)
}

func IsStep(m *domain.Machine) bool {
	return isWorkflowRole(m, domain.WorkflowRoleStep)
}

// IsSignatureStore reports whether m is the Machine its Application casts as the engine's reusable
// signature store -- an optional role, so a Workspace whose approval Application casts none has no
// Machine answering true, and the one-time image captured at decision time is the whole feature.
//
// Exported for internal/execution, which cannot read the Workspace off ctx (it must not import
// internal/rendering) and so asks this package, which owns the binding predicates, over the machines
// map it already receives.
func IsSignatureStore(m *domain.Machine) bool {
	return isWorkflowRole(m, domain.WorkflowRoleSignature)
}

func isWorkflowRole(m *domain.Machine, role string) bool {
	return m != nil && m.WorkflowEngine == domain.WorkflowEngineDocumentApproval && m.WorkflowRole == role
}

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
