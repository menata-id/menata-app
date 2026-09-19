package composition

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/data"
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
	Pending []rendering.SummaryCard
	Buckets []string

	OverdueCount int
	TodayCount   int

	// Mine is every Document this identity submitted, which is a different question from
	// Pending's "waiting on me" and deliberately unfiltered by SLA.
	Mine []rendering.SummaryCard
}

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
func ApprovalInbox(ctx context.Context, l *Loader, userID string, now time.Time) (Inbox, error) {
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
	users, err := l.ListRecords(ctx, "mch_user")
	if err != nil {
		return Inbox{}, err
	}
	return buildInbox(steps, documents, activities, users, userID, now), nil
}

// buildInbox is the whole of the inbox's derivation, over records someone else already fetched.
// Keeping it free of I/O is what makes the sequencing, bucketing and submitter-resolution rules
// testable at all: they need four related record sets and a fixed clock, not a database.
func buildInbox(steps, documents, activities, users []*data.Record, userID string, now time.Time) Inbox {
	docByID := make(map[string]*data.Record, len(documents))
	for _, d := range documents {
		docByID[d.ID] = d
	}
	stepsByDoc := make(map[string][]*data.Record, len(documents))
	for _, s := range steps {
		docID := DisplayString(s.Values[action.FieldStepDocument])
		stepsByDoc[docID] = append(stepsByDoc[docID], s)
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID] = DisplayString(u.Values["fld_name"])
	}
	submitterByDoc := submittersFromActivity(activities)

	var inbox Inbox
	for _, s := range steps {
		if DisplayString(s.Values["fld_assignee"]) != userID {
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
		mode := DisplayString(doc.Values[action.FieldDocumentMode])
		if !action.CanDecide(mode, s, stepsByDoc[docID]) {
			continue
		}

		approved := approvedCount(stepsByDoc[docID])
		submitter := names[submitterByDoc[docID]]
		if submitter == "" {
			submitter = "someone"
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
		inbox.Pending = append(inbox.Pending, rendering.SummaryCard{
			AvatarInitials: Initials(submitter),
			Reference:      action.DocumentReference(doc.SortOrder),
			Title:          DisplayString(doc.Values["fld_title"]),
			Subtitle:       fmt.Sprintf("%s · %s · %d/%d approved · Submitted by %s", DisplayString(doc.Values["fld_document_type"]), mode, approved, len(stepsByDoc[docID]), submitter),
			StatusLabel:    DisplayString(doc.Values["fld_status"]),
			SLADue:         doc.Values["fld_due_date"],
			Href:           fmt.Sprintf("/machines/%s/records/%s", action.StepMachineID, s.ID),
		})
		inbox.Buckets = append(inbox.Buckets, bucket)
	}

	for _, d := range documents {
		if submitterByDoc[d.ID] != userID {
			continue
		}
		mode := DisplayString(d.Values[action.FieldDocumentMode])
		approved := approvedCount(stepsByDoc[d.ID])
		inbox.Mine = append(inbox.Mine, rendering.SummaryCard{
			AvatarInitials: Initials(names[userID]),
			Reference:      action.DocumentReference(d.SortOrder),
			Title:          DisplayString(d.Values["fld_title"]),
			Subtitle:       fmt.Sprintf("%s · %s · %d/%d approved", DisplayString(d.Values["fld_document_type"]), mode, approved, len(stepsByDoc[d.ID])),
			StatusLabel:    DisplayString(d.Values["fld_status"]),
			Href:           fmt.Sprintf("/machines/%s/records/%s", action.DocumentMachineID, d.ID),
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

// submittersFromActivity maps a Document id to the actor of its earliest logged event, which is
// its submission (Phase 13 logs "submitted" at creation). Events are sorted oldest-first and the
// first actor per Document wins, so a later decision event never overwrites the submitter.
func submittersFromActivity(activities []*data.Record) map[string]string {
	sorted := make([]*data.Record, len(activities))
	copy(sorted, activities)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	submitters := make(map[string]string, len(sorted))
	for _, a := range sorted {
		docID := DisplayString(a.Values["fld_record_id"])
		if _, ok := submitters[docID]; ok {
			continue
		}
		if actor := DisplayString(a.Values["fld_actor"]); actor != "" {
			submitters[docID] = actor
		}
	}
	return submitters
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
