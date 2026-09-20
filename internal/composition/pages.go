package composition

import (
	"context"
	"fmt"
	"sort"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
)

// Machine ids these screens compose over. Named once here for the same reason internal/action
// names its own: Case 19's screens are hardcoded to real Machines, and a typo in a string literal
// repeated across six functions is invisible until the page renders empty.
const (
	taskMachineID     = "mch_task"
	projectMachineID  = "mch_project"
	userMachineID     = "mch_user"
	activityMachineID = "mch_activity"
)

// Declared Datasets and Measures these screens read (007 §7.2-§7.4), named here for the same
// reason the Machine ids above are. Naming an id to look a declaration up is the target pattern,
// not the hardcoding the conformance gates exist to stop -- the same distinction
// rendering.routeByID("nav_xxx") already draws: the *value* lives in metadata, the code only says
// which one it wants.
const (
	taskWorkloadDataset   = "ds_task_workload"
	taskByProjectDataset  = "ds_task_by_project"
	taskByStatusDataset   = "ds_task_by_status"
	userCapacityDataset   = "ds_user_capacity"
	documentStatusDataset = "ds_document_by_status"

	// msr_total/msr_active are deliberately record-agnostic: the same two ids are declared by
	// ds_task_workload, ds_task_by_project, ds_task_by_status and ds_document_by_status, so the
	// same constant reads a count of Tasks on one screen and a count of Documents on another. A
	// name like measureTotalCards would be wrong the moment the second Machine used it -- which
	// is exactly what happened when the Dashboard's document tiles started reading this id.
	measureTotal  = "msr_total"
	measureActive = "msr_active"
	// msr_total_capacity stays specific because its declaration is: it sums one particular
	// number Field (fld_weekly_capacity), so the name and the measure are equally narrow.
	measureTotalCapacity = "msr_total_capacity"
)

// mch_task's own fld_status option values, named rather than repeated as inline literals.
//
// This is an improvement in kind but not in level: the values still live in Go as well as in
// metadata/task.yaml's options:, so they remain compile-time-bound. What would actually remove
// them is a semantic marker on the option itself (something like `terminal: true` for "done"), so
// a screen could ask metadata which status means finished instead of knowing the word. That is
// the decomposition audit's P3, deliberately not built yet -- it is the least mature of the six
// variation points the audit mapped, and no second shape has forced it.
const (
	taskStatusTodo       = "todo"
	taskStatusInProgress = "in_progress"
	taskStatusDone       = "done"
)

// Dashboard is the landing dashboard's composed content (ROADMAP.md Phase 6's own forcing case):
// Project rollups, Document status counts, the Documents still in review, and a recent-events tail.
type Dashboard struct {
	Projects  []rendering.ProjectSummary
	Documents rendering.DocumentSummary
	Pending   []*data.Record
	Activity  []rendering.ActivityEntry
}

// DashboardData composes the dashboard. Each Machine is read once for the whole page -- three
// reads plus the activity feed's two, not one per Project.
func DashboardData(ctx context.Context, l *Loader, activityLimit int) (Dashboard, error) {
	projects, err := l.ListRecords(ctx, projectMachineID)
	if err != nil {
		return Dashboard{}, err
	}
	documents, err := l.ListRecords(ctx, action.DocumentMachineID)
	if err != nil {
		return Dashboard{}, err
	}
	activity, err := RecentActivity(ctx, l, activityLimit)
	if err != nil {
		return Dashboard{}, err
	}

	taskCounts, err := l.AggregateDataset(ctx, taskByProjectDataset)
	if err != nil {
		return Dashboard{}, err
	}
	docCounts, err := l.AggregateDataset(ctx, documentStatusDataset)
	if err != nil {
		return Dashboard{}, err
	}

	d := buildDashboard(projects, documents, taskCounts, docCounts)
	d.Activity = activity
	return d, nil
}

