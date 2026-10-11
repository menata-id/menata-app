package composition

import (
	"context"
	"sort"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/expression"
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
	calendarTasksDataset = "ds_all_tasks"
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

// taskRow resolves one Task row's display shape from the Machine's own card_fields declaration. All
// screens that render a Task row go through it, so none of them names fld_title, fld_status or
// fld_due_date, and a Machine whose Fields are called something else still renders.
//
// **DueText stays the *raw* stored value rather than the projected display string**, and that is not the
// same decision as the one this comment used to describe. Projection formats a date for reading
// ("2 Jan 2006"), which nothing can parse back, so the Calendar's bucketing by day needs the stored form -- but
// the *badge* is resolved here instead of in the Page: a Page parsing a date and evaluating a rule against
// `time.Now()` breaks 007 §4.4's division and §4.6's determinism rule, which is why `now` is a parameter.
func taskRow(t *data.Record, projects map[string]string, taskMachine *domain.Machine, now time.Time) rendering.TaskRow {
	shape := ProjectedByRole(taskMachine, t, nil)
	return rendering.TaskRow{
		Task:        t,
		ProjectName: projects[DisplayString(t.Values["fld_project"])],
		Title:       shape[string(domain.CardFieldRoleTitle)],
		Status:      shape[string(domain.CardFieldRoleStatus)],
		DueText:     DisplayString(t.Values[FieldForRole(taskMachine, domain.CardFieldRoleDate)]),
		SLA:         experience.ResolveSLABadge(t.Values[FieldForRole(taskMachine, domain.CardFieldRoleDate)], now),
	}
}

// maxCalendarWeeks bounds how far from this week the Calendar will navigate: ten years either way is more
// than any plan reaches, and it keeps `?week=` from asking time.AddDate for a date it cannot represent.
const maxCalendarWeeks = expression.MaxWeekOffset

// CalendarWeek composes the Monday-Sunday week `weekOffset` weeks from the one containing now (0 is this
// week, -1 the last), one column per day (Case 19 PM05). Tasks with no due date are simply absent -- a week
// grid has nowhere to put them.
//
// The Tasks come from the bounded selection ds_all_tasks instead of a whole-Machine
// read, and Truncation says so when that bound bit: a calendar that silently dropped the tail of a large
// Workspace would show an empty day for work that exists.
func CalendarWeek(ctx context.Context, l *Loader, now time.Time, weekOffset int) (rendering.CalendarContent, error) {
	sel, err := l.SelectRelated(ctx, calendarTasksDataset, expression.Context{})
	if err != nil {
		return rendering.CalendarContent{}, err
	}
	m := l.Machine(taskMachineID)
	weekOffset = clampWeekOffset(weekOffset)
	inWeek := tasksDueInWeek(sel.Records, m, calendarWeekStart(now, weekOffset))

	names, err := projectNames(ctx, l)
	if err != nil {
		return rendering.CalendarContent{}, err
	}
	people, err := l.PersonNames(ctx)
	if err != nil {
		return rendering.CalendarContent{}, err
	}
	tags, err := l.CardTagsFor(ctx, m, inWeek)
	if err != nil {
		return rendering.CalendarContent{}, err
	}
	c := buildCalendarWeek(inWeek, names, people, tags, now, weekOffset, m)
	c.Truncation = rendering.Truncation{Hit: sel.Truncated, Limit: sel.Limit}
	return c, nil
}

func clampWeekOffset(n int) int {
	if n > maxCalendarWeeks {
		return maxCalendarWeeks
	}
	if n < -maxCalendarWeeks {
		return -maxCalendarWeeks
	}
	return n
}

// calendarWeekStart is the Monday of the week weekOffset weeks from now. Go's Weekday starts the week on
// Sunday and this grid starts on Monday, so Sunday needs the full week subtracted rather than a negative
// offset.
func calendarWeekStart(now time.Time, weekOffset int) time.Time {
	offset := int(now.Weekday()) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return now.AddDate(0, 0, -offset+7*weekOffset)
}

// tasksDueInWeek keeps the Tasks whose date falls in the seven days from monday, so the tag join below reads
// chips for what the grid draws and not for every Task in the Workspace.
func tasksDueInWeek(tasks []*data.Record, taskMachine *domain.Machine, monday time.Time) []*data.Record {
	inWeek := make(map[string]bool, 7)
	for i := 0; i < 7; i++ {
		inWeek[monday.AddDate(0, 0, i).Format("2006-01-02")] = true
	}
	dateField := FieldForRole(taskMachine, domain.CardFieldRoleDate)
	var out []*data.Record
	for _, t := range tasks {
		if inWeek[DisplayString(t.Values[dateField])] {
			out = append(out, t)
		}
	}
	return out
}

// calendarRange is the toolbar's "05 – 11 Oct 2026": the year and month are written once when both ends share
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

func buildCalendarWeek(tasks []*data.Record, projects, people map[string]string, tags map[string][]rendering.CardTag, now time.Time, weekOffset int, taskMachine *domain.Machine) rendering.CalendarContent {
	dateField := FieldForRole(taskMachine, domain.CardFieldRoleDate)
	personField := FieldForRole(taskMachine, domain.CardFieldRolePerson)
	byDate := make(map[string][]rendering.TaskRow)
	for _, t := range tasks {
		row := taskRow(t, projects, taskMachine, now)
		if row.DueText == "" {
			continue
		}
		done := IsComplete(taskMachine, t)
		row.Complete = cardComplete(taskMachine, t)
		row.Date = experience.ResolveCardDate(t.Values[dateField], done, now)
		row.Tags = tags[t.ID]
		row.Assignee = people[DisplayString(t.Values[personField])]
		byDate[row.DueText] = append(byDate[row.DueText], row)
	}

	monday := calendarWeekStart(now, weekOffset)
	days := make([]rendering.CalendarDay, 0, 7)
	for i := 0; i < 7; i++ {
		day := monday.AddDate(0, 0, i)
		days = append(days, rendering.CalendarDay{
			Weekday: day.Format("Mon"),
			Date:    day.Format("02 Jan"),
			IsToday: SameDay(day, now),
			Tasks:   byDate[day.Format("2006-01-02")],
		})
	}
	return rendering.CalendarContent{
		Days:       days,
		Range:      calendarRange(monday, monday.AddDate(0, 0, 6)),
		WeekOffset: weekOffset,
	}
}

// projectNames maps a Project's id to its display name, the join three screens need to label a
// Task with the Project it belongs to.
func projectNames(ctx context.Context, l *Loader) (map[string]string, error) {
	projects, err := l.ListRecords(ctx, projectMachineID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(projects))
	for _, p := range projects {
		names[p.ID] = DisplayString(p.Values["fld_name"])
	}
	return names, nil
}

// SameDay reports whether two instants fall on the same calendar day.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
