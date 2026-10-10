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
// signatureMachine is the Machine an Application casts in the optional `signature` role
// (domain.WorkflowRoleSignature) -- a person's own reusable signature, looked up when a "done"
// step captured no one-time image of its own because its approver chose to save a reusable one
// instead (action.SignatureFields' own ImageField doc comment). nil when the Application casts no
// such role, which is normal: the map savedSignatureImages returns is then empty, and every such
// box falls back to a plain checkmark.
func ReviewDocument(ctx context.Context, l *Loader, stepMachine, docMachine, signatureMachine *domain.Machine, step *data.Record, viewer domain.Actor, pdfPages int, hasSignature bool, now time.Time) (rendering.ReviewView, error) {
	f := action.DeclaredFields(stepMachine, docMachine)
	documentID := DisplayString(step.Values[f.Parent])
	document, err := l.Record(ctx, docMachine.ID, documentID)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	siblings, err := l.ListRecordsBy(ctx, stepMachine.ID, f.Parent, documentID)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	savedSignatures, err := savedSignatureImages(ctx, l, signatureMachine)
	if err != nil {
		return rendering.ReviewView{}, err
	}
	return buildReview(step, document, siblings, names, stepMachine, docMachine, viewer, pdfPages, hasSignature, now, savedSignatures), nil
}

// savedSignatureImages maps a person's own id to their reusable signature's stored image key
// (signature_store:'s own OwnerField/ImageField, Stage D) -- the Signature positions panel's
// fallback for a "done" step whose one-time image (signature_placement:'s ImageField) is empty.
// A store declaring either Field empty, or no signature Machine cast at all, answers nil rather
// than guessing a name -- the same "undeclared means skip it" contract every reader of these two
// blocks already takes. Only the first signature found per owner is kept, which is the same
// "most recently saved wins nothing in particular, just pick one" posture hasSavedSignature (its
// own existence check) already takes by not caring how many there are.
func savedSignatureImages(ctx context.Context, l *Loader, signatureMachine *domain.Machine) (map[string]string, error) {
	if signatureMachine == nil {
		return nil, nil
	}
	fields := action.StoreFields(signatureMachine)
	if fields.OwnerField == "" || fields.ImageField == "" {
		return nil, nil
	}
	records, err := l.ListRecords(ctx, signatureMachine.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(records))
	for _, r := range records {
		owner := DisplayString(r.Values[fields.OwnerField])
		if owner == "" {
			continue
		}
		if _, exists := out[owner]; exists {
			continue
		}
		out[owner] = DisplayString(r.Values[fields.ImageField])
	}
	return out, nil
}

