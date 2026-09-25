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

// pendingStepsFor selects the Approval Steps this viewer can act on right now: assigned to them,
// still undecided, whose Document exists, and which the Machine's own sequencing rule says are
// reachable (behavior.CanAct -- the arm that reads sibling steps, so a parallel Document's
// second step is actionable and a sequential one's is not).
//
// It is a function rather than four conditions inline because two callers need the identical
// answer and must never disagree: buildInbox, which turns each into a card, and
// PendingApprovalCount, which only counts them for the nav badge. A badge reporting a different
// number from the list it links to is the kind of defect nobody reports and everybody distrusts,
// and the way to make it impossible is one predicate, not two that currently match.
func pendingStepsFor(steps []*data.Record, docByID map[string]*data.Record, stepsByDoc map[string][]*data.Record, userID string, seq *domain.Sequencing) []*data.Record {
	var selected []*data.Record
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
		selected = append(selected, s)
	}
	return selected
}

// PendingApprovalCount is the number the nav badge shows: how many Approval Steps this viewer can
// decide right now.
//
// It exists because the badge used to obtain that integer by composing the entire Approval Inbox
// (ApprovalInbox, via showPendingCount) -- which reads the activity log and every member's name to
// build cards nobody renders, and, through logSLABreaches, *writes*. The badge fires on every page
// carrying it: 463 times in the six hours of log reviewed on 2026-09-22, roughly a third of all
// requests. Two reads and no write is what the number actually needs.
//
// It shares pendingStepsFor with the inbox itself, so the badge and the list it links to cannot
// drift apart. It deliberately does NOT log SLA breaches: breach detection is the *inbox's*
// read-triggered side effect (logSLABreaches' own doc comment explains why this app has nowhere
// else to put it yet), and duplicating it onto a badge would mean a count endpoint racing the
// page it decorates to write the same activity rows.
func PendingApprovalCount(ctx context.Context, l *Loader, userID string, stepMachine *domain.Machine) (int, error) {
	steps, err := l.ListRecords(ctx, action.StepMachineID)
	if err != nil {
		return 0, err
	}
	documents, err := l.ListRecords(ctx, action.DocumentMachineID)
	if err != nil {
		return 0, err
	}
	docByID := make(map[string]*data.Record, len(documents))
	for _, d := range documents {
		docByID[d.ID] = d
	}
	stepsByDoc := make(map[string][]*data.Record, len(documents))
	for _, s := range steps {
		stepsByDoc[DisplayString(s.Values[action.FieldStepDocument])] = append(
			stepsByDoc[DisplayString(s.Values[action.FieldStepDocument])], s)
	}
	var seq *domain.Sequencing
	if stepMachine != nil {
		seq = stepMachine.Sequencing
	}
	return len(pendingStepsFor(steps, docByID, stepsByDoc, userID, seq)), nil
}

// MineFilters is My Documents' own status chip row (Flow 2 mockup, My Documents' "All / Draft /
// In review / Approved / Rejected"), counts built from mine -- the *unfiltered* list -- so a chip's
// own count never changes depending on which chip is already active, the same rule
// pendingTabContent's Overdue/Due-today chips and assignedTabContent's four decision chips both
// already follow.
//
// The Draft chip landed 2026-09-25 (Flow 2 gap study Tahap 4) alongside the save-as-draft flow
// that can finally produce that status -- this row used to explain why there was no such chip
// ("fld_status declares exactly [in_review, approved, rejected]"); that condition is gone.
func MineFilters(mine []rendering.PendingApprovalCard, statusKey string) []rendering.FilterChip {
	counts := make(map[string]int, 4)
	for _, c := range mine {
		counts[c.Status]++
	}
	return []rendering.FilterChip{
		{Key: "all", Label: "All", Count: len(mine), Active: statusKey == "" || statusKey == "all"},
		{Key: action.DocumentStatusDraft, Label: "Draft", Count: counts[action.DocumentStatusDraft], Active: statusKey == action.DocumentStatusDraft},
		{Key: action.DocumentStatusInReview, Label: "In review", Count: counts[action.DocumentStatusInReview], Active: statusKey == action.DocumentStatusInReview},
		{Key: action.DocumentStatusApproved, Label: "Approved", Count: counts[action.DocumentStatusApproved], Active: statusKey == action.DocumentStatusApproved},
		{Key: action.DocumentStatusRejected, Label: "Rejected", Count: counts[action.DocumentStatusRejected], Active: statusKey == action.DocumentStatusRejected},
	}
}

