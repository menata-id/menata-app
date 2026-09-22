package web

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/action"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/storage"
)

// signDocument burns every approved step's own signature image onto the Document's PDF, plus the
// growing top-of-page-1 approval-status banner (ROADMAP.md Phase 17; banner owner request,
// 2026-09-19). Called after every individual step approval, not just the one that completes the
// whole Document -- both the signature stamps and the banner are recomposited from scratch each
// time (from the Document's own original fld_file, never from a previous fld_signed_file), so
// calling this more often is safe and simply makes each new approval visible immediately.
//
// Best-effort, the same posture as logActivity: a compositing failure must not undo a decision
// that has already been recorded and saved, only get logged. A step missing a placement or a
// signature on file is silently skipped (action.StampFor), not an error -- this composites
// whatever is ready rather than blocking a real Approve on a placement someone forgot to make.
func signDocument(ctx context.Context, store *data.Store, files *storage.Store, document *data.Record, documentID string) {
	steps, err := store.ListRecordsBy(ctx, action.StepMachineID, action.FieldStepDocument, documentID)
	if err != nil {
		log.Printf("sign document %s: list steps: %v", documentID, err)
		return
	}
	sort.Slice(steps, func(i, j int) bool { return stepSequence(steps[i]) < stepSequence(steps[j]) })

	var stamps []action.Stamp
	var banner []action.ApprovalStatusLine
	for _, step := range steps {
		if toDisplayString(step.Values[action.FieldStepDecision]) != action.DecisionApproved {
			continue
		}
		banner = append(banner, action.ApprovalStatusLine{
			Sequence:     int(stepSequence(step)),
			Total:        len(steps),
			ApproverName: approverName(ctx, store, step),
			ApprovedAt:   step.UpdatedAt,
		})

		image, err := signatureImageForStep(ctx, store, files, step)
		if err != nil {
			log.Printf("sign document %s: signature image for step %s: %v", documentID, step.ID, err)
			continue
		}
		if stamp, ok := action.StampFor(step, image); ok {
			stamps = append(stamps, stamp)
		}
	}
	if len(stamps) == 0 && len(banner) == 0 {
		return
	}

	_, docBytes, err := loadDocumentPDF(ctx, store, files, documentID)
	if err != nil {
		log.Printf("sign document %s: load PDF: %v", documentID, err)
		return
	}

	signed := docBytes
	if len(stamps) > 0 {
		signed, err = action.CompositeSignatures(signed, stamps)
		if err != nil {
			log.Printf("sign document %s: composite signatures: %v", documentID, err)
			return
		}
	}
	signed, err = action.CompositeStatusBanner(signed, action.ApprovalStatusBanner(banner))
	if err != nil {
		log.Printf("sign document %s: composite status banner: %v", documentID, err)
		return
	}

	key, err := files.Save(action.DocumentMachineID, action.FieldDocumentSignedFile, "signed.pdf", bytes.NewReader(signed))
	if err != nil {
		log.Printf("sign document %s: save signed file: %v", documentID, err)
		return
	}
	document.Values[action.FieldDocumentSignedFile] = key
	if _, err := store.UpdateRecord(ctx, action.DocumentMachineID, documentID, document.Values); err != nil {
		log.Printf("sign document %s: save fld_signed_file: %v", documentID, err)
	}
}

// stepSequence reads an Approval Step's own fld_sequence -- internal/action's own sequenceOf is
// unexported (decide.go), so signDocument needs its own copy of this one-line read, same value
// shape (data.ApplyDefaults/data.ValidateRecord already guarantee it's a float64 by the time any
// step reaches here).
func stepSequence(step *data.Record) float64 {
	v, _ := step.Values[action.FieldStepSequence].(float64)
	return v
}

// approverName is the name printed on a step's line of the signed PDF's status banner.
//
// It prefers the snapshot the decision itself recorded (fld_decided_by_name) over any live lookup,
// which is the whole point of that Field existing: a name lives on the identity now, so resolving
// it fresh on every re-composite would rewrite the name on an already-signed document whenever
// that person later changed theirs.
//
// The live path below is the fallback for steps decided before the snapshot existed. It resolves
// through the membership rather than through a Field on the record (there is no longer a name
// Field to read), and ends at the raw id when even that finds nothing -- a banner line carrying an
// id is still evidence the step was approved, where dropping the line would hide it.
func approverName(ctx context.Context, store *data.Store, step *data.Record) string {
	if name := toDisplayString(step.Values[action.FieldStepDecidedByName]); name != "" {
		return name
	}
	assignee := toDisplayString(step.Values[action.FieldStepAssignee])
	workspaceID, _ := data.WorkspaceScope(ctx)
	if names, err := store.MemberNames(ctx, workspaceID); err == nil && names[assignee] != "" {
		return names[assignee]
	}
	return assignee
}