// buildReview is the whole derivation, over records someone else already fetched -- the same
// split buildInbox uses, and for the same reason: the rules worth testing (whose step is
// actionable, which approver is you, whether a signature placement exists) need related record
// sets and a fixed clock, not a database.
func buildReview(step, document *data.Record, siblings []*data.Record, names map[string]string, stepMachine, docMachine *domain.Machine, viewer domain.Actor, pdfPages int, hasSignature bool, now time.Time, savedSignatures map[string]string) rendering.ReviewView {
	// One derivation for every Field id this screen reads (action.DeclaredFields) -- see buildInbox.
	f := action.DeclaredFields(stepMachine, docMachine)

	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}
	decision := DisplayString(step.Values[f.Decision])

	v := rendering.ReviewView{
		StepID:       step.ID,
		DocumentID:   document.ID,
		Reference:    action.DocumentReference(document.SortOrder),
		Title:        DisplayString(document.Values["fld_title"]),
		DocumentType: DisplayString(document.Values["fld_document_type"]),
		Status:       DisplayString(document.Values[f.DocumentStatus]),
		Steps:        stepStates(seq, document, siblings, names, viewer.ID, f),
		StepLabel:    names[DisplayString(step.Values[f.Actor])],
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

	// The Field holding the upload comes from the compositing Event's own `source_field` rather than
	// from a constant (action.CompositeFields, 2026-09-29): it is the declaration that already answers
	// "which Field is the document", and reading a constant meant a Workspace naming its upload
	// anything else showed no file at all -- silently, on the screen whose subject is the document.
	if key := DisplayString(document.Values[action.CompositeFields(stepMachine).SourceField]); key != "" {
		v.FileName = storage.DisplayName(key)
		v.FileHref = "/uploads/" + key
	}

	if s, ok := submissionsOf([]*data.Record{document}, f.Submitter)[document.ID]; ok {
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

	// The Signature positions panel (2026-09-29 redesign): every sibling's own marker on whichever
	// page they share, not only the viewer's own. ordered is derived the same way stepStates
	// derived v.Steps above (orderedBySequence over the same siblings), so the two slices line up
	// index for index -- signatureBoxes relies on that rather than re-deriving it a second time.
	ordered := orderedBySequence(siblings, f)
	for i, s := range ordered {
		if p, _, _, _, ok := placementOf(stepMachine, s); ok {
			v.Steps[i].SignaturePage = p
		}
	}
	boxes, page, signed, total := signatureBoxes(ordered, v.Steps, stepMachine, f, savedSignatures)
	v.SignatureBoxes = boxes
	v.SignatureSignedCount = signed
	v.SignatureStepCount = total
	if page > 0 {
		v.SignaturePage = page
		// Both routes the signature-placement screen (board 09) already serves -- reusing them
		// keeps this screen additive: it introduces no rendering capability the app did not
		// already have, only a read-only framing of it, and it means this panel's "See all
		// positions" link can never drift from that screen's own URL shape.
		v.SignaturePreviewHref = PlacementPreviewHref(docMachine.ID, document.ID, page)
		v.AllPositionsHref = PlacementPageHref(docMachine.ID, document.ID, page)
	}
	return v
}

// signatureBoxes derives the Signature positions panel's own markers: every sibling Approval
// Step's own signature-placement box, paired to the same done/current/waiting state
// reviewProgress's own []StepApprover already carries (approvers), so the panel and the progress
// list can never disagree about who has signed. ordered and approvers must be the same length and
// the same order -- buildReview guarantees that by deriving both from orderedBySequence.
//
// The reference page is the viewer's own placement's page when they have placed one, else the
// first sibling's (in sequence order) that has -- so the panel centres on a page somebody actually
// chose rather than defaulting to page 1. A sibling placed on a *different* page is left out of
// this panel rather than force-projected onto a page it is not on; "See all positions" is where it
// is.
//
// A "signed" box's own ImageHref prefers the step's one-time captured image
// (action.SignatureFields' ImageField) and falls back to savedSignatures[assignee] -- the same
// precedence internal/execution's signatureImageForStep already composites onto the PDF with,
// restated here so the panel shows the same ink it burned onto the document rather than a second,
// possibly-disagreeing guess.
func signatureBoxes(ordered []*data.Record, approvers []rendering.StepApprover, stepMachine *domain.Machine, f action.EngineFields, savedSignatures map[string]string) (boxes []rendering.ReviewSignatureBox, page, signed, total int) {
	total = len(ordered)
	for _, a := range approvers {
		if a.State == "done" {
			signed++
		}
	}
	for i, s := range ordered {
		if p, _, _, _, ok := placementOf(stepMachine, s); ok && approvers[i].IsYou {
			page = p
			break
		}
	}
	if page == 0 {
		for _, s := range ordered {
			if p, _, _, _, ok := placementOf(stepMachine, s); ok {
				page = p
				break
			}
		}
	}
	if page == 0 {
		return nil, 0, signed, total
	}
	for i, s := range ordered {
		p, x, y, width, ok := placementOf(stepMachine, s)
		if !ok || p != page {
			continue
		}
		a := approvers[i]
		kind := "waiting"
		switch {
		case a.State == "done":
			kind = "signed"
		case a.State == "current" && a.IsYou:
			kind = "yours"
		}
		label := a.Name
		if label == "" {
			label = "Unassigned"
		}
		if a.IsYou {
			label += " (You)"
		}
		imageHref := ""
		if kind == "signed" {
			imageKey := DisplayString(s.Values[action.SignatureFields(stepMachine).ImageField])
			if imageKey == "" {
				imageKey = savedSignatures[DisplayString(s.Values[f.Actor])]
			}
			if imageKey != "" {
				imageHref = "/uploads/" + imageKey
			}
		}
		boxes = append(boxes, rendering.ReviewSignatureBox{
			Index:     i + 1,
			Label:     label,
			Kind:      kind,
			ImageHref: imageHref,
			X:         x,
			Y:         y,
			Width:     width,
		})
	}
	return boxes, page, signed, total
}

// placementOf reads a step's own signature-placement coordinates, reporting ok only when a page was
// actually chosen -- the same "the page Field decides whether a placement exists" rule
// signatureplacement.templ's signaturePlaced has always used, restated here because that helper is
// unexported to the rendering package and this plane is where raw reads now belong.
//
// Which four Fields those are comes from the step Machine's own signature_placement: declaration
// (Stage D). A Machine declaring none reads every coordinate as zero and therefore has no placement,
// which is the right answer rather than a special case.
func placementOf(stepMachine *domain.Machine, s *data.Record) (page int, x, y, width float64, ok bool) {
	p := action.SignatureFields(stepMachine)
	page = int(numberValue(s, p.PageField))
	if page < 1 {
		return 0, 0, 0, 0, false
	}
	width = numberValue(s, p.WidthField)
	if width <= 0 {
		// The same default widthControl offers when a step has coordinates but no explicit width.
		width = 20
	}
	return page, numberValue(s, p.XField), numberValue(s, p.YField), width, true
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
	for _, t := range stepMachine.TransitionsFrom(stepMachine.ActionField(domain.ActionDecide), decision) {
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
//  0. their own step, if they have already decided it -- returned outright, before anything else
//     is even considered. Without this, an approver who already approved (or rejected) and then
//     reopens the Document from a link -- My Documents, an activity entry, a bookmark -- got
//     silently handed *whichever other sibling the Document is waiting on instead*, and the
//     screen's footer then read as if nobody's decision had landed ("Awaiting its own assignee's
//     decision") with no mention that the viewer's own already had. Found on a real parallel,
//     two-approver Document (2026-09-29): the first approver, revisiting after approving, saw the
//     second approver's still-pending step and read the page as "you haven't signed" even though
//     the Approval Progress list beside it already said otherwise -- two panels answering the same
//     question two different ways because only one of them was asked about the viewer's own step.
//  1. their own step, if they have one still to decide -- an approver who reaches this screen from
//     a Document link gets the same screen the Inbox would have given them;
//  2. the step the Document is waiting on, which is "where this is now";
//  3. the first undecided step, if none is actionable yet (a parallel flow locked behind nothing
//     still has one);
//  4. the last step by sequence, for a Document already finished -- the decision that closed it.
//
// Step 0 is deliberately not folded into step 1's own loop: step 1 only matches a step that is
// both undecided *and* currently actionable (CanAct), which is right for it -- a sequential
// approver whose own step is still locked behind an earlier one should see what it is waiting on,
// not their own unreachable step. A *decided* step carries no such nuance; there is nothing left to
// wait on, so it always wins.
//
// nil, with no error, when the Document has no steps at all. That is not a failure: a Document can
// exist without them (the generic create form makes one), and the caller decides what to do --
// internal/web 404s, and internal/composition's own card falls back to the generic page rather
// than linking a screen with nothing to render.
//
// **docMachine is required, and passing nil here was a live 404 for a day** (2026-09-28). The Field
// reaching a step's parent is derived by *comparing* the step Machine's relations against the document
// Machine's id (action.DeclaredFields), so with nil there is no id to match and f.Parent comes back
// empty -- ListRecordsBy then filters on "" , matches no rows, and every Document looked like a
// Document with no steps. The 404 that produced is the one branch below that is a legitimate answer,
// which is exactly why nothing looked wrong: the screen failed in a shape it is supposed to have.
func ReviewStepForDocument(ctx context.Context, l *Loader, stepMachine, docMachine *domain.Machine, document *data.Record, viewerID string) (*data.Record, error) {
	f := action.DeclaredFields(stepMachine, docMachine)
	steps, err := l.ListRecordsBy(ctx, stepMachine.ID, f.Parent, document.ID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, nil
	}
	ordered := orderedBySequence(steps, f)

	// Step 0 (see doc comment above): a step the viewer already decided always wins, before the
	// "what is this Document waiting on" question below is even asked.
	if viewerID != "" {
		for _, s := range ordered {
			if DisplayString(s.Values[f.Actor]) == viewerID && !canStillDecide(stepMachine, DisplayString(s.Values[f.Decision])) {
				return s, nil
			}
		}
	}

	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}

	var firstPending, waitingOn *data.Record
	for _, s := range ordered {
		if !canStillDecide(stepMachine, DisplayString(s.Values[f.Decision])) {
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
		if viewerID != "" && DisplayString(s.Values[f.Actor]) == viewerID {
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