// buildDashboard splits what used to be one loop into its two genuinely different halves. Counting
// -- Tasks per Project, Documents per status -- is now declared (ds_task_by_project,
// ds_document_by_status). Collecting the in_review Documents themselves is not counting but
// selection, and stays here: picking records by a predicate is 007 §8's Query Model, which the
// decomposition audit explicitly recommends against building until something forces it.
func buildDashboard(projects, documents []*data.Record, taskCounts, docCounts Aggregation) Dashboard {
	var d Dashboard
	d.Projects = make([]rendering.ProjectSummary, 0, len(projects))
	for _, p := range projects {
		mine := taskCounts.ByDimension[p.ID]
		d.Projects = append(d.Projects, rendering.ProjectSummary{
			Project:    p,
			OpenTasks:  int(mine[measureActive]),
			TotalTasks: int(mine[measureTotal]),
		})
	}

	d.Documents = rendering.DocumentSummary{
		InReview: int(docCounts.ByDimension[action.DocumentStatusInReview][measureTotal]),
		Approved: int(docCounts.ByDimension[action.DocumentStatusApproved][measureTotal]),
		Rejected: int(docCounts.ByDimension[action.DocumentStatusRejected][measureTotal]),
	}
	for _, doc := range documents {
		if DisplayString(doc.Values[action.FieldDocumentStatus]) == action.DocumentStatusInReview {
			d.Pending = append(d.Pending, doc)
		}
	}
	return d
}

// MyTasks is one identity's personal work queue (ROADMAP.md Phase 14), bucketed by due date.
type MyTasks struct {
	Summary   rendering.MyTasksSummary
	Today     []rendering.TaskRow
	Upcoming  []rendering.TaskRow
	Completed []rendering.TaskRow
}

// PersonalTasks composes My Tasks for one identity. Bucketing reuses experience.EvaluateSLA
// (Phase 13) rather than re-deriving day-truncation logic.
//
// This is the one screen of the five the decomposition audit counted that a declared Dataset
// cannot express, and the reason is worth stating rather than leaving as an apparent oversight.
// Its counts are filtered by the viewing identity (assignee == userID), and a declared where: is
// a comparison against a literal, not against a value supplied per request -- so a Dataset would
// need parameterized filters. Its other two counts (Overdue, DueToday) compare a due date against
// now, which the equals/not_equals vocabulary cannot express at any level. Both are real gaps,
// neither has a second case yet, and inventing either one for this single screen is the premature
// declaration B5 exists to refuse.
func PersonalTasks(ctx context.Context, l *Loader, userID string, now time.Time) (MyTasks, error) {
	tasks, err := l.ListRecords(ctx, taskMachineID)
	if err != nil {
		return MyTasks{}, err
	}
	names, err := projectNames(ctx, l)
	if err != nil {
		return MyTasks{}, err
	}
	return buildMyTasks(tasks, names, userID, now), nil
}

func buildMyTasks(tasks []*data.Record, projects map[string]string, userID string, now time.Time) MyTasks {
	var out MyTasks
	for _, t := range tasks {
		if DisplayString(t.Values["fld_assignee"]) != userID {
			continue
		}
		row := rendering.TaskRow{Task: t, ProjectName: projects[DisplayString(t.Values["fld_project"])]}

		if DisplayString(t.Values["fld_status"]) == "done" {
			out.Completed = append(out.Completed, row)
			continue
		}
		out.Summary.Open++

		// A Task with no parseable due date is upcoming rather than dropped: "someday" is a
		// real answer, and silently hiding the row would be worse than showing it undated.
		due, err := time.Parse("2006-01-02", DisplayString(t.Values["fld_due_date"]))
		if err != nil {
			out.Upcoming = append(out.Upcoming, row)
			continue
		}
		status, label := experience.EvaluateSLA(due, now)
		switch {
		case status == experience.SLAOverdue:
			out.Summary.Overdue++
			out.Today = append(out.Today, row)
		case label == "Due today":
			out.Summary.DueToday++
			out.Today = append(out.Today, row)
		default:
			out.Upcoming = append(out.Upcoming, row)
		}
	}
	return out
}

// Sprint is the sprint dashboard's composed content (ROADMAP.md Phase 14). The mockup's
// points/burndown/blocked content is deliberately absent -- see rendering.SprintSummary's own doc
// comment for why.
type Sprint struct {
	Summary   rendering.SprintSummary
	Workload  []rendering.MemberCapacity
	Attention []rendering.TaskRow
}

