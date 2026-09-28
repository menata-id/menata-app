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
// approved is the value that means a step endorsed the document, and it comes from the Event that
// triggered this: `when_equals: approved` on the declaration. That is the same string this package used
// to compare against as action.DecisionApproved, read from the declaration that already states it
// rather than from a constant -- so a Machine whose decision values are `setuju`/`tolak` composites the
// steps it should.
func compositeSignedDocument(ctx context.Context, svc Services, machines map[string]*domain.Machine, machine *domain.Machine, record *data.Record, cfg domain.Composite, approved string) {
	if svc.Files == nil {
		log.Printf("composite signed document for %s/%s: no file store configured", machine.ID, record.ID)
		return
	}
	parentField, ok := machine.FieldByID(cfg.ParentField)
	if !ok {
		return // refused at load (metadata.validateComposite); nothing sensible to do here
	}
	parentMachine := machines[parentField.RelatedMachine]
	// Which Field holds a step's decision, its order and its actor all come from this Machine's own
	// declarations (action.DeclaredFields) rather than from this package naming them -- 001 Principle
	// #8, and what lets the compositing run over a Machine whose Fields are called anything.
	f := action.DeclaredFields(machine, parentMachine)
	parentID := displayString(record.Values[cfg.ParentField])
	if parentID == "" {
		return
	}

	steps, err := svc.Store.ListRecordsBy(ctx, machine.ID, cfg.ParentField, parentID)
	if err != nil {
		log.Printf("composite signed document for %s: list steps: %v", parentID, err)
		return
	}
	// A Machine declaring no ordering has no declared order to sort by: the store's own sort_order
	// (creation order) is then the order, which is what leaving the slice alone gives.
	if f.Order != "" {
		sort.Slice(steps, func(i, j int) bool { return numberValue(steps[i], f.Order) < numberValue(steps[j], f.Order) })
	}

	var stamps []action.Stamp
	var banner []action.ApprovalStatusLine
	for _, step := range steps {
		if displayString(step.Values[f.Decision]) != approved {
			continue
		}
		banner = append(banner, action.ApprovalStatusLine{
			Sequence:     int(numberValue(step, f.Order)),
			Total:        len(steps),
			ApproverName: approverName(ctx, svc.Store, step, machine, f),
			ApprovedAt:   step.UpdatedAt,
		})
		image, err := signatureImageForStep(ctx, svc, machines, machine, step, f)
		if err != nil {
			log.Printf("composite signed document for %s: signature image for step %s: %v", parentID, step.ID, err)
			continue
		}
		if stamp, ok := action.StampFor(machine, step, image); ok {
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

// numberValue reads a declared number Field off a record. data.ApplyDefaults/ValidateRecord guarantee
// it is a float64 by the time any record reaches here; an undeclared Field reads as zero, which is what
// an unordered Machine should contribute to a sort.
func numberValue(record *data.Record, field string) float64 {
	if field == "" {
		return 0
	}
	v, _ := record.Values[field].(float64)
	return v
}

// approverName is the name printed on the stamp: the snapshot the decision recorded
// (fld_decided_by_name, written by the declared Action effect -- Stage B), falling back to a live
// lookup for steps decided before that Field existed.
func approverName(ctx context.Context, store *data.Store, step *data.Record, machine *domain.Machine, f action.EngineFields) string {
	// The Field the decision snapshotted the name into is the one the declared Action effect writes
	// from the acting identity (Stage B) -- so this reads the declaration rather than the id.
	if field := writtenFrom(machine, domain.ActionDecide, domain.WriteFromActorName); field != "" {
		if name := displayString(step.Values[field]); name != "" {
			return name
		}
	}
	assignee := displayString(step.Values[f.Actor])
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
//
// Which Field holds each image comes from the two Machines' own declarations (Stage D,
// signature_placement:/signature_store:). A store declaring neither is skipped rather than queried by a
// guessed name -- an empty Field id would otherwise list every record in it.
func signatureImageForStep(ctx context.Context, svc Services, machines map[string]*domain.Machine, machine *domain.Machine, step *data.Record, f action.EngineFields) ([]byte, error) {
	if image := action.SignatureFields(machine).ImageField; image != "" {
		if key := displayString(step.Values[image]); key != "" {
			return readStoredFile(svc, key)
		}
	}
	owner := displayString(step.Values[f.Actor])
	for _, m := range machines {
		if !action.IsSignatureStore(m) {
			continue
		}
		store := action.StoreFields(m)
		if store.OwnerField == "" || store.ImageField == "" {
			continue
		}
		sigs, err := svc.Store.ListRecordsBy(ctx, m.ID, store.OwnerField, owner)
		if err != nil {
			return nil, err
		}
		if len(sigs) > 0 {
			return readStoredFile(svc, displayString(sigs[0].Values[store.ImageField]))
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

// writtenFrom is the Field a Machine's declared Action effect fills from one source -- the declaration
// this package reads instead of naming fld_decided_by_name. Empty when the Machine declares no such
// write, and then the caller falls back to a live lookup.
func writtenFrom(m *domain.Machine, actionName, source string) string {
	effect, ok := m.EffectFor(actionName)
	if !ok {
		return ""
	}
	for _, wr := range effect.Writes {
		if wr.From == source {
			return wr.Field
		}
	}
	return ""
}
