package action

// CanReviseDocument and CanContinueDraft gate the two Draft-related moves Tahap 4 added
// (2026-09-25) the same way CanDeleteDocument gates delete: a plain business-state check in Go,
// not a declared Transition. internal/conformance.TestDocumentStatusIsDerivedNotSettable holds a
// deliberate invariant that predates Draft -- no mch_document transition on fld_status may declare
// an Action, because a Document's status is derived from its Approval Steps
// (evt_step_decision_rollup), and giving a person-performed edge an Action there is exactly the
// hole that let the generic update route set fld_status to "approved" directly before Fase 7.
// Neither new move is that hole (each is reachable only through its own dedicated,
// Permission-gated route -- internal/web's reviseDocument/continueDocumentWizard -- never the
// generic one), but the invariant is still real and still worth keeping literally true: no
// fld_status edge anywhere carries an Action. So these checks live here instead, the same posture
// CanDeleteDocument's own business-state gate already takes for delete.

// CanReviseDocument reports whether a Document may move back to Draft through Revise -- only from
// Rejected, the one status My Documents' own "Revise" button appears on.
func CanReviseDocument(status string) (ok bool, reason string) {
	if status != DocumentStatusRejected {
		return false, "only a rejected document can be revised"
	}
	return true, ""
}

// CanContinueDraft reports whether a Document may be finalized into review through the submit
// wizard's "Continue" -- only from Draft.
func CanContinueDraft(status string) (ok bool, reason string) {
	if status != DocumentStatusDraft {
		return false, "only a draft document can be continued"
	}
	return true, ""
}