func SprintDashboard(ctx context.Context, l *Loader, now time.Time) (Sprint, error) {
	tasks, err := l.ListRecords(ctx, taskMachineID)
	if err != nil {
		return Sprint{}, err
	}
	users, err := l.ListRecords(ctx, userMachineID)
	if err != nil {
		return Sprint{}, err
	}
	names, err := projectNames(ctx, l)
	if err != nil {
		return Sprint{}, err
	}
	byStatus, err := l.AggregateDataset(ctx, taskByStatusDataset)
	if err != nil {
		return Sprint{}, err
	}
	workload, err := l.AggregateDataset(ctx, taskWorkloadDataset)
	if err != nil {
		return Sprint{}, err
	}

	return buildSprint(tasks, users, names, now, byStatus, workload), nil
}

// buildSprint reads two Datasets, and the second one is the point: ds_task_workload is the same
// declaration Team Capacity composes from, reused here unchanged. That reuse is what makes a
// Dataset a "named, reusable semantic data definition" (007 §7.2) rather than a per-screen config
// block -- two screens now agree on what "active cards per assignee" means because they read one
// declaration, not because two Go loops happen to be written the same way.
//
// The Attention list stays a loop for the same reason buildDashboard's Pending does: it selects
// records rather than counting them, and its predicate is temporal (overdue or due today against
// now), which the declared where: shape cannot express at all.
func buildSprint(tasks, users []*data.Record, projects map[string]string, now time.Time, byStatus, workload Aggregation) Sprint {
	var out Sprint

	out.Summary.Total = int(byStatus.Total[measureTotal])
	out.Summary.Open = int(byStatus.ByDimension[taskStatusTodo][measureTotal])
	out.Summary.InProgress = int(byStatus.ByDimension[taskStatusInProgress][measureTotal])
	out.Summary.Done = int(byStatus.ByDimension[taskStatusDone][measureTotal])

	active := workload.ByDimension

	for _, t := range tasks {
		if DisplayString(t.Values["fld_status"]) == taskStatusDone {
			continue
		}
		if due, err := time.Parse("2006-01-02", DisplayString(t.Values["fld_due_date"])); err == nil {
			if slaStatus, label := experience.EvaluateSLA(due, now); slaStatus == experience.SLAOverdue || label == "Due today" {
				out.Attention = append(out.Attention, rendering.TaskRow{
					Task:        t,
					ProjectName: projects[DisplayString(t.Values["fld_project"])],
				})
			}
		}
	}

	out.Workload = make([]rendering.MemberCapacity, 0, len(users))
	for _, u := range users {
		out.Workload = append(out.Workload, rendering.MemberCapacity{
			User:        u,
			ActiveCards: int(active[u.ID][measureActive]),
		})
	}
	return out
}

// Capacity is the Team Capacity screen's composed content (ROADMAP.md Phase 14).
type Capacity struct {
	Members       []rendering.MemberCapacity
	TotalCapacity int
	TotalActive   int
}

func TeamCapacity(ctx context.Context, l *Loader) (Capacity, error) {
	workload, err := l.AggregateDataset(ctx, taskWorkloadDataset)
	if err != nil {
		return Capacity{}, err
	}
	capacity, err := l.AggregateDataset(ctx, userCapacityDataset)
	if err != nil {
		return Capacity{}, err
	}

	// Users are still read directly: the table lists one row per member, and a Dataset returns
	// numbers, not records.
	users, err := l.ListRecords(ctx, userMachineID)
	if err != nil {
		return Capacity{}, err
	}
	return buildCapacity(users, workload, capacity), nil
}

