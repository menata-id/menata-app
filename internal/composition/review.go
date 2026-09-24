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
func ReviewDocument(ctx context.Context, l *Loader, stepMachine, docMachine *domain.Machine, step *data.Record, viewer domain.Actor, pdfPages int, hasSignature bool, now time.Time) (rendering.ReviewView, error) {
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
	names, err := l.PersonNames(ctx)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	return buildReview(step, document, siblings, activities, names, stepMachine, docMachine, viewer, pdfPages, hasSignature, now), nil
}

// buildReview is the whole derivation, over records someone else already fetched -- the same
// split buildInbox uses, and for the same reason: the rules worth testing (whose step is
// actionable, which approver is you, whether a signature placement exists) need related record
// sets and a fixed clock, not a database.
func buildReview(step, document *data.Record, siblings, activities []*data.Record, names map[string]string, stepMachine, docMachine *domain.Machine, viewer domain.Actor, pdfPages int, hasSignature bool, now time.Time) rendering.ReviewView {

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
		Steps:        stepStates(seq, document, siblings, names, viewer.ID),
		StepLabel:    stepLabel(step, names[DisplayString(step.Values[action.FieldStepAssignee])]),
		Decision:     decision,
		PDFPages:     pdfPages,
		HasSignature: hasSignature,
	}

	// The three questions the Approve/Reject bar asks, kept separate because they fail differently:
	// the viewer may be the wrong person (nothing to offer), the right person on a step still
	// locked behind an earlier one (offer it, but disabled would lie -- the server refuses), or the
	// right person on a step already decided (show what was decided).
	// Who may decide is no longer "is the viewer this step's fld_assignee": since Fase 6c-1 a step
	// may be held by a Group instead, and authorization.AllowsAction is the one function that
	// knows which (CAP-F24). Asking it directly -- rather than pre-filtering on fld_assignee and
	// then asking -- is what keeps a Group-held step decidable here; an assignee check ANDed in
	// front would have silently cancelled the whole capability on this screen.
	// "Is there still a decision to make" is now read off the Machine's own declared state model
	// rather than compared against the literal `pending` (Case 03 Fase 7): a step can be decided
	// exactly when some declared edge leaves its current value through the decide Action. Same
	// answer today -- only `pending` has outgoing edges -- but it is the same declaration the
	// server's own guard enforces (internal/web's declaredDecision), so the bar and the POST
	// cannot drift the way a constant and a rule can.
	if stepMachine != nil && canStillDecide(stepMachine, decision) && behavior.CanAct(seq, document, step, siblings) {
		// The same declared Permission internal/web enforces on POST .../decide, asked here only
		// to decide whether to draw the bar. Presentation, never protection -- the server is what
		// actually refuses, the posture the decide bar already established.
		v.CanDecide = authorization.AllowsAction(stepMachine, domain.ActionDecide, step.Values, viewer)
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
			// Named only when it is somebody else's -- the panel switches to the third person on
			// a non-empty Approver, and the viewer's own signature should never be labelled with
			// their own name back at them.
			Approver: placementApprover(step, names, viewer.ID),
			Page:     page,
			X:        x,
			Y:        y,
			Width:    width,
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

// canStillDecide reports whether any declared Transition leaves decision through the decide
// Action -- "is this step still open", asked of the declaration instead of a constant.
//
// A Machine declaring no transitions at all answers true, which is the same Principle #6 reading
// behavior.CheckTransitions applies: an undeclared state model restricts nothing, so the screen
// falls back to offering the bar and lets the server's own Permission decide, exactly as it did
// before this declaration existed.
func canStillDecide(stepMachine *domain.Machine, decision string) bool {
	if len(stepMachine.Transitions) == 0 {
		return true
	}
	for _, t := range stepMachine.TransitionsFrom(action.FieldStepDecision, decision) {
		if t.Action == domain.ActionDecide {
			return true
		}
	}
	return false
}

// ReviewStepForDocument picks which Approval Step the Review screen should open on when the
// caller has a Document rather than a Step -- which is My Documents' case: it lists Documents, and
// the viewer there is the submitter, who has no step of their own to point at.
//
// It exists because the alternative shapes were both worse. Sending a submitter to the Document's
// *generic* record page is what the app did until 2026-09-24, and that page is the one this repo
// already ruled out for these two Machines: "POC scaffolding no real approver should land on"
// (owner request, 2026-09-19, recorded on rendering.detailBackLink). Making the Review screen's
// whole view model tolerate a nil step would spread "there may be no step" through every field and
// every branch of a screen whose entire subject is a step's own decision.
//
// The order is what a person opening a Document actually wants to see first:
//
//  1. their own step, if they have one still to decide -- an approver who reaches this screen from
//     a Document link gets the same screen the Inbox would have given them;
//  2. the step the Document is waiting on, which is "where this is now";
//  3. the first undecided step, if none is actionable yet (a parallel flow locked behind nothing
//     still has one);
//  4. the last step by sequence, for a Document already finished -- the decision that closed it.
//
// nil, with no error, when the Document has no steps at all. That is not a failure: a Document can
// exist without them (the generic create form makes one), and the caller decides what to do --
// internal/web 404s, and internal/composition's own card falls back to the generic page rather
// than linking a screen with nothing to render.
func ReviewStepForDocument(ctx context.Context, l *Loader, stepMachine *domain.Machine, document *data.Record, viewerID string) (*data.Record, error) {
	steps, err := l.ListRecordsBy(ctx, action.StepMachineID, action.FieldStepDocument, document.ID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, nil
	}
	ordered := orderedBySequence(steps)

	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}

	var firstPending, waitingOn *data.Record
	for _, s := range ordered {
		if !canStillDecide(stepMachine, DisplayString(s.Values[action.FieldStepDecision])) {
			continue
		}
		if firstPending == nil {
			firstPending = s
		}
		if !behavior.CanAct(seq, document, s, ordered) {
			continue
		}
		if waitingOn == nil {
			waitingOn = s
		}
		// The viewer's own actionable step wins outright, wherever it sits in the order.
		if viewerID != "" && DisplayString(s.Values[action.FieldStepAssignee]) == viewerID {
			return s, nil
		}
	}
	switch {
	case waitingOn != nil:
		return waitingOn, nil
	case firstPending != nil:
		return firstPending, nil
	default:
		return ordered[len(ordered)-1], nil
	}
}

// placementApprover names whose signature a placement belongs to, or "" when it is the viewer's
// own. A step held by a Group has no one person's name to give, so it falls back to the step's own
// label -- which is what the approval progress list already calls it.
func placementApprover(step *data.Record, names map[string]string, viewerID string) string {
	assignee := DisplayString(step.Values[action.FieldStepAssignee])
	if assignee != "" && assignee == viewerID {
		return ""
	}
	if name := names[assignee]; name != "" {
		return name
	}
	return stepLabel(step, "")
}