// SplitDrafts partitions My Documents' own cards into Drafts and Submitted (Flow 2 mockup's own
// two sections, MyDocuments.dc.html) -- a Draft card shows "Continue", nothing else does. Order
// within each half is preserved from mine, and either half may come back empty (a status chip
// narrows mine before this runs, so e.g. the Draft chip active leaves submitted empty).
func SplitDrafts(mine []rendering.PendingApprovalCard) (drafts, submitted []rendering.PendingApprovalCard) {
	for _, c := range mine {
		if c.Status == action.DocumentStatusDraft {
			drafts = append(drafts, c)
		} else {
			submitted = append(submitted, c)
		}
	}
	return drafts, submitted
}

// FilterCardsByStatus narrows cards to statusKey's own Status; "" or "all" (MineFilters' own "no
// chip picked" values) returns cards unchanged.
func FilterCardsByStatus(cards []rendering.PendingApprovalCard, statusKey string) []rendering.PendingApprovalCard {
	if statusKey == "" || statusKey == "all" {
		return cards
	}
	out := make([]rendering.PendingApprovalCard, 0, len(cards))
	for _, c := range cards {
		if c.Status == statusKey {
			out = append(out, c)
		}
	}
	return out
}

// SearchCards narrows cards to those whose Title or Reference contains q, case-insensitively -- My
// Documents' own search box (Flow 2 mockup: "Search title or DOC number…"). An empty q returns
// cards unchanged, the same convention every filter on this app already uses.
func SearchCards(cards []rendering.PendingApprovalCard, q string) []rendering.PendingApprovalCard {
	if q == "" {
		return cards
	}
	needle := strings.ToLower(q)
	out := make([]rendering.PendingApprovalCard, 0, len(cards))
	for _, c := range cards {
		if strings.Contains(strings.ToLower(c.Title), needle) || strings.Contains(strings.ToLower(c.Reference), needle) {
			out = append(out, c)
		}
	}
	return out
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
	for _, s := range pendingStepsFor(steps, docByID, stepsByDoc, userID, seq) {
		docID := DisplayString(s.Values[action.FieldStepDocument])
		doc := docByID[docID]

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
			ID:           docID,
			Reference:    action.DocumentReference(doc.SortOrder),
			Title:        DisplayString(doc.Values["fld_title"]),
			DocumentType: DisplayString(doc.Values["fld_document_type"]),
			Mode:         behavior.SequencingMode(seq, doc),
			Approved:     approved,
			TotalSteps:   len(stepsByDoc[docID]),
			Submitter:    submitter,
			SubmittedAt:  submittedAt,
			Status:       DisplayString(doc.Values[action.FieldDocumentStatus]),
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
		// cannot report the mode two different ways depending on which tab drew it. Submitter stays
		// empty: see Inbox.Mine (every card here was submitted by the viewer, so it would read
		// "Submitted by you" on every one). SubmittedAt is filled in now (2026-09-25) -- it always
		// was resolved (`submissions[d.ID]`, the same map the Pending branch reads), just never
		// rendered because pendingApprovalCard's own "Submitted by" line, the only place it used to
		// appear, is gated on Submitter being non-empty. A Draft card's own "Not submitted -- Last
		// edited {SubmittedAt}" line (rendering) is this field's first real reader.
		//
		// **The href was wrong until 2026-09-24**, and the comment here explained why at the time:
		// "the href goes to the Document, not to a step's review screen, because on this list the
		// viewer is the submitter and has no step to decide." The premise was right and the
		// conclusion did not follow. Having no step to decide is a reason not to show a decision
		// bar -- which the Review screen already handles, gating it on authorization.AllowsAction --
		// not a reason to send someone to the Document's *generic record page*, which is the one
		// destination this repo had already ruled out for these two Machines ("POC scaffolding no
		// real approver should land on", owner request 2026-09-19). A submitter asking "where has
		// my document got to" got a field-by-field CRUD form with an Edit and a Delete button.
		//
		// It links the Review screen by Document id now (internal/web.reviewStep resolves which
		// step that opens on). The fallback below is the one case that screen cannot render: a
		// Document with no steps at all, which the generic create form can make and the wizard
		// never does.
		status := DisplayString(d.Values[action.FieldDocumentStatus])
		mineSubmittedAt := ""
		if at := submissions[d.ID].at; !at.IsZero() {
			mineSubmittedAt = at.Format("2 Jan 2006")
		}
		inbox.Mine = append(inbox.Mine, rendering.PendingApprovalCard{
			ID:           d.ID,
			Reference:    action.DocumentReference(d.SortOrder),
			Title:        DisplayString(d.Values["fld_title"]),
			DocumentType: DisplayString(d.Values["fld_document_type"]),
			Mode:         behavior.SequencingMode(seq, d),
			Approved:     approvedCount(stepsByDoc[d.ID]),
			TotalSteps:   len(stepsByDoc[d.ID]),
			SubmittedAt:  mineSubmittedAt,
			Status:       status,
			SLADue:       d.Values["fld_due_date"],
			Approvers:    stepStates(seq, d, stepsByDoc[d.ID], names, userID),
			Href:         reviewHref(d.ID, len(stepsByDoc[d.ID]), status),
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
	ordered := orderedBySequence(steps)
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

// orderedBySequence sorts Approval Steps by their declared fld_sequence, ascending -- the semantic
// order of an approval chain, which is not the storage sort_order (creation order) and not the
// order the database happened to return them in.
//
// Extracted from stepStates on 2026-09-24 when ReviewStepForDocument needed the same ordering to
// answer "which step is this Document waiting on". Two copies of a sort that decides which
// approver a screen names first is the kind of duplication that reads identical until one of them
// gains a tiebreak.
func orderedBySequence(steps []*data.Record) []*data.Record {
	ordered := make([]*data.Record, len(steps))
	copy(ordered, steps)
	sort.Slice(ordered, func(i, j int) bool {
		a, _ := strconv.Atoi(DisplayString(ordered[i].Values[action.FieldStepSequence]))
		b, _ := strconv.Atoi(DisplayString(ordered[j].Values[action.FieldStepSequence]))
		return a < b
	})
	return ordered
}

// reviewHref is a Document card's destination: its Review screen, or -- for a Document with no
// Approval Steps, which that screen has nothing to draw for -- its generic record page, the one
// place that can still show something. A Draft is the one status guaranteed to have zero steps by
// construction (2026-09-25, Tahap 4) and has its own real destination -- the submit wizard,
// reopened on this draft -- so it takes priority over the zero-steps fallback rather than landing
// on the generic record page like the other, pre-existing zero-step edge case still does.
func reviewHref(documentID string, steps int, status string) string {
	if status == action.DocumentStatusDraft {
		return fmt.Sprintf("/machines/%s/records/%s/continue-submit", action.DocumentMachineID, documentID)
	}
	if steps == 0 {
		return fmt.Sprintf("/machines/%s/records/%s", action.DocumentMachineID, documentID)
	}
	return fmt.Sprintf("/machines/%s/records/%s/review", action.DocumentMachineID, documentID)
}
