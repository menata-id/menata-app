package execution

import (
	"bytes"
	"context"
	"log"
	"os"
	"sort"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// compositeSignedDocument is ServiceCompositeSignedDocument: it burns every approved step's signature
// and the growing approval-status banner onto the parent document's PDF, and stores the result
// (ROADMAP.md Stage C, 2026-09-28; the audit's Gap B).
//
// It moved here from internal/web on that date, and the move is the point. What made a signed PDF
// appear was a direct call from flow code -- `signDocument(ctx, store, files, document, documentID)` in
// the decide handler -- named by no metadata anywhere, so the one visible outcome of an approval was
// reachable only from Go (001 #3 inverted). Now an Approval Step declares the Event that causes it, and
// this runs because `then.service` says so.
//
// **What became declared is which records and Fields**, not how. cfg says which relation reaches the
// parent, which Field holds the PDF to composite onto, and which Field receives the result; the binary
// PDF work stays in internal/action (composite.go, banner.go), exactly as 002 intends. The Fields this
// still reads by id -- a step's decision, sequence, signature image and placement -- are the read side
// Stage B deliberately left, and they are what documentApprovalFieldCoupling measures.
//
// Best-effort throughout, like every other Service here: the decision is already recorded and saved by
// the time this runs, so a compositing failure is logged and never allowed to undo it. A step with no
// placement or no signature on file is skipped rather than failing the batch (action.StampFor), which
// is what lets an approval land immediately even if someone forgot to place their box.
func compositeSignedDocument(ctx context.Context, svc Services, machines map[string]*domain.Machine, machine *domain.Machine, record *data.Record, cfg domain.Composite) {
	if svc.Files == nil {
		log.Printf("composite signed document for %s/%s: no file store configured", machine.ID, record.ID)
		return
	}
	parentField, ok := machine.FieldByID(cfg.ParentField)
	if !ok {
		return // refused at load (metadata.validateComposite); nothing sensible to do here
	}
	parentID := displayString(record.Values[cfg.ParentField])
	if parentID == "" {
		return
	}

	steps, err := svc.Store.ListRecordsBy(ctx, machine.ID, cfg.ParentField, parentID)
	if err != nil {
		log.Printf("composite signed document for %s: list steps: %v", parentID, err)
		return
	}
	sort.Slice(steps, func(i, j int) bool { return stepSequence(steps[i]) < stepSequence(steps[j]) })

	var stamps []action.Stamp
	var banner []action.ApprovalStatusLine
	for _, step := range steps {
		if displayString(step.Values[action.FieldStepDecision]) != action.DecisionApproved {
			continue
		}
		banner = append(banner, action.ApprovalStatusLine{
			Sequence:     int(stepSequence(step)),
			Total:        len(steps),
			ApproverName: approverName(ctx, svc.Store, step),
			ApprovedAt:   step.UpdatedAt,
		})
		image, err := signatureImageForStep(ctx, svc, machines, step)
		if err != nil {
			log.Printf("composite signed document for %s: signature image for step %s: %v", parentID, step.ID, err)
			continue
		}
		if stamp, ok := action.StampFor(step, image); ok {
			stamps = append(stamps, stamp)
		}
	}
	if len(stamps) == 0 && len(banner) == 0 {
		return
	}

	parent, err := svc.Store.GetRecord(ctx, parentField.RelatedMachine, parentID)
	if err != nil {
		log.Printf("composite signed document for %s: read parent: %v", parentID, err)
		return
	}
	// Always the declared source, never a previous result: every run recomposites from scratch, which
	// is what makes this safe to call on each approval rather than only on the last (owner request,
	// 2026-09-19 -- every approval should land in the PDF immediately).
	source, err := readStoredFile(svc, displayString(parent.Values[cfg.SourceField]))
	if err != nil {
		log.Printf("composite signed document for %s: read %s: %v", parentID, cfg.SourceField, err)
		return
	}

	signed := source
	if len(stamps) > 0 {
		if signed, err = action.CompositeSignatures(signed, stamps); err != nil {
			log.Printf("composite signed document for %s: composite signatures: %v", parentID, err)
			return
		}
	}
	if signed, err = action.CompositeStatusBanner(signed, action.ApprovalStatusBanner(banner)); err != nil {
		log.Printf("composite signed document for %s: composite status banner: %v", parentID, err)
		return
	}

	key, err := svc.Files.Save(parentField.RelatedMachine, cfg.TargetField, "signed.pdf", bytes.NewReader(signed))
	if err != nil {
		log.Printf("composite signed document for %s: save: %v", parentID, err)
		return
	}
	parent.Values[cfg.TargetField] = key
	if _, err := svc.Store.UpdateRecord(ctx, parentField.RelatedMachine, parentID, parent.Values); err != nil {
		log.Printf("composite signed document for %s: save %s: %v", parentID, cfg.TargetField, err)
	}
}

// stepSequence reads a step's own ordinal. data.ApplyDefaults/ValidateRecord guarantee it is a float64
// by the time any record reaches here.
func stepSequence(step *data.Record) float64 {
	v, _ := step.Values[action.FieldStepSequence].(float64)
	return v
}

// approverName is the name printed on the stamp: the snapshot the decision recorded
// (fld_decided_by_name, written by the declared Action effect -- Stage B), falling back to a live
// lookup for steps decided before that Field existed.
func approverName(ctx context.Context, store *data.Store, step *data.Record) string {
	if name := displayString(step.Values[action.FieldStepDecidedByName]); name != "" {
		return name
	}
	assignee := displayString(step.Values[action.FieldStepAssignee])
	workspaceID, _ := data.WorkspaceScope(ctx)
	if names, err := store.MemberNames(ctx, workspaceID); err == nil && names[assignee] != "" {
		return names[assignee]
	}
	return assignee
}

// signatureImageForStep prefers the one-time image captured when this step was decided, and falls back
// to the approver's own reusable signature.
//
// The reusable store is found by the role its Application casts it in, asked of internal/action rather
// than resolved here: this package must not import internal/rendering (007 §17), so it cannot read the
// Workspace off ctx -- but the machines map it already receives carries the answer, and action owns the
// predicates that read a declared workflow binding. An Application casting no signature store is normal
// (an optional role), and then the step's own image is the whole feature.
func signatureImageForStep(ctx context.Context, svc Services, machines map[string]*domain.Machine, step *data.Record) ([]byte, error) {
	if key := displayString(step.Values[action.FieldStepSignatureImage]); key != "" {
		return readStoredFile(svc, key)
	}
	owner := displayString(step.Values[action.FieldStepAssignee])
	for _, m := range machines {
		if !action.IsSignatureStore(m) {
			continue
		}
		sigs, err := svc.Store.ListRecordsBy(ctx, m.ID, action.FieldSignatureOwner, owner)
		if err != nil {
			return nil, err
		}
		if len(sigs) > 0 {
			return readStoredFile(svc, displayString(sigs[0].Values[action.FieldSignatureImage]))
		}
	}
	return nil, os.ErrNotExist
}

func readStoredFile(svc Services, key string) ([]byte, error) {
	if key == "" {
		return nil, os.ErrNotExist
	}
	path, err := svc.Files.Path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