// signatureImageForStep resolves the image signDocument should stamp for one approved step:
// its own one-time fld_signature_image if Approve captured one (owner request, 2026-09-19 --
// see metadata/approval_step.yaml's doc comment), falling back to the assignee's reusable
// mch_signature otherwise. Checked in that order so a captured-but-not-saved signature always
// wins over a stale saved one, though in practice a step only ever carries its own image when
// the assignee had no saved signature at Approve time to begin with.
func signatureImageForStep(ctx context.Context, store *data.Store, files *storage.Store, step *data.Record) ([]byte, error) {
	if key := toDisplayString(step.Values[action.FieldStepSignatureImage]); key != "" {
		return readStoredImage(files, key)
	}
	ownerID := toDisplayString(step.Values[action.FieldStepAssignee])
	return signatureImageFor(ctx, store, files, ownerID)
}

// signatureImageFor fetches ownerID's own reusable signature image (Phase 15 Step 5's
// mch_signature, an ordinary Machine). The first match is used if more than one exists -- no case
// needs choosing between several yet.
func signatureImageFor(ctx context.Context, store *data.Store, files *storage.Store, ownerID string) ([]byte, error) {
	sigs, err := store.ListRecordsBy(ctx, action.SignatureMachineID, action.FieldSignatureOwner, ownerID)
	if err != nil {
		return nil, err
	}
	if len(sigs) == 0 {
		return nil, fmt.Errorf("no signature on file")
	}
	key := toDisplayString(sigs[0].Values[action.FieldSignatureImage])
	return readStoredImage(files, key)
}

// hasSavedSignature reports whether ownerID already has a reusable mch_signature on file --
// signDocument doesn't need the bytes, so this skips the file read signatureImageFor does.
// Threaded into the review screen's decision bar (rendering/reviewdocument.templ) as the
// render-time half of the signature-capture gate; decideStep is the write-time half.
func hasSavedSignature(ctx context.Context, store *data.Store, ownerID string) (bool, error) {
	sigs, err := store.ListRecordsBy(ctx, action.SignatureMachineID, action.FieldSignatureOwner, ownerID)
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
	if machine.ID != action.StepMachineID {
		return true, nil
	}
	return hasSavedSignature(ctx, store, actorID)
}

func readStoredImage(files *storage.Store, key string) ([]byte, error) {
	path, err := files.Path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// placementFields are the only Fields the signature-placement screen writes: where a step's
// signature box sits, and how wide it is.
//
// Naming them here is what makes this route safe in the way the generic update route was not. That
// route rewrites a whole record from whatever the form submits, so the screen had to echo every
// other Field back as a hidden input or lose it -- a list that silently forgot four Fields once and
// erased a one-time signature on the next drag (ROADMAP.md Fase 6c-2/6c-3). A route that writes
// four named Fields and touches nothing else cannot have that bug at all, so the echo is gone
// rather than merely correct.
var placementFields = []string{
	action.FieldStepSignaturePage,
	action.FieldStepSignatureX,
	action.FieldStepSignatureY,
	action.FieldStepSignatureWidth,
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
func updateSignaturePlacement(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.StepMachineID {
			http.Error(w, "this machine has no signature placement", http.StatusNotFound)
			return
		}
		ctx := req.Context()
		step, err := store.GetRecord(ctx, machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		documentID, _ := step.Values[action.FieldStepDocument].(string)
		document, err := store.GetRecord(ctx, action.DocumentMachineID, documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		if !composition.MayPlaceSignature(machine, document, step, currentActor(req, store, cfg)) {
			http.Error(w, "only this document's submitter, or this step's own approver, may place its signature", http.StatusForbidden)
			return
		}

		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		values := data.ValuesFromForm(machine, req.Form)
		for _, id := range placementFields {
			// Only the four, and only when the form actually sent them -- a form that omits one
			// leaves the stored value alone rather than clearing it.
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
