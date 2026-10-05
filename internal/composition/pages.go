package composition

import (
	"context"
	"fmt"
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
	dashboardTasksDataset = "ds_all_tasks"
	myTasksDataset        = "ds_my_tasks"
	// Board Settings' three (Case 19 PM06): the Lists and Labels a board is built from, and how many cards
	// carry each Label.
	boardListsDataset  = "ds_board_lists"
	boardLabelsDataset = "ds_board_labels"
	labelUsageDataset  = "ds_label_usage"
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

// DashboardData composes the dashboard (Case 19 PM04). Every Task figure on it -- the four tiles, the
// per-project rows, the per-person bars -- is derived from one bounded selection of Task records
// (ds_all_tasks) through the Machine's own `completion:`, so "finished" has one definition on this screen
// rather than the Dataset's `value: done` and the board's circle each holding one. The mockup has nothing
// else on the page, so nothing else is composed.
func DashboardData(ctx context.Context, l *Loader, now time.Time) (rendering.DashboardContent, error) {
	tasks, err := l.SelectRelated(ctx, dashboardTasksDataset, expression.Context{})
	if err != nil {
		return rendering.DashboardContent{}, err
	}
	projects, err := l.ListRecords(ctx, projectMachineID)
	if err != nil {
		return rendering.DashboardContent{}, err
	}
	people, err := l.PersonNames(ctx)
	if err != nil {
		return rendering.DashboardContent{}, err
	}

	d := buildDashboard(tasks.Records, projects, people, l.Machine(taskMachineID), l.Machine(projectMachineID), now)
	d.TasksTruncation = rendering.Truncation{Hit: tasks.Truncated, Limit: tasks.Limit}
	return d, nil
}

// buildDashboard is the pure half. Tasks are the only input for every figure on the page.
//
// Which Field names a Task's Project and its person come from the Machine rather than from a literal: the
// person is the `person` card_fields role, the Project is the one Field referencing the Project Machine.
// A Task Machine with neither simply yields no per-project or per-person rows.
func buildDashboard(tasks, projects []*data.Record, people map[string]string, taskMachine, projectMachine *domain.Machine, now time.Time) rendering.DashboardContent {
	var d rendering.DashboardContent
	projectField := fieldReferencing(taskMachine, projectMachineID)
	personField := FieldForRole(taskMachine, domain.CardFieldRolePerson)

	perProject := map[string]taskLoad{}
	perPerson := map[string]taskLoad{}
	open, overdue := 0, 0
	for _, t := range tasks {
		done := IsComplete(taskMachine, t)
		if !done {
			open++
			if isOverdue(taskMachine, t, now) {
				overdue++
			}
		}
		bump := func(m map[string]taskLoad, key string) {
			if key == "" {
				return
			}
			l := m[key]
			l.total++
			if !done {
				l.open++
			}
			m[key] = l
		}
		bump(perProject, DisplayString(t.Values[projectField]))
		bump(perPerson, DisplayString(t.Values[personField]))
	}
	total := len(tasks)
	completed := total - open

	d.Tiles = []rendering.SummaryItem{
		{Label: "All cards", Value: fmt.Sprint(total), Hint: projectCountHint(perProject)},
		{Label: "Open", Value: fmt.Sprint(open), Hint: "Not finished yet", Tone: domain.ToneNeutral},
		{Label: "Overdue", Value: fmt.Sprint(overdue), Hint: "Past its due date", Tone: toneIf(overdue > 0, domain.ToneBad)},
		{Label: "Completed", Value: fmt.Sprint(completed), Hint: fmt.Sprintf("%d%% of all cards", percentOf(completed, total)), Tone: toneIf(completed > 0, domain.ToneGood)},
	}

	d.Projects = make([]rendering.ProjectSummary, 0, len(projects))
	for _, p := range projects {
		mine := perProject[p.ID]
		shape := ProjectedByRole(projectMachine, p, nil)
		d.Projects = append(d.Projects, rendering.ProjectSummary{
			OpenTasks:  mine.open,
			TotalTasks: mine.total,
			Name:       shape[string(domain.CardFieldRoleTitle)],
			Status:     shape[string(domain.CardFieldRoleStatus)],
		})
	}

	for id, l := range perPerson {
		// An assignee the Workspace no longer has a name for is left off the list rather than shown as an id.
		// The tiles above still count their cards, so the figures can differ.
		if people[id] == "" {
			continue
		}
		d.People = append(d.People, rendering.PersonLoad{Name: people[id], Open: l.open, Total: l.total})
	}
	sort.Slice(d.People, func(i, j int) bool {
		a, b := d.People[i], d.People[j]
		if a.Open != b.Open {
			return a.Open > b.Open
		}
		return a.Name < b.Name
	})

	return d
}

// isOverdue reports whether an unfinished record's date has passed. The caller has already ruled out
// finished ones -- a finished card is never overdue, which is the sentence the Dashboard's own subtitle says.
func isOverdue(m *domain.Machine, r *data.Record, now time.Time) bool {
	due, err := time.Parse("2006-01-02", DisplayString(r.Values[FieldForRole(m, domain.CardFieldRoleDate)]))
	if err != nil {
		return false
	}
	status, _ := experience.EvaluateSLA(due, now)
	return status == experience.SLAOverdue
}

// fieldReferencing is the id of the Field on m that points at the Machine target, "" when there is none. The
// first such Field wins; a Machine with two Fields to the same target would need to say which, and Task has one.
func fieldReferencing(m *domain.Machine, target string) string {
	for _, f := range m.Fields {
		if f.IsReference() && f.RelatedMachine == target {
			return f.ID
		}
	}
	return ""
}

// taskLoad is a count of Tasks and how many of them are unfinished, per Project or per person.
type taskLoad struct{ open, total int }

func projectCountHint(perProject map[string]taskLoad) string {
	if len(perProject) == 1 {
		return "In 1 project"
	}
	return fmt.Sprintf("In %d projects", len(perProject))
}

func percentOf(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	return part * 100 / whole
}

// toneIf is the signal tone when a figure is worth signalling and the neutral one when it is zero -- a red
// "0" would read as a problem on a screen whose point is that there is none.
func toneIf(signal bool, tone domain.BadgeTone) domain.BadgeTone {
	if signal {
		return tone
	}
	return domain.ToneNeutral
}

// MyTasks is one identity's personal work queue (development-history.md Phase 14), bucketed by due date:
// Overdue, due within the next seven days (today included), Later (including undated), and Completed --
// "completed" being the Machine's own `completion:` declaration, not a status literal.
type MyTasks struct {
	Overdue   []rendering.TaskRow
	Next7Days []rendering.TaskRow
	Later     []rendering.TaskRow
	Completed []rendering.TaskRow
}

// myTasksWindowDays is how far ahead "Next 7 days" reaches, counting today.
const myTasksWindowDays = 7

// MyNotifications lists the viewer's own mch_notification records, newest first (Flow 2 gap study
// Tahap 6) -- the same "filter by identity in Go" shape PersonalTasks/PendingApprovalCount already
// are (007 §20's named, accepted pattern for this class of per-viewer worklist), keyed by the
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

// PersonalTasks composes My Tasks for one identity. Bucketing reuses experience.EvaluateSLA
// (Phase 13) rather than re-deriving day-truncation logic.
//
// This is the one screen of the five the decomposition audit counted that a declared Dataset
// cannot express. Its counts are filtered by the viewing identity (assignee == userID), and a
// declared where: is a comparison against a literal, not against a value supplied per request --
// so a Dataset would need parameterized filters. Its other two counts (Overdue, DueToday) compare
// a due date against now, which the equals/not_equals vocabulary cannot express at any level.
//
// **This comment used to end "neither has a second case yet", and that was already false when it
// was written** (kajian 2026-09-29, ROADMAP.md). Seven exported functions in this package take a
// viewing identity and then filter records in Go after reading them all: ApprovalInbox,
// PendingApprovalCount, AssignedToMe, MyNotifications, UnreadNotificationCount, PersonalTasks and
// ReviewStepForDocument. MyNotifications' own comment, forty lines up, says so in as many words --
// "the same 'filter by identity in Go' shape PersonalTasks/PendingApprovalCount already are". Two
// comments in one file contradicting each other is how a met trigger stays invisible: CLAUDE.md's
// step 3 fires on the *second* case of a shape, and this is the seventh.
//
// **The two halves are not equally ripe, which is why only one moved.** The identity half's trigger
// is met seven times over. The date half has roughly two cases (this screen and
// experience.EvaluateSLA's own day truncation), so `lt`/`gt` in internal/expression stays parked --
// B5 still refuses it. Whoever builds the identity filter should not take the date one along for
// the ride on the strength of this comment.
//
// **And "seven cases" is right about the coupling and wrong as a forcing case -- measured 2026-09-29,
// after this comment had already carried the number through three answers unchecked.** All seven do
// filter by identity in Go. **None of them is closed by an identity-aware `where:` alone.** Four
// (ApprovalInbox, AssignedToMe, MyNotifications, ReviewStepForDocument) need record *selection*,
// which a Dataset cannot do -- it produces numbers. PendingApprovalCount reads a *sibling*
// (behavior.CanAct) and can never be a Dataset at all. UnreadNotificationCount and this screen need
// a second predicate, and Measure.Where is a single expression.Comparison with no conjunction. So
// the sentinel on its own would have landed with zero consumers, which is the premature declaration
// B5 refuses -- the very rule this comment invokes two paragraphs up.
//
// The lesson is the one about numbers, not about filters: a count of *sites sharing a shape* is not a
// count of *cases a primitive would close*, and the two were conflated here until someone checked
// each site against the thing being built. ROADMAP.md's "(c)" entry carries the per-site
// classification, and the work that follows from it is `select: records` (007 §7.7-§7.9), not the
// sentinel alone.
//
// It was also **detached**: this block sat above MyNotifications' comment with no blank line
// between them, so godoc fused the two and PersonalTasks itself had no documentation at all -- and
// the text it fused into was one of the cases contradicting it.
func PersonalTasks(ctx context.Context, l *Loader, userID string, now time.Time) (MyTasks, error) {
	// The assignee filter is declared now (ds_my_tasks) and runs in the database, so this no longer
	// reads every Task in the Workspace to keep the viewer's own. What did *not* move is the status
	// bucketing below: `done` sends a Task to the Completed section rather than dropping it, so it is
	// not a filter and pushing it down would empty that section.
	tasks, err := l.SelectDataset(ctx, myTasksDataset, expression.Context{CurrentUser: userID})
	if err != nil {
		return MyTasks{}, err
	}
	names, err := projectNames(ctx, l)
	if err != nil {
		return MyTasks{}, err
	}
	m := l.Machine(taskMachineID)
	tags, err := l.CardTagsFor(ctx, m, tasks)
	if err != nil {
		return MyTasks{}, err
	}
	columns, err := l.BoardColumns(ctx, m, m.DefaultView())
	if err != nil {
		return MyTasks{}, err
	}
	return buildMyTasks(tasks, names, now, m, tags, columns), nil
}

// listName is the name of the list a record sits in: its value of the Machine's default View's `group_by`
// Field, resolved through that View's columns for a relation-grouped board (the list's own title) and read
// as it is for an option-grouped one. "" when the default View is not grouped.
func listName(m *domain.Machine, r *data.Record, columns []experience.Column) string {
	group := m.DefaultView().GroupBy
	if group == "" {
		return ""
	}
	value := DisplayString(r.Values[group])
	for _, c := range columns {
		if c.ID != "" && c.ID == value {
			return c.Label
		}
	}
	if f, ok := m.FieldByID(group); ok && f.Type != domain.FieldTypeRelation {
		return value
	}
	return ""
}

// taskRow resolves one Task row's display shape from the Machine's own card_fields declaration. All
// three screens that render a Task row go through it -- My Tasks, the Sprint dashboard's Attention
// list and the Calendar week, which share taskRowList's markup -- so none of them names fld_title,
// fld_status or fld_due_date, and a Machine whose Fields are called something else still renders
// (2026-09-28; this is what let mytasks.templ/calendar.templ stop reading Values["fld_title"]).
//
// **DueText stays the *raw* stored value rather than the projected display string**, and that is not the
// same decision as the one this comment used to describe. Projection formats a date for reading
// ("2 Jan 2006"), which nothing can parse back, so the bucketing below needs the stored form -- but the
// *badge* is resolved here now instead of in the Page. `rendering.TaskRow.Due any` existed so
// mytasks.templ could parse a date and evaluate a rule against `time.Now()`; that is 007 §4.4's division
// and §4.6's determinism rule, and it is why `now` is a parameter.
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

// buildMyTasks buckets the viewer's Tasks. Completed is the Machine's own declaration (IsComplete) and is
// kept apart from the dated buckets whatever the date says; an undated Task is Later rather than dropped
// ("someday" is a real answer). Overdue and Next 7 days are ordered by date, so the soonest is on top.
func buildMyTasks(tasks []*data.Record, projects map[string]string, now time.Time, taskMachine *domain.Machine, tags map[string][]rendering.CardTag, columns []experience.Column) MyTasks {
	var out MyTasks
	y, mo, d := now.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
	for _, t := range tasks {
		row := taskRow(t, projects, taskMachine, now)
		done := IsComplete(taskMachine, t)
		row.Complete = cardComplete(taskMachine, t)
		row.Date = experience.ResolveCardDate(t.Values[FieldForRole(taskMachine, domain.CardFieldRoleDate)], done, now)
		row.ListName = listName(taskMachine, t, columns)
		row.Tags = tags[t.ID]

		if done {
			out.Completed = append(out.Completed, row)
			continue
		}
		due, err := time.Parse("2006-01-02", row.DueText)
		status := experience.SLAOK
		if err == nil {
			status, _ = experience.EvaluateSLA(due, now)
		}
		switch {
		case err != nil:
			out.Later = append(out.Later, row)
		case status == experience.SLAOverdue:
			out.Overdue = append(out.Overdue, row)
		case due.Before(today.AddDate(0, 0, myTasksWindowDays+1)):
			out.Next7Days = append(out.Next7Days, row)
		default:
			out.Later = append(out.Later, row)
		}
	}
	byDue := func(rows []rendering.TaskRow) {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].DueText < rows[j].DueText })
	}
	byDue(out.Overdue)
	byDue(out.Next7Days)
	return out
}

