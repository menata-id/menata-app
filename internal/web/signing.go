package web

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/storage"
)

// signDocument burns every approved step's own signature image onto the Document's PDF
// (ROADMAP.md Phase 17), once decideStep has driven it to action.DocumentStatusApproved.
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

	var stamps []action.Stamp
	for _, step := range steps {
		if toDisplayString(step.Values[action.FieldStepDecision]) != action.DecisionApproved {
			continue
		}
		ownerID := toDisplayString(step.Values[action.FieldStepAssignee])
		image, err := signatureImageFor(ctx, store, files, ownerID)
		if err != nil {
			log.Printf("sign document %s: signature image for %s: %v", documentID, ownerID, err)
			continue
		}
		if stamp, ok := action.StampFor(step, image); ok {
			stamps = append(stamps, stamp)
		}
	}
	if len(stamps) == 0 {
		return
	}

	_, docBytes, err := loadDocumentPDF(ctx, store, files, documentID)
	if err != nil {
		log.Printf("sign document %s: load PDF: %v", documentID, err)
		return
	}
	signed, err := action.CompositeSignatures(docBytes, stamps)
	if err != nil {
		log.Printf("sign document %s: composite: %v", documentID, err)
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
	path, err := files.Path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
