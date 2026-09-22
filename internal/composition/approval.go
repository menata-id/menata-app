package composition

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
)

// Inbox is the Approval Inbox's composed content, before any SLA filter is applied
// (ROADMAP.md Phase 15 Step 1, document-approval.html).
//
// Pending carries every actionable step and Buckets classifies them positionally -- Buckets[i]
// describes Pending[i]. Two parallel slices rather than a field on SummaryCard because the bucket
// is a property of this screen's filter, not of the card: the same card renders identically
// whichever tab is showing, and rendering.SummaryCard is shared with screens that have no SLA
// filter at all.
type Inbox struct {
	// Pending is every actionable Approval Step, rendered as rendering.PendingApprovalCard
	// (document-approval.html's own Pending-my-approval grid) -- richer than rendering.SummaryCard
	// below, which Mine still uses: SLA framing, a submitted-by/date line and a per-step progress
	// bar that only this one worklist needs.
	Pending []rendering.PendingApprovalCard
	Buckets []string

	OverdueCount int
	TodayCount   int

	// Mine is every Document this identity submitted, which is a different question from
	// Pending's "waiting on me" and deliberately unfiltered by SLA.
	//
	// The same rendering.PendingApprovalCard the Pending list uses, by owner instruction
	// (2026-09-21): "tampilan card nya sama, hanya beda bahwa card tersebut dari semua dokumen yang
	// dia submit". It was rendering.SummaryCard -- a plain list row -- which is what the board 11
	// mockup drew, but that board was retired and the owner wants one card face across both views.
	// The card's own "Submitted by" line is what distinguishes the two here: it is left empty,
	// since on this list the submitter is always the viewer, and pendingApprovalCard omits the line
	// entirely rather than rendering "Submitted by you".
	Mine []rendering.PendingApprovalCard

	// NewBreaches names every Document found newly overdue this render, not yet logged
	// (ROADMAP.md Phase 21 round 2, Step G) -- a decision, not a write: buildInbox stays pure and
	// testable without a database; ApprovalInbox (below) is what actually performs the logging,
	// using this list.
	NewBreaches []SLABreach
}

// SLABreach names one Document whose SLA has just been found overdue for the first time.
type SLABreach struct {
	DocumentID string
	Summary    string
}

// slaBreachMarker prefixes every SLA-breach Activity entry's own summary -- both the real,
// human-readable message (Legal Review SLA breached-style copy, document-approval.html) and the
// idempotency check that stops it being logged twice. Checking the activity log itself rather
// than adding a new Document Field sidesteps a real hazard: a boolean Field not present in the
// generic edit form's own HTML would be silently reset to false by ValuesFromForm on the next
// ordinary edit (ValuesFromForm always sets every boolean Field it knows about, present or not),
// re-logging the same breach on every subsequent edit. The activity log is untouched by editing a
// Document, so it is the one place this is genuinely stable.
const slaBreachMarker = "SLA breached: "

// Bucket values for Inbox.Buckets, matching the filter keys the inbox's own tabs submit.
const (
	BucketOverdue  = "overdue"
	BucketToday    = "today"
	BucketUpcoming = "upcoming"
)

// ApprovalInbox composes the Approval Inbox for one identity: every Approval Step assigned to it,
// still pending, and actually actionable right now (action.CanDecide -- a locked sequential step
// does not belong in "pending my approval" even though its own fld_decision is "pending"), plus
// every Document that identity submitted.
//
// "Submitted by" is derived from the existing mch_activity log (Phase 13's own "submitted" event)
// rather than a new Document Field -- Document has no user-editable slot for this, and the data
// already exists.
// stepMachine is mch_approval_step, threaded through so buildInbox can project its own
// card_fields (007 §7.6, the composable-runtime kajian's Fase 1 pilot) onto each pending
// card. relations is only computed when stepMachine actually declares card_fields -- zero added
// cost while no metadata opts in, the same "pay only for what you declare" posture SLAField/
// GroupBy already established.
func ApprovalInbox(ctx context.Context, l *Loader, userID string, now time.Time, stepMachine *domain.Machine) (Inbox, error) {
	steps, err := l.ListRecords(ctx, action.StepMachineID)
	if err != nil {
		return Inbox{}, err
	}
	documents, err := l.ListRecords(ctx, action.DocumentMachineID)
	if err != nil {
		return Inbox{}, err
	}
	activities, err := l.ListRecords(ctx, "mch_activity")
	if err != nil {
		return Inbox{}, err
	}
	names, err := l.PersonNames(ctx)
	if err != nil {
		return Inbox{}, err
	}
	var relations rendering.RelationOptions
	if stepMachine != nil && len(stepMachine.CardFields) > 0 {
		relations, err = l.RelationOptions(ctx, stepMachine)
		if err != nil {
			return Inbox{}, err
		}
	}
	inbox := buildInbox(steps, documents, activities, names, userID, now, stepMachine, relations)
	logSLABreaches(ctx, l.store, inbox.NewBreaches)
	return inbox, nil
}

