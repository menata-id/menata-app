package composition

import (
	"context"
	"fmt"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// SignaturePlacement composes board 09 (ui-sample/case-03-flow1/09-signature-positions.html,
// Fase 6c-3): one Document's page, and every Approval Step's signature marker on it.
//
// Same split as ReviewDocument (review.go): this does the reads, buildPlacement does the
// derivation, and the .templ renders a shape that is already decided. totalPages arrives as a
// parameter for the same reason pdfPages does there -- counting a PDF's pages is a storage read
// the transport layer already performs, and giving this plane a second route to the filesystem is
// what that boundary exists to prevent.
func SignaturePlacement(ctx context.Context, l *Loader, stepMachine *domain.Machine, document *data.Record, steps []*data.Record, relations rendering.RelationOptions, page, totalPages int, viewer domain.Actor) (rendering.PlacementView, error) {
	groups, err := l.GroupOptions(ctx, stepMachine)
	if err != nil {
		return rendering.PlacementView{}, err
	}
	return buildPlacement(stepMachine, document, steps, relations, groups, page, totalPages, viewer), nil
}

// formOwnedFields are the Fields each placement form supplies itself, and therefore the only ones
// carryForward must leave out.
//
// Everything else a step holds has to ride along as a hidden input, because these forms PUT to the
// *generic* update route, which rewrites a record from what it is given while data.ValuesFromForm
// drops whatever is absent. That is the trap Fase 6c-2 found and fixed by hand -- and the hand fix
// was already incomplete: it added three names and missed fld_signature_image, so a one-time
// signature captured at Approve time was still erased by the next drag.
//
// Deriving the list from m.Fields instead is what makes the trap structurally impossible rather
// than fixed once. The next Field added to mch_approval_step is carried without anyone
// remembering to, which is exactly the remembering that failed twice.
var formOwnedFields = map[string]bool{
	action.FieldStepSignaturePage:  true,
	action.FieldStepSignatureX:     true,
	action.FieldStepSignatureY:     true,
	action.FieldStepSignatureWidth: true,
}

// buildPlacement is the whole derivation, over records someone else already fetched.
func buildPlacement(stepMachine *domain.Machine, document *data.Record, steps []*data.Record, relations rendering.RelationOptions, groups rendering.GroupOptions, page, totalPages int, viewer domain.Actor) rendering.PlacementView {
	v := rendering.PlacementView{
		DocumentID:    document.ID,
		DocumentTitle: DisplayString(document.Values["fld_title"]),
		Page:          page,
		TotalPages:    totalPages,
	}
	for i, s := range steps {
		step := rendering.PlacementStep{
			StepID:   s.ID,
			Index:    i + 1,
			StepName: DisplayString(s.Values[action.FieldStepName]),
			// The same declared Permission the generic PUT route enforces, asked here only to
			// decide whether to render a draggable marker or a static one. Presentation, never
			// protection -- and since Fase 6c-3 it resolves a Group-held step through its Group,
			// which is the whole reason such a step's marker is draggable at all.
			Editable:    stepMachine != nil && authorization.AllowsAction(stepMachine, domain.ActionEdit, s.Values, viewer),
			CarryFields: carryForward(stepMachine, s),
		}
		step.ApproverKind, step.Approver = approverOf(s, relations, groups)
		if p, x, y, width, ok := placementOf(s); ok {
			step.Placed, step.Page, step.X, step.Y, step.Width = true, p, x, y, width
		} else {
			// The defaults the place/width controls offer, resolved here so the .templ renders a
			// number rather than deciding one.
			step.X, step.Y, step.Width = 50, 50, 20
		}
		v.Steps = append(v.Steps, step)
	}
	return v
}

// approverOf resolves who holds a step -- a person or a whole Group -- into one label plus the
// kind that produced it.
//
// The kind is returned rather than inferred from which label is non-empty, because board 09 draws
// them differently (a User badge and a Group badge) and an empty person name must not silently
// read as a Group. An unset fld_approver_type means the person arm, which is every step written
// before CAP-F24 and every User row the wizard writes (it deliberately stores no type there, so
// authorization's fallback stays the live path).
func approverOf(s *data.Record, relations rendering.RelationOptions, groups rendering.GroupOptions) (kind, label string) {
	if DisplayString(s.Values[action.FieldStepApproverType]) == domain.ActorKindGroup {
		return domain.ActorKindGroup, rendering.GroupLabel(groups, DisplayString(s.Values[action.FieldStepApproverGroup]))
	}
	return domain.ActorKindUser, rendering.RelationLabel(relations, domain.UserMachineID, DisplayString(s.Values[action.FieldStepAssignee]))
}

// carryForward is every Field this Machine declares that the placement forms do not themselves
// submit, as name/value pairs ready to render as hidden inputs. See formOwnedFields for why this
// is derived rather than listed.
//
// A Field with no value on this record is still emitted, as an empty input. That is deliberate:
// data.ValuesFromForm treats empty and absent identically (it skips both), so an empty input
// changes nothing -- but rendering one keeps the form's shape stable across records, which is what
// makes "did this Field get carried?" answerable by looking at one rendered page.
func carryForward(m *domain.Machine, s *data.Record) []rendering.CarryField {
	if m == nil {
		return nil
	}
	fields := make([]rendering.CarryField, 0, len(m.Fields))
	for _, f := range m.Fields {
		if formOwnedFields[f.ID] {
			continue
		}
		fields = append(fields, rendering.CarryField{Name: f.ID, Value: DisplayString(s.Values[f.ID])})
	}
	return fields
}

// PlacementPageHref and PlacementPreviewHref are the two routes board 09 links: another page of
// the same screen, and the rendered image of one page. Both live here rather than in the .templ so
// the page renders strings it was handed -- the same posture ReviewPlacement's own PreviewHref
// already takes.
func PlacementPageHref(documentID string, page int) string {
	return fmt.Sprintf("/machines/%s/records/%s/signature-placement?page=%d", action.DocumentMachineID, documentID, page)
}

func PlacementPreviewHref(documentID string, page int) string {
	return fmt.Sprintf("/machines/%s/records/%s/pdf-preview?page=%d", action.DocumentMachineID, documentID, page)
}

// PlacementFieldsForTest exposes carryForward to internal/web's own round-trip test, which must
// submit exactly what the rendered form submits. A test that hand-listed those names instead would
// drift from the page the same way the hidden-input list drifted from the Machine -- which is the
// drift the whole carry-forward change exists to end.
func PlacementFieldsForTest(m *domain.Machine, s *data.Record) []rendering.CarryField {
	return carryForward(m, s)
}
