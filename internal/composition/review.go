package composition

import (
	"context"
	"fmt"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// ReviewDocument composes board 10 (ui-sample/case-03-flow1/10-review-document.html, Fase 6b):
// one Approval Step, seen by the person who has to decide it, with the Document it belongs to.
//
// This function exists because the screen it feeds may contain no field reads of its own. A new
// internal/rendering/*.templ cannot be added to internal/conformance's projection ratchet -- the
// list may only shrink -- so every Values[...] the board needs is resolved here and
// reviewdocument.templ renders a shape that is already decided. That constraint is the reason the
// record-detail page never had a composition entry point and this one does: the detail page
// assembles its Case 3 view model inside the handler and the templates, which is exactly the debt
// the ratchet froze.
//
// It is keyed on the *step*, not the Document. The inbox already links to the step, and "which of
// these approvers is you" then costs nothing -- it is the record we were asked for.
//
// pdfPages comes from the caller rather than being read here: counting a PDF's pages is file I/O
// the transport layer already performs for the signature-placement and pdf-preview routes
// (internal/web's loadSignaturePlacementData), and pushing a storage read down here would give the
// Composition plane a second, parallel way to reach the filesystem. Pass 0 when it is unknown; the
// page renders without the page count rather than failing.
func ReviewDocument(ctx context.Context, l *Loader, stepMachine, docMachine *domain.Machine, step *data.Record, viewer string, pdfPages int, hasSignature bool, now time.Time) (rendering.ReviewView, error) {
	documentID := DisplayString(step.Values[action.FieldStepDocument])
	document, err := l.store.GetRecord(ctx, action.DocumentMachineID, documentID)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	siblings, err := l.ListRecordsBy(ctx, action.StepMachineID, action.FieldStepDocument, documentID)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	activities, err := l.ListRecords(ctx, "mch_activity")
	if err != nil {
		return rendering.ReviewView{}, err
	}
	users, err := l.ListRecords(ctx, "mch_user")
	if err != nil {
		return rendering.ReviewView{}, err
	}
	return buildReview(step, document, siblings, activities, users, stepMachine, docMachine, viewer, pdfPages, hasSignature, now), nil
}

// buildReview is the whole derivation, over records someone else already fetched -- the same
// split buildInbox uses, and for the same reason: the rules worth testing (whose step is
// actionable, which approver is you, whether a signature placement exists) need related record
// sets and a fixed clock, not a database.
func buildReview(step, document *data.Record, siblings, activities, users []*data.Record, stepMachine, docMachine *domain.Machine, viewer string, pdfPages int, hasSignature bool, now time.Time) rendering.ReviewView {
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID] = DisplayString(u.Values["fld_name"])
	}

	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}
	decision := DisplayString(step.Values[action.FieldStepDecision])

	v := rendering.ReviewView{
		StepID:       step.ID,
		DocumentID:   document.ID,
		Reference:    action.DocumentReference(document.SortOrder),
		Title:        DisplayString(document.Values["fld_title"]),
		DocumentType: DisplayString(document.Values["fld_document_type"]),
		Status:       DisplayString(document.Values[action.FieldDocumentStatus]),
		Steps:        stepStates(seq, document, siblings, names, viewer),
		StepLabel:    stepLabel(step, names[DisplayString(step.Values[action.FieldStepAssignee])]),
		Decision:     decision,
		PDFPages:     pdfPages,
		HasSignature: hasSignature,
	}

	// The three questions the Approve/Reject bar asks, kept separate because they fail differently:
	// the viewer may be the wrong person (nothing to offer), the right person on a step still
	// locked behind an earlier one (offer it, but disabled would lie -- the server refuses), or the
	// right person on a step already decided (show what was decided).
	if decision == action.DecisionPending &&
		DisplayString(step.Values[action.FieldStepAssignee]) == viewer &&
		behavior.CanAct(seq, document, step, siblings) {
		v.CanDecide = true
	}
	if stepMachine != nil {
		// The same declared Permission internal/web enforces on POST .../decide, asked here only
		// to decide whether to draw the bar. Presentation, never protection -- the server is what
		// actually refuses, the posture decideButtons already established.
		v.CanDecide = v.CanDecide && authorization.AllowsAction(stepMachine, domain.ActionDecide, step.Values, viewer)
	}

	if key := DisplayString(document.Values[action.FieldDocumentFile]); key != "" {
		v.FileName = storage.DisplayName(key)
		v.FileHref = "/uploads/" + key
	}

	if s, ok := submittersFromActivity(activities)[document.ID]; ok {
		v.SubmittedBy = names[s.actor]
		v.SubmittedAt = s.at.Format("2 Jan 2006")
	}

	// Day-scale wording, deliberately: board 10 writes "Breached · 4 hours ago" but fld_due_date is
	// a type: date with no time component, the same blocker board 07 hit -- ROADMAP.md's deferral
	// table carries it once for both screens rather than once each.
	if docMachine != nil && docMachine.SLAField != "" {
		if due, err := time.Parse("2006-01-02", DisplayString(document.Values[docMachine.SLAField])); err == nil {
			status, label := experience.EvaluateSLA(due, now)
			v.SLALabel = label
			v.SLAOverdue = status == experience.SLAOverdue
		}
	}

	if page, x, y, width, ok := placementOf(step); ok {
		v.Placement = &rendering.ReviewPlacement{
			Page:  page,
			X:     x,
			Y:     y,
			Width: width,
			// The page image the signature-placement screen already serves. Reusing that route
			// rather than adding a per-step one keeps this screen additive: it introduces no
			// rendering capability the app did not already have, only a read-only framing of it.
			PreviewHref: fmt.Sprintf("/machines/%s/records/%s/pdf-preview?page=%d", action.DocumentMachineID, document.ID, page),
		}
	}
	return v
}

// placementOf reads a step's own signature-placement coordinates, reporting ok only when a page
// was actually chosen -- the same "fld_signature_page decides whether a placement exists" rule
// signatureplacement.templ's signaturePlaced has always used, restated here because that helper is
// unexported to the rendering package and this plane is where raw reads now belong.
func placementOf(s *data.Record) (page int, x, y, width float64, ok bool) {
	page = int(numberValue(s, action.FieldStepSignaturePage))
	if page < 1 {
		return 0, 0, 0, 0, false
	}
	width = numberValue(s, action.FieldStepSignatureWidth)
	if width <= 0 {
		// The same default widthControl offers when a step has coordinates but no explicit width.
		width = 20
	}
	return page, numberValue(s, action.FieldStepSignatureX), numberValue(s, action.FieldStepSignatureY), width, true
}

func numberValue(s *data.Record, fieldID string) float64 {
	switch v := s.Values[fieldID].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		var f float64
		if _, err := fmt.Sscanf(DisplayString(s.Values[fieldID]), "%g", &f); err != nil {
			return 0
		}
		return f
	}
}