// logSLABreaches writes one Activity record per newly-detected breach (ROADMAP.md Phase 21 round
// 2, Step G) -- best-effort, the same posture internal/web's own logActivity already takes
// elsewhere: a logging failure must not fail the page render it happened alongside, only get
// logged itself. A deliberate, narrow exception to "reads don't write" (007 §20's own anti-pattern
// is about *security scope* established before retrieval, not about *any* side effect from a GET);
// this app has no scheduler to do it any other way yet (ROADMAP.md tracks the real criteria for
// when one becomes forced).
func logSLABreaches(ctx context.Context, store *data.Store, breaches []SLABreach) {
	for _, b := range breaches {
		values := map[string]any{
			"fld_machine_id": action.DocumentMachineID,
			"fld_record_id":  b.DocumentID,
			"fld_summary":    b.Summary,
		}
		if _, err := store.CreateRecord(ctx, "mch_activity", values); err != nil {
			log.Printf("failed to log SLA breach for document %s: %v", b.DocumentID, err)
		}
	}
}

// buildInbox is the whole of the inbox's derivation, over records someone else already fetched.
// Keeping it free of I/O is what makes the sequencing, bucketing and submitter-resolution rules
// testable at all: they need four related record sets and a fixed clock, not a database.
func buildInbox(steps, documents, activities []*data.Record, names map[string]string, userID string, now time.Time, stepMachine *domain.Machine, relations rendering.RelationOptions) Inbox {
	docByID := make(map[string]*data.Record, len(documents))
	for _, d := range documents {
		docByID[d.ID] = d
	}
	stepsByDoc := make(map[string][]*data.Record, len(documents))
	for _, s := range steps {
		docID := DisplayString(s.Values[action.FieldStepDocument])
		stepsByDoc[docID] = append(stepsByDoc[docID], s)
	}
	submissions := submittersFromActivity(activities)

	// stepMachine is optional for this builder's other callers, so the ordering rule it declares
	// is resolved once here rather than nil-checked at each use.
	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}

	var inbox Inbox
	for _, s := range steps {
		if DisplayString(s.Values[action.FieldStepAssignee]) != userID {
			continue
		}
		if DisplayString(s.Values[action.FieldStepDecision]) != action.DecisionPending {
			continue
		}
		docID := DisplayString(s.Values[action.FieldStepDocument])
		doc := docByID[docID]
		if doc == nil {
			continue
		}
		if !behavior.CanAct(seq, doc, s, stepsByDoc[docID]) {
			continue
		}

		approved := approvedCount(stepsByDoc[docID])
		sub := submissions[docID]
		submitter := names[sub.actor]
		if submitter == "" {
			submitter = "someone"
		}
		submittedAt := ""
		if !sub.at.IsZero() {
			submittedAt = sub.at.Format("2 Jan 2006")
		}

		bucket := BucketUpcoming
		if due, err := time.Parse("2006-01-02", DisplayString(doc.Values["fld_due_date"])); err == nil {
			if status, label := experience.EvaluateSLA(due, now); status == experience.SLAOverdue {
				bucket = BucketOverdue
				inbox.OverdueCount++
			} else if label == "Due today" {
				bucket = BucketToday
				inbox.TodayCount++
			}
		}
		var cardFields []rendering.ProjectedField
		if stepMachine != nil && len(stepMachine.CardFields) > 0 {
			cardFields = ProjectCardFields(stepMachine, s, relations)
		}
		inbox.Pending = append(inbox.Pending, rendering.PendingApprovalCard{
			Reference:    action.DocumentReference(doc.SortOrder),
			Title:        DisplayString(doc.Values["fld_title"]),
			DocumentType: DisplayString(doc.Values["fld_document_type"]),
			Mode:         behavior.SequencingMode(seq, doc),
			Approved:     approved,
			TotalSteps:   len(stepsByDoc[docID]),
			Submitter:    submitter,
			SubmittedAt:  submittedAt,
			SLADue:       doc.Values["fld_due_date"],
			Approvers:    stepStates(seq, doc, stepsByDoc[docID], names, ""),
			Href:         fmt.Sprintf("/machines/%s/records/%s/review", action.StepMachineID, s.ID),
			CardFields:   cardFields,
		})
		inbox.Buckets = append(inbox.Buckets, bucket)
	}

	for _, d := range documents {
		if submissions[d.ID].actor != userID {
			continue
		}
		// Every field the Pending branch resolves, resolved the same way -- Mode through
		// behavior.SequencingMode rather than the raw fld_mode this used to print, so one card face
		// cannot report the mode two different ways depending on which tab drew it. Submitter and
		// SubmittedAt stay zero: see Inbox.Mine. The href goes to the Document, not to a step's
		// review screen, because on this list the viewer is the submitter and has no step to decide.
		inbox.Mine = append(inbox.Mine, rendering.PendingApprovalCard{
			Reference:    action.DocumentReference(d.SortOrder),
			Title:        DisplayString(d.Values["fld_title"]),
			DocumentType: DisplayString(d.Values["fld_document_type"]),
			Mode:         behavior.SequencingMode(seq, d),
			Approved:     approvedCount(stepsByDoc[d.ID]),
			TotalSteps:   len(stepsByDoc[d.ID]),
			SLADue:       d.Values["fld_due_date"],
			Approvers:    stepStates(seq, d, stepsByDoc[d.ID], names, userID),
			Href:         fmt.Sprintf("/machines/%s/records/%s", action.DocumentMachineID, d.ID),
		})
	}

	alreadyLogged := make(map[string]bool, len(activities))
	for _, a := range activities {
		if strings.HasPrefix(DisplayString(a.Values["fld_summary"]), slaBreachMarker) {
			alreadyLogged[DisplayString(a.Values["fld_record_id"])] = true
		}
	}
	for _, d := range documents {
		if alreadyLogged[d.ID] || DisplayString(d.Values[action.FieldDocumentStatus]) != action.DocumentStatusInReview {
			continue
		}
		due, err := time.Parse("2006-01-02", DisplayString(d.Values["fld_due_date"]))
		if err != nil {
			continue
		}
		if status, _ := experience.EvaluateSLA(due, now); status != experience.SLAOverdue {
			continue
		}
		inbox.NewBreaches = append(inbox.NewBreaches, SLABreach{
			DocumentID: d.ID,
			Summary:    fmt.Sprintf("%s%q (due %s)", slaBreachMarker, DisplayString(d.Values["fld_title"]), due.Format("2 Jan 2006")),
		})
	}
	return inbox
}