// buildCapacity is the first screen composed from declared Datasets rather than a hand-written
// count loop (007 §7.2-§7.4; the decomposition audit's P1). What used to be three literal field
// ids and a `!= "done"` comparison in Go is now metadata/task.yaml's ds_task_workload and
// metadata/user.yaml's ds_user_capacity; this function's remaining job is binding -- deciding
// which Measure lands in which view-model field, which stays code by design (007 §11.3).
//
// TotalActive deliberately sums the per-member numbers instead of reading the Dataset's own
// Total: those two differ, and the difference is visible. Total counts every open Task including
// ones assigned to nobody (or to a since-deleted identity), while the table below it lists only
// real Users -- so using Total would print a header number the rows underneath can't add up to.
func buildCapacity(users []*data.Record, workload, capacity Aggregation) Capacity {
	out := Capacity{
		Members:       make([]rendering.MemberCapacity, 0, len(users)),
		TotalCapacity: int(capacity.Total[measureTotalCapacity]),
	}
	for _, u := range users {
		mine := workload.ByDimension[u.ID]
		out.TotalActive += int(mine[measureActive])
		out.Members = append(out.Members, rendering.MemberCapacity{
			User:        u,
			ActiveCards: int(mine[measureActive]),
			TotalCards:  int(mine[measureTotal]),
		})
	}
	return out
}

// CalendarWeek composes the Monday-Sunday week containing now, one column per day (ROADMAP.md
// Phase 14). Tasks with no due date are simply absent -- a week grid has nowhere to put them.
func CalendarWeek(ctx context.Context, l *Loader, now time.Time) ([]rendering.CalendarDay, error) {
	tasks, err := l.ListRecords(ctx, taskMachineID)
	if err != nil {
		return nil, err
	}
	names, err := projectNames(ctx, l)
	if err != nil {
		return nil, err
	}
	return buildCalendarWeek(tasks, names, now), nil
}

func buildCalendarWeek(tasks []*data.Record, projects map[string]string, now time.Time) []rendering.CalendarDay {
	byDate := make(map[string][]rendering.TaskRow)
	for _, t := range tasks {
		due := DisplayString(t.Values["fld_due_date"])
		if due == "" {
			continue
		}
		byDate[due] = append(byDate[due], rendering.TaskRow{
			Task:        t,
			ProjectName: projects[DisplayString(t.Values["fld_project"])],
		})
	}

	// Go's Weekday starts the week on Sunday; this grid starts on Monday, so Sunday needs the
	// full week subtracted rather than a negative offset.
	offset := int(now.Weekday()) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	monday := now.AddDate(0, 0, -offset)

	days := make([]rendering.CalendarDay, 0, 7)
	for i := 0; i < 7; i++ {
		day := monday.AddDate(0, 0, i)
		days = append(days, rendering.CalendarDay{
			Label:   day.Format("Mon Jan 2"),
			IsToday: SameDay(day, now),
			Tasks:   byDate[day.Format("2006-01-02")],
		})
	}
	return days
}

// ActivityFeed is the cross-Machine event feed grouped by day (ROADMAP.md Phase 14) -- the same
// mch_activity data the dashboard shows as a flat tail, shaped for its own page.
type ActivityFeed struct {
	Today     []rendering.ActivityEntry
	Yesterday []rendering.ActivityEntry
	Earlier   []rendering.ActivityEntry
}

func GroupedActivity(ctx context.Context, l *Loader, limit int, now time.Time) (ActivityFeed, error) {
	events, names, err := recentEvents(ctx, l, limit)
	if err != nil {
		return ActivityFeed{}, err
	}
	return buildActivityFeed(events, names, now), nil
}

func buildActivityFeed(events []*data.Record, names map[string]string, now time.Time) ActivityFeed {
	yesterday := now.AddDate(0, 0, -1)

	var feed ActivityFeed
	for _, e := range events {
		entry := rendering.ActivityEntry{
			Summary: DisplayString(e.Values["fld_summary"]),
			Actor:   names[DisplayString(e.Values["fld_actor"])],
		}
		switch {
		case SameDay(e.CreatedAt, now):
			entry.When = e.CreatedAt.Format("15:04")
			feed.Today = append(feed.Today, entry)
		case SameDay(e.CreatedAt, yesterday):
			entry.When = e.CreatedAt.Format("15:04")
			feed.Yesterday = append(feed.Yesterday, entry)
		default:
			entry.When = e.CreatedAt.Format("2006-01-02 15:04")
			feed.Earlier = append(feed.Earlier, entry)
		}
	}
	return feed
}

