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
	taskWorkloadDataset  = "ds_task_workload"
	userCapacityDataset  = "ds_user_capacity"
	measureTotalCards    = "msr_total"
	measureActiveCards   = "msr_active"
	measureTotalCapacity = "msr_total_capacity"
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
	tasks, err := l.ListRecords(ctx, taskMachineID)
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

	d := buildDashboard(projects, tasks, documents)
	d.Activity = activity
	return d, nil
}

func buildDashboard(projects, tasks, documents []*data.Record) Dashboard {
	open := make(map[string]int, len(projects))
	total := make(map[string]int, len(projects))
	for _, t := range tasks {
		projectID, _ := t.Values["fld_project"].(string)
		total[projectID]++
		if DisplayString(t.Values["fld_status"]) != "done" {
			open[projectID]++
		}
	}

	var d Dashboard
	d.Projects = make([]rendering.ProjectSummary, 0, len(projects))
	for _, p := range projects {
		d.Projects = append(d.Projects, rendering.ProjectSummary{
			Project:    p,
			OpenTasks:  open[p.ID],
			TotalTasks: total[p.ID],
		})
	}

	for _, doc := range documents {
		switch DisplayString(doc.Values["fld_status"]) {
		case "in_review":
			d.Documents.InReview++
			d.Pending = append(d.Pending, doc)
		case "approved":
			d.Documents.Approved++
		case "rejected":
			d.Documents.Rejected++
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
	return buildSprint(tasks, users, names, now), nil
}

func buildSprint(tasks, users []*data.Record, projects map[string]string, now time.Time) Sprint {
	var out Sprint
	active := make(map[string]int, len(users))
	for _, t := range tasks {
		out.Summary.Total++
		status := DisplayString(t.Values["fld_status"])
		switch status {
		case "todo":
			out.Summary.Open++
		case "in_progress":
			out.Summary.InProgress++
		case "done":
			out.Summary.Done++
		}
		if status == "done" {
			continue
		}
		active[DisplayString(t.Values["fld_assignee"])]++
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
		out.Workload = append(out.Workload, rendering.MemberCapacity{User: u, ActiveCards: active[u.ID]})
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
	workload, ok := l.Dataset(taskMachineID, taskWorkloadDataset)
	if !ok {
		return Capacity{}, fmt.Errorf("composition: machine %s declares no dataset %s", taskMachineID, taskWorkloadDataset)
	}
	capacity, ok := l.Dataset(userMachineID, userCapacityDataset)
	if !ok {
		return Capacity{}, fmt.Errorf("composition: machine %s declares no dataset %s", userMachineID, userCapacityDataset)
	}

	users, err := l.ListRecords(ctx, userMachineID)
	if err != nil {
		return Capacity{}, err
	}
	tasks, err := l.ListRecords(ctx, taskMachineID)
	if err != nil {
		return Capacity{}, err
	}
	return buildCapacity(users, tasks, workload, capacity), nil
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
func buildCapacity(users, tasks []*data.Record, workload, capacity domain.Dataset) Capacity {
	byAssignee := Aggregate(workload, tasks).ByDimension

	out := Capacity{
		Members:       make([]rendering.MemberCapacity, 0, len(users)),
		TotalCapacity: int(Aggregate(capacity, users).Total[measureTotalCapacity]),
	}
	for _, u := range users {
		mine := byAssignee[u.ID]
		out.TotalActive += int(mine[measureActiveCards])
		out.Members = append(out.Members, rendering.MemberCapacity{
			User:        u,
			ActiveCards: int(mine[measureActiveCards]),
			TotalCards:  int(mine[measureTotalCards]),
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