// approvedCount is how many of a Document's own Approval Steps are already approved -- shared by
// both the "pending my approval" and "my documents" cards, which both need it for the same
// "N/M approved" progress text.
func approvedCount(steps []*data.Record) int {
	approved := 0
	for _, s := range steps {
		if DisplayString(s.Values[action.FieldStepDecision]) == action.DecisionApproved {
			approved++
		}
	}
	return approved
}

// submission is what buildInbox learns about a Document's own submission from the activity log:
// who, and when -- Mine only needs the actor, but PendingApprovalCard's own "Submitted by X · 6
// Sep 2026" line (document-approval.html) needs the date too.
type submission struct {
	actor string
	at    time.Time
}

// submittersFromActivity maps a Document id to its submission -- the actor and time of its
// earliest logged event, which is its submission (Phase 13 logs "submitted" at creation). Events
// are sorted oldest-first and the first actor per Document wins, so a later decision event never
// overwrites the submitter.
func submittersFromActivity(activities []*data.Record) map[string]submission {
	sorted := make([]*data.Record, len(activities))
	copy(sorted, activities)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	submissions := make(map[string]submission, len(sorted))
	for _, a := range sorted {
		docID := DisplayString(a.Values["fld_record_id"])
		if _, ok := submissions[docID]; ok {
			continue
		}
		if actor := DisplayString(a.Values["fld_actor"]); actor != "" {
			submissions[docID] = submission{actor: actor, at: a.CreatedAt}
		}
	}
	return submissions
}

