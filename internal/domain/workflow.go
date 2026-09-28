package domain

// Workflow is how an Application says which runtime workflow engine it runs on, and which of its
// own Machines plays each role in that engine (ROADMAP.md "Document Approval: closing the last
// three layers", Stage A; the audit is menata-app-document's
// audits/2026-09-28-kajian-metadata-based-document-approval.md, Gap C).
//
// It replaces a literal. internal/action's approval engine used to decide "is this Machine mine"
// by matching the Application id `app_document_approval` and the Machine ids `mch_document` /
// `mch_approval_step` -- so the engine woke for exactly one Application name holding exactly two
// Machine names. 001 Principle #2 puts application *behavior* in the runtime, and an engine that
// only recognises one set of names is behavior owned by one application: the same approval
// mechanics could not be renamed, varied, or installed a second time under a different name, even
// though nothing about them is specific to those names.
//
// Declared on the Application rather than on the Machine, because it is a statement about a *set*
// of Machines working together -- which one is the document, which one is the step -- and a Machine
// declaring its own role would leave "whose engine?" unanswered, which is the half that went wrong
// (see Machine.ApplicationID). The loader stamps each named Machine's WorkflowEngine/WorkflowRole
// from here (internal/metadata.stampWorkflowRoles), so the declaration lives in exactly one place
// and the index over it is derived, never retyped (001 Principle #8).
//
// Two Applications in one Workspace may bind the same engine, deliberately: that is the capability
// this exists to give, and each one's own Machines then answer for themselves. Nothing here is
// process-wide -- a Workspace's own copy of an Application declares its own binding, so two
// Workspaces may bind the same engine to Machines named differently.
type Workflow struct {
	// Engine is one of KnownWorkflowEngines.
	Engine string
	// Roles maps each role name the engine requires to the id of the Machine playing it, selected
	// from this Application's own `machines:` list (validated at load, since a role filled by
	// another Application's Machine would hand the engine records it does not own).
	Roles map[string]string
}

// MachineForRole reports the Machine id playing one role of this binding, or "" when this
// Application declares no binding or no such role.
func (w *Workflow) MachineForRole(role string) string {
	if w == nil {
		return ""
	}
	return w.Roles[role]
}

// WorkflowEngineDocumentApproval is the sequential/parallel multi-step approval engine
// internal/action implements: a document whose approval steps are decided in order or all at once
// (006-runtime-model.md names Approve and Reject as Actions; ROADMAP.md Phase 12).
const WorkflowEngineDocumentApproval = "document_approval"

const (
	// WorkflowRoleDocument is the Machine whose records are the thing being approved.
	WorkflowRoleDocument = "document"
	// WorkflowRoleStep is the Machine whose records are the individual approval steps, one per
	// approver, ordered when the document asks for sequential mode.
	WorkflowRoleStep = "step"
)

// KnownWorkflowEngines is the closed set of engines a Workflow may name, mapped to the roles each
// one requires -- the same static-seam discipline KnownActions/KnownServices already establish
// (007 §14), and for the same reason: an engine name the runtime cannot realize must fail at load
// rather than leaving a screen that quietly offers nothing.
//
// The roles are *required*, not optional: an engine cannot run against a partial cast, and a
// binding missing one would fail at the first request instead of at load. Every engine here is
// implemented in Go and stays there (002: physical strategies remain runtime-owned) -- what
// metadata decides is which Machines it runs over.
var KnownWorkflowEngines = map[string][]string{
	WorkflowEngineDocumentApproval: {WorkflowRoleDocument, WorkflowRoleStep},
}
