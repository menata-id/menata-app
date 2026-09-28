package web

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/action"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// What internal/web still owns here is the *render-time* half of the signature gate and the placement
// route. The compositing itself moved to internal/execution on 2026-09-28 (Stage C): it is a declared
// Service now (composite_signed_document, triggered by mch_approval_step's own
// evt_step_signed_document), so what causes a signed PDF is metadata rather than a call from this
// package. signDocument went with stepSequence, approverName and signatureImageForStep -- nothing here
// used any of them but that operation.

// hasSavedSignature reports whether ownerID already has a reusable mch_signature on file --
// signDocument doesn't need the bytes, so this skips the file read signatureImageFor does.
// Threaded into the review screen's decision bar (rendering/reviewdocument.templ) as the
// render-time half of the signature-capture gate; decideStep is the write-time half.
func hasSavedSignature(ctx context.Context, store *data.Store, ownerID string) (bool, error) {
	signatures := approvalMachine(ctx, domain.WorkflowRoleSignature)
	if signatures == nil {
		return false, nil // no signature store cast here -- see signatureImageFor
	}
	// Which Field says whose signature this is comes from that Machine's own signature_store:
	// declaration (Stage D). Undeclared means the question cannot be asked of it at all -- listing by
	// an empty Field id would return every signature in the store and answer "yes" for everyone.
	owner := action.StoreFields(signatures).OwnerField
	if owner == "" {
		return false, nil
	}
	sigs, err := store.ListRecordsBy(ctx, signatures.ID, owner, ownerID)
	if err != nil {
		return false, err
	}
	return len(sigs) > 0, nil
}

// hasSignatureForGate is the review screen's own render-time gate input
// (rendering/reviewdocument.templ): whether actorID may Approve without the canvas modal appearing
// first.
//
// It keeps its "true for every Machine other than mch_approval_step" arm although Fase 6b left it
// exactly one caller, which already checks that itself. The arm is what makes the function safe to
// call without knowing the Machine, and removing it would make a future third caller's omission
// silent -- it would read as "this person has a signature" rather than "this question does not
// apply here". Until Fase 6b it was called on *every* Machine's detail page, mch_task included;
// that is the call the review screen took away.
func hasSignatureForGate(ctx context.Context, store *data.Store, machine *domain.Machine, actorID string) (bool, error) {
	if !action.IsStep(machine) {
		return true, nil
	}
	return hasSavedSignature(ctx, store, actorID)
}

// updateSignaturePlacement moves or resizes one step's signature box (board 09).
//
// It exists because the question this screen asks is not the one mch_approval_step's edit
// Permission answers. Board 09 is `STEP 2 OF 3` of the submit wizard -- the person laying the
// boxes out is the one *submitting* the Document -- while that Permission says only a step's own
// approver may change it. Both are right; they are simply about different people, and
// composition.MayPlaceSignature is the one function that resolves which applies, called here and
// by the screen that decides whether to draw a draggable marker.
//
// Until this route existed those forms PUT to the generic record route, so the wizard redirected a
// submitter to a screen where every marker was static and the write would have been refused
// anyway. It is the bug the owner hit the first time they submitted a document after the role
// rules landed.
func updateSignaturePlacement(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsStep(machine) {
			http.Error(w, "this machine has no signature placement", http.StatusNotFound)
			return
		}
		ctx := req.Context()
		step, err := store.GetRecord(ctx, machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		documentMachine := approvalMachine(ctx, domain.WorkflowRoleDocument)
		// The relation reaching the parent is derived, not named -- the same DeclaredFields every
		// other reader in this engine already uses (Stage B).
		documentID, _ := step.Values[action.DeclaredFields(machine, documentMachine).Parent].(string)
		document, err := store.GetRecord(ctx, approvalMachineID(ctx, domain.WorkflowRoleDocument), documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		if !composition.MayPlaceSignature(machine, documentMachine, document, step, currentActor(req, store, cfg)) {
			http.Error(w, "only this document's submitter, or this step's own approver, may place its signature", http.StatusForbidden)
			return
		}

		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		// The only Fields this screen writes: where a step's signature box sits and how wide it is,
		// read from this Machine's own signature_placement: declaration (Stage D) -- deliberately
		// PlacementFields rather than Fields, so moving a box can never touch the image a decision
		// already captured.
		//
		// Writing a named set is what makes this route safe in the way the generic update route was
		// not. That route rewrites a whole record from whatever the form submits, so the screen had to
		// echo every other Field back as a hidden input or lose it -- a list that silently forgot four
		// Fields once and erased a one-time signature on the next drag (ROADMAP.md Fase 6c-2/6c-3). A
		// route that writes only these and touches nothing else cannot have that bug at all.
		values := data.ValuesFromForm(machine, req.Form)
		for _, id := range machine.SignaturePlacement.PlacementFields() {
			// Only when the form actually sent one -- a form that omits a Field leaves the stored
			// value alone rather than clearing it.
			if _, sent := req.Form[id]; sent {
				step.Values[id] = values[id]
			}
		}
		if _, err := store.UpdateRecord(ctx, machine.ID, step.ID, step.Values); err != nil {
			recordError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