// stepStates derives each of a Document's own Approval Steps as done/current/rejected/waiting,
// ordered by fld_sequence, for PendingApprovalCard's compact progress bar -- the same three live
// states approvalStepRow (internal/rendering/approvalstepper.templ) already renders for the
// Document detail page's vertical stepper, recomputed here rather than shared: that templ's own
// sequence sort is unexported to its package.
//
// Fase 6b gave it a second caller, ReviewDocument (review.go), for board 10's Approval Progress
// list -- which is why the raw reads below stayed here instead of moving into that screen. A new
// .templ may contain no Values[...] at all (internal/conformance's projection ratchet), so the
// review screen renders this slice and reads nothing itself.
//
// viewer is the actor whose own step gets StepApprover.IsYou; pass "" when nobody is viewing in
// particular, as the inbox does -- every card there is already the viewer's own.
func stepStates(seq *domain.Sequencing, parent *data.Record, steps []*data.Record, names map[string]string, viewer string) []rendering.StepApprover {
	ordered := make([]*data.Record, len(steps))
	copy(ordered, steps)
	sort.Slice(ordered, func(i, j int) bool {
		a, _ := strconv.Atoi(DisplayString(ordered[i].Values[action.FieldStepSequence]))
		b, _ := strconv.Atoi(DisplayString(ordered[j].Values[action.FieldStepSequence]))
		return a < b
	})
	approvers := make([]rendering.StepApprover, len(ordered))
	for i, s := range ordered {
		state := "waiting"
		switch DisplayString(s.Values[action.FieldStepDecision]) {
		case action.DecisionApproved:
			state = "done"
		case action.DecisionRejected:
			state = "rejected"
		default:
			if behavior.CanAct(seq, parent, s, ordered) {
				state = "current"
			}
		}
		// The name comes from the names map buildInbox already assembled for My Documents'
		// avatar, so board 07's approver list costs no extra query -- it was in hand and simply
		// not carried onto the card. An assignee with no resolvable record degrades to an empty
		// name, which the card renders as initials-only rather than as a blank row.
		assignee := DisplayString(s.Values[action.FieldStepAssignee])
		name := names[assignee]
		// Only a decided step has a time worth showing; a pending one's UpdatedAt is whenever its
		// signature marker was last dragged, which would read as a decision that never happened.
		decided := ""
		if state == "done" || state == "rejected" {
			decided = s.UpdatedAt.Format("15:04")
		}
		approvers[i] = rendering.StepApprover{
			Name:     name,
			Initials: Initials(name),
			State:    state,
			Label:    stepLabel(s, name),
			Decided:  decided,
			IsYou:    viewer != "" && assignee == viewer,
		}
	}
	return approvers
}

// stepLabel is what a step is for: its declared fld_step_name, or the assignee's own name while
// that Field is empty.
//
// The fallback is not a placeholder, it is the behaviour every screen had before fld_step_name
// existed (approvalStepRow still titles each row with the assignee). Nothing writes the Field until
// board 08's wizard collects it in Fase 6c, so a Document submitted today reads exactly as it did
// yesterday, and one submitted after 6c gains the process-level title board 10 draws.
func stepLabel(s *data.Record, assigneeName string) string {
	if declared := strings.TrimSpace(DisplayString(s.Values[action.FieldStepName])); declared != "" {
		return declared
	}
	return assigneeName
}

// Initials is a person's display initials for a SummaryCard's avatar (Study 38's Avatar cluster)
// -- the first letter of up to the first two words of name.
func Initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	out := strings.ToUpper(fields[0][:1])
	if len(fields) > 1 {
		out += strings.ToUpper(fields[1][:1])
	}
	return out
}