// RecentActivity is the dashboard's flat newest-first tail of the same events.
func RecentActivity(ctx context.Context, l *Loader, limit int) ([]rendering.ActivityEntry, error) {
	events, names, err := recentEvents(ctx, l, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]rendering.ActivityEntry, 0, len(events))
	for _, e := range events {
		entries = append(entries, rendering.ActivityEntry{
			Summary: DisplayString(e.Values["fld_summary"]),
			Actor:   names[DisplayString(e.Values["fld_actor"])],
			When:    e.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	return entries, nil
}

// recentEvents returns mch_activity newest-first (capped at limit, or all when limit is 0) plus
// an actor-id -> display-name map: the shared read behind both activity shapes.
//
// The sort runs over a copy. The Loader hands back its cached slice, so sorting in place would
// reorder mch_activity for every later reader in the same request.
func recentEvents(ctx context.Context, l *Loader, limit int) ([]*data.Record, map[string]string, error) {
	cached, err := l.ListRecords(ctx, activityMachineID)
	if err != nil {
		return nil, nil, err
	}
	events := make([]*data.Record, len(cached))
	copy(events, cached)
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	users, err := l.ListRecords(ctx, userMachineID)
	if err != nil {
		return nil, nil, err
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID] = DisplayString(u.Values["fld_name"])
	}
	return events, names, nil
}

// AutomationRules describes this Application's real Constraint, Event, and Action metadata/
// behavior as Trigger/Condition/Action rows (ROADMAP.md Phase 14) -- a read-only description of
// what already exists, not a generic automation engine and not fictional example workflows.
//
// It composes over metadata alone and touches no records, so it takes the Machines directly
// rather than a Loader.
func AutomationRules(machines []*domain.Machine) []rendering.AutomationRule {
	var rules []rendering.AutomationRule
	for _, m := range machines {
		for _, c := range m.Constraints {
			onField, _ := m.FieldByID(c.On)
			relatedFieldName := c.BlockIf.Condition.Field
			if related := findMachine(machines, c.BlockIf.RelatedMachine); related != nil {
				if f, ok := related.FieldByID(c.BlockIf.Condition.Field); ok {
					relatedFieldName = f.Name
				}
			}
			rules = append(rules, rendering.AutomationRule{
				Name:      c.ID,
				Trigger:   fmt.Sprintf("%s's %s becomes %q", m.Name, onField.Name, c.WhenEquals),
				Condition: fmt.Sprintf("a related %s (via its %s field) has %s %s %q", c.BlockIf.RelatedMachine, c.BlockIf.RelatedField, relatedFieldName, c.BlockIf.Condition.Op, c.BlockIf.Condition.Value),
				Action:    "Block the transition (422)",
			})
		}
	}

	for _, m := range machines {
		for _, e := range m.Events {
			onField, _ := m.FieldByID(e.On)
			trigger := fmt.Sprintf("%s's %s changes", m.Name, onField.Name)
			if e.WhenEquals != "" {
				trigger = fmt.Sprintf("%s's %s becomes %q", m.Name, onField.Name, e.WhenEquals)
			}
			rules = append(rules, rendering.AutomationRule{
				Name:      e.ID,
				Trigger:   trigger,
				Condition: "(none)",
				Action:    fmt.Sprintf("Run %s: %q", e.Then.Name, e.Then.Summary),
			})
		}
	}

	// The Approval Step's sequencing rule lives in internal/action rather than in Constraint
	// metadata, so it has no Constraint to be derived from and is stated here instead.
	return append(rules, rendering.AutomationRule{
		Name:      "Approval Step sequencing",
		Trigger:   "POST /machines/mch_approval_step/records/{id}/decide",
		Condition: "sequential mode: every earlier-sequence step on the same Document is already approved; parallel mode: always",
		Action:    "Record the decision; recompute the Document's aggregate status (approved once every step is approved, rejected if any step is)",
	})
}

func findMachine(machines []*domain.Machine, id string) *domain.Machine {
	for _, m := range machines {
		if m.ID == id {
			return m
		}
	}
	return nil
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
