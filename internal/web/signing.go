package web

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"sort"

	"menata.app/internal/action"
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
			ApproverName: approverName(ctx, store, toDisplayString(step.Values[action.FieldStepAssignee])),
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

// approverName resolves an Approval Step's own fld_assignee (a mch_user id) to that person's
// display name for the status banner. Falls back to the raw id on any read failure -- a banner
// with an id in it is still useful; silently dropping the whole line would hide that this step
// was ever approved.
func approverName(ctx context.Context, store *data.Store, userID string) string {
	user, err := store.GetRecord(ctx, "mch_user", userID)
	if err != nil {
		return userID
	}
	if name := toDisplayString(user.Values["fld_name"]); name != "" {
		return name
	}
	return userID
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