// maxCalendarWeeks bounds how far from this week the Calendar will navigate: ten years either way is more
// than any plan reaches, and it keeps `?week=` from asking time.AddDate for a date it cannot represent.
const maxCalendarWeeks = 520

// CalendarWeek composes the Monday-Sunday week `weekOffset` weeks from the one containing now (0 is this
// week, -1 the last), one column per day (Case 19 PM05). Tasks with no due date are simply absent -- a week
// grid has nowhere to put them.
//
// The Tasks come from the same bounded selection the Dashboard counts (ds_all_tasks) instead of a whole-Machine
// read, and Truncation says so when that bound bit: a calendar that silently dropped the tail of a large
// Workspace would show an empty day for work that exists.
func CalendarWeek(ctx context.Context, l *Loader, now time.Time, weekOffset int) (rendering.CalendarContent, error) {
	sel, err := l.SelectRelated(ctx, dashboardTasksDataset, expression.Context{})
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

// BoardSettings composes Case 19 PM06: the Lists a board groups by and the Labels its cards can carry, each
// Label with how many cards carry it. Every figure and name is declared -- the two selections are Datasets
// (`ds_board_lists`, `ds_board_labels`), a Label's name and colour are its Machine's own `title` and `color`
// card_fields, and the usage count is `ds_label_usage` read by Label id -- so no Field id is named here.
//
// A Label whose colour is missing or outside the palette is drawn in the neutral entry, as CardTagsFor does
// for a chip: dropping it would hide that the Label exists. The two links to the Machines' own pages are
// built from each Dataset's source, because the generic Machine page is where a List or Label is created,
// renamed and reordered -- this screen is the overview, not a second editor.
func BoardSettings(ctx context.Context, l *Loader) (rendering.BoardSettingsContent, error) {
	listSel, err := l.SelectRelated(ctx, boardListsDataset, expression.Context{})
	if err != nil {
		return rendering.BoardSettingsContent{}, err
	}
	labelSel, err := l.SelectRelated(ctx, boardLabelsDataset, expression.Context{})
	if err != nil {
		return rendering.BoardSettingsContent{}, err
	}
	usage, err := l.AggregateDataset(ctx, labelUsageDataset)
	if err != nil {
		return rendering.BoardSettingsContent{}, err
	}
	listDS, _ := l.Dataset(boardListsDataset)
	labelDS, _ := l.Dataset(boardLabelsDataset)
	return buildBoardSettings(l.Machine(listDS.Source), listSel, l.Machine(labelDS.Source), labelSel, usage), nil
}

func buildBoardSettings(listMachine *domain.Machine, lists Selection, labelMachine *domain.Machine, labels Selection, usage Aggregation) rendering.BoardSettingsContent {
	c := rendering.BoardSettingsContent{
		ListsHref:        "/machines/" + listMachine.ID,
		LabelsHref:       "/machines/" + labelMachine.ID,
		ListsTruncation:  rendering.Truncation{Limit: lists.Limit, Hit: lists.Truncated},
		LabelsTruncation: rendering.Truncation{Limit: labels.Limit, Hit: labels.Truncated},
	}
	listTitle := FieldForRole(listMachine, domain.CardFieldRoleTitle)
	for _, r := range lists.Records {
		c.Lists = append(c.Lists, rendering.SettingsList{Name: DisplayString(r.Values[listTitle])})
	}
	labelTitle, labelColor := FieldForRole(labelMachine, domain.CardFieldRoleTitle), FieldForRole(labelMachine, domain.CardFieldRoleColor)
	for _, r := range labels.Records {
		color := domain.TagSlate
		if v := DisplayString(r.Values[labelColor]); domain.IsTagColor(v) {
			color = domain.TagColor(v)
		}
		c.Labels = append(c.Labels, rendering.SettingsLabel{
			Name:    DisplayString(r.Values[labelTitle]),
			Color:   color,
			Records: int(usage.ByDimension[r.ID][measureTotal]),
		})
	}
	return c
}
