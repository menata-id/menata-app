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
			Editable: MayPlaceSignature(stepMachine, document, s, viewer),
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

// MayPlaceSignature answers who can move a signature box on this screen, and it is deliberately a
// different question from "who may edit this Approval Step".
//
// Board 09 is `STEP 2 OF 3` of the submit wizard: the person laying the boxes out is the one
// submitting the Document, and internal/web.submitDocumentWizard redirects them straight here. But
// mch_approval_step's edit Permission says only a step's *own approver* may change it -- which is
// right for every other screen and wrong for this one. Until 2026-09-21 the two were the same
// check, so the wizard sent a submitter to a screen on which they could move nothing: every marker
// rendered static, and the PUT behind it would have been refused anyway. It became total when
// `edit` gained its approver-role arm; before that a submitter who happened to also be a step's
// assignee could still drag that one marker, which is why it read as a regression rather than as
// the long-standing mismatch it is.
//
// So: the Document's own submitter, or the step's own approver. The first arm is a cross-record
// rule -- it reads fld_submitted_by on the *parent* -- which no Permission arm in this runtime can
// express (ROADMAP.md's deferral table). It lives here rather than in metadata for that reason,
// and it lives in ONE function so the screen that draws a draggable marker and the route that
// accepts the drag cannot disagree, which is the property authorization.AllowsAction's own doc
// comment describes for buttons and POSTs.
func MayPlaceSignature(stepMachine *domain.Machine, document, step *data.Record, actor domain.Actor) bool {
	if stepMachine == nil || actor.ID == "" {
		return false
	}
	if document != nil {
		if submitter, ok := document.Values[action.FieldDocumentSubmittedBy].(string); ok && submitter != "" && submitter == actor.ID {
			return true
		}
	}
	return authorization.AllowsAction(stepMachine, domain.ActionEdit, step.Values, actor)
}
