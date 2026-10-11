package composition

import (
	"context"
	"sort"
	"time"

	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// Machine ids these screens compose over. Named once here for the same reason internal/action
// names its own: Case 19's screens are hardcoded to real Machines, and a typo in a string literal
// repeated across six functions is invisible until the page renders empty.
const (
	taskMachineID         = "mch_task"
	projectMachineID      = "mch_project"
	userMachineID         = "mch_user"
	notificationMachineID = "mch_notification"
)

// Declared Datasets and Measures these screens read (007 §7.2-§7.4), named here for the same
// reason the Machine ids above are. Naming an id to look a declaration up is the target pattern,
// not the hardcoding the conformance gates exist to stop -- the same distinction
// rendering.routeByID("nav_xxx") already draws: the *value* lives in metadata, the code only says
// which one it wants.
const (
	// The one declared Relation (007 §7.5, Tahap A) and its id. The Dataset id is domain's, not a
	// literal here: registry.KnownWorkflowEngines declares that the approval engine's `document` role must
	// provide it, so the selector and the requirement are one string (001 #8). Two agreeing literals is
	// how the same id reached the library and `default` while two other Workspaces went on 500ing.
	documentsWithStepsDataset = domain.DatasetDocumentsWithSteps
	documentStepsRelation     = "rel_steps"

	// Measure ids follow one shape: msr_total is the aggregate, and anything after it is the
	// qualifier narrowing what got aggregated. So the family reads as variations of one thing
	// rather than as unrelated words -- msr_total (every record), msr_total_open (only those not
	// finished), msr_total_capacity (summing one number Field instead of counting).
	//
	// msr_total alone is genuinely record-agnostic: counting records means the same thing on every
	// Machine, which is why one constant correctly reads Tasks on one screen and Documents on
	// another (measureTotalCards was wrong the moment the Dashboard's document tiles used it).
	//
	// The qualifiers are not agnostic, and the naming is what keeps that honest. msr_total_open
	// counts records whose status is not `done`, a meaning that exists only against one Machine's
	// own option list. It was briefly msr_active, until mch_project turned out to declare `active`
	// as a literal status option -- the same word would then have meant "not done" in one dataset
	// and "status == active" in another. A qualifier must not collide with a real option value.
	measureTotal = "msr_total"
)

// MyNotifications lists the viewer's own mch_notification records, newest first (Flow 2 gap study
// Tahap 6) -- the same "filter by identity in Go" shape PendingApprovalCount already
// is (007 §20's named, accepted pattern for this class of per-viewer worklist), keyed by the
// already-declared prm_edit_own_notification's own actor_field, fld_recipient.
func MyNotifications(ctx context.Context, l *Loader, viewerID string) ([]rendering.NotificationRow, error) {
	records, err := l.ListRecordsBy(ctx, notificationMachineID, "fld_recipient", viewerID)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })

	rows := make([]rendering.NotificationRow, 0, len(records))
	for _, r := range records {
		rows = append(rows, rendering.NotificationRow{
			ID:        r.ID,
			Message:   DisplayString(r.Values["fld_message"]),
			Link:      DisplayString(r.Values["fld_link"]),
			Unread:    DisplayString(r.Values["fld_read"]) == "unread",
			CreatedAt: r.CreatedAt.Format("2 Jan 2006 15:04"),
		})
	}
	return rows, nil
}

// UnreadNotificationCount is the bell badge's own count -- the same shape PendingApprovalCount
// already is (list the viewer's own rows, count in Go; no dedicated COUNT query exists anywhere in
// this codebase, so this doesn't invent one either).
func UnreadNotificationCount(ctx context.Context, l *Loader, viewerID string) (int, error) {
	records, err := l.ListRecordsBy(ctx, notificationMachineID, "fld_recipient", viewerID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, r := range records {
		if DisplayString(r.Values["fld_read"]) == "unread" {
			count++
		}
	}
	return count, nil
}

// calendarRange is a week's "05 – 11 Oct 2026": the year and month are written once when both ends share
// them, and each end carries its own when they do not.
func calendarRange(first, last time.Time) string {
	switch {
	case first.Year() != last.Year():
		return first.Format("02 Jan 2006") + " \u2013 " + last.Format("02 Jan 2006")
	case first.Month() != last.Month():
		return first.Format("02 Jan") + " \u2013 " + last.Format("02 Jan 2006")
	default:
		return first.Format("02") + " \u2013 " + last.Format("02 Jan 2006")
	}
}
