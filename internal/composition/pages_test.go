package composition

import (
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/rendering"
)

func task(id, project, assignee, status, due string) *data.Record {
	return rec(id, map[string]any{
		"fld_project":  project,
		"fld_assignee": assignee,
		"fld_status":   status,
		"fld_due_date": due,
	})
}

var projectLabels = map[string]string{"prj_1": "Apollo", "prj_2": "Borneo"}

// A Project's rollup counts its own Tasks only, and "open" means every status except done --
// not just todo, or in_progress would vanish from the count.
func TestBuildDashboard_ProjectRollups(t *testing.T) {
	projects := []*data.Record{
		rec("prj_1", map[string]any{"fld_name": "Apollo"}),
		rec("prj_2", map[string]any{"fld_name": "Borneo"}),
	}
	tasks := []*data.Record{
		task("tsk_1", "prj_1", "usr_ana", "todo", ""),
		task("tsk_2", "prj_1", "usr_ana", "in_progress", ""),
		task("tsk_3", "prj_1", "usr_ana", "done", ""),
		task("tsk_4", "prj_2", "usr_ana", "todo", ""),
	}

	got := buildDashboard(projects, tasks, nil)
	if len(got.Projects) != 2 {
		t.Fatalf("want a summary per Project, got %d", len(got.Projects))
	}
	if got.Projects[0].OpenTasks != 2 || got.Projects[0].TotalTasks != 3 {
		t.Errorf("Apollo open/total = %d/%d, want 2/3", got.Projects[0].OpenTasks, got.Projects[0].TotalTasks)
	}
	if got.Projects[1].OpenTasks != 1 || got.Projects[1].TotalTasks != 1 {
		t.Errorf("Borneo open/total = %d/%d, want 1/1", got.Projects[1].OpenTasks, got.Projects[1].TotalTasks)
	}
}

// A Task belonging to no Project must not be attributed to one, and must not crash the rollup.
func TestBuildDashboard_OrphanTaskCountsAgainstNoProject(t *testing.T) {
	projects := []*data.Record{rec("prj_1", map[string]any{"fld_name": "Apollo"})}
	tasks := []*data.Record{task("tsk_orphan", "", "usr_ana", "todo", "")}

	got := buildDashboard(projects, tasks, nil)
	if got.Projects[0].TotalTasks != 0 {
		t.Errorf("orphan Task must not be counted against Apollo, got total %d", got.Projects[0].TotalTasks)
	}
}

// Only in_review Documents are "pending"; the other three statuses are counted but not listed.
func TestBuildDashboard_DocumentStatusSplit(t *testing.T) {
	documents := []*data.Record{
		rec("doc_1", map[string]any{"fld_status": "draft"}),
		rec("doc_2", map[string]any{"fld_status": "in_review"}),
		rec("doc_3", map[string]any{"fld_status": "in_review"}),
		rec("doc_4", map[string]any{"fld_status": "approved"}),
		rec("doc_5", map[string]any{"fld_status": "rejected"}),
		rec("doc_6", map[string]any{"fld_status": "something_else"}),
	}

	got := buildDashboard(nil, nil, documents)
	s := got.Documents
	if s.Draft != 1 || s.InReview != 2 || s.Approved != 1 || s.Rejected != 1 {
		t.Errorf("counts draft/review/approved/rejected = %d/%d/%d/%d, want 1/2/1/1", s.Draft, s.InReview, s.Approved, s.Rejected)
	}
	if len(got.Pending) != 2 {
		t.Errorf("Pending lists the in_review Documents only, got %d", len(got.Pending))
	}
}

// My Tasks buckets by day and by status: done is Completed regardless of date, overdue and
// due-today share the Today column but count separately, and an unparseable date is Upcoming
// rather than dropped.
func TestBuildMyTasks_Buckets(t *testing.T) {
	tasks := []*data.Record{
		task("tsk_theirs", "prj_1", "usr_budi", "todo", "2026-09-09"),
		task("tsk_done", "prj_1", "usr_ana", "done", "2026-09-01"),
		task("tsk_overdue", "prj_1", "usr_ana", "todo", "2026-09-09"),
		task("tsk_today", "prj_1", "usr_ana", "in_progress", "2026-09-10"),
		task("tsk_later", "prj_2", "usr_ana", "todo", "2026-09-20"),
		task("tsk_undated", "prj_2", "usr_ana", "todo", ""),
	}

	got := buildMyTasks(tasks, projectLabels, "usr_ana", at(10))

	if len(got.Completed) != 1 || got.Completed[0].Task.ID != "tsk_done" {
		t.Errorf("Completed = %v, want just tsk_done", ids(got.Completed))
	}
	if want := []string{"tsk_overdue", "tsk_today"}; !equal(ids(got.Today), want) {
		t.Errorf("Today = %v, want %v", ids(got.Today), want)
	}
	if want := []string{"tsk_later", "tsk_undated"}; !equal(ids(got.Upcoming), want) {
		t.Errorf("Upcoming = %v, want %v", ids(got.Upcoming), want)
	}
	if got.Summary.Open != 4 || got.Summary.Overdue != 1 || got.Summary.DueToday != 1 {
		t.Errorf("summary open/overdue/today = %d/%d/%d, want 4/1/1", got.Summary.Open, got.Summary.Overdue, got.Summary.DueToday)
	}
	// The Project label is resolved, not left as the raw id.
	if got.Today[0].ProjectName != "Apollo" {
		t.Errorf("ProjectName = %q, want %q", got.Today[0].ProjectName, "Apollo")
	}
}

// Sprint counts every Task including done, but workload and attention consider only open ones.
func TestBuildSprint_CountsAndAttention(t *testing.T) {
	users := []*data.Record{
		rec("usr_ana", map[string]any{"fld_name": "Ana"}),
		rec("usr_budi", map[string]any{"fld_name": "Budi"}),
	}
	tasks := []*data.Record{
		task("tsk_1", "prj_1", "usr_ana", "todo", "2026-09-09"),        // overdue -> attention
		task("tsk_2", "prj_1", "usr_ana", "in_progress", "2026-09-10"), // due today -> attention
		task("tsk_3", "prj_1", "usr_ana", "todo", "2026-09-30"),        // later -> not attention
		task("tsk_4", "prj_2", "usr_budi", "done", "2026-09-01"),       // done -> no workload
	}

	got := buildSprint(tasks, users, projectLabels, at(10))

	s := got.Summary
	if s.Total != 4 || s.Open != 2 || s.InProgress != 1 || s.Done != 1 {
		t.Errorf("total/open/in_progress/done = %d/%d/%d/%d, want 4/2/1/1", s.Total, s.Open, s.InProgress, s.Done)
	}
	if want := []string{"tsk_1", "tsk_2"}; !equal(ids(got.Attention), want) {
		t.Errorf("Attention = %v, want %v", ids(got.Attention), want)
	}
	if len(got.Workload) != 2 {
		t.Fatalf("want a workload row per user, got %d", len(got.Workload))
	}
	if got.Workload[0].ActiveCards != 3 {
		t.Errorf("Ana's active cards = %d, want 3", got.Workload[0].ActiveCards)
	}
	if got.Workload[1].ActiveCards != 0 {
		t.Errorf("Budi's only Task is done, so active = %d, want 0", got.Workload[1].ActiveCards)
	}
}

func TestBuildCapacity(t *testing.T) {
	users := []*data.Record{
		rec("usr_ana", map[string]any{"fld_name": "Ana", "fld_weekly_capacity": float64(40)}),
		rec("usr_budi", map[string]any{"fld_name": "Budi"}), // no capacity declared
	}
	tasks := []*data.Record{
		task("tsk_1", "prj_1", "usr_ana", "todo", ""),
		task("tsk_2", "prj_1", "usr_ana", "done", ""),
		task("tsk_3", "prj_1", "usr_budi", "todo", ""),
	}

	got := buildCapacity(users, tasks)

	if got.TotalCapacity != 40 {
		t.Errorf("TotalCapacity = %d, want 40 (a user with no declared capacity adds nothing)", got.TotalCapacity)
	}
	if got.TotalActive != 2 {
		t.Errorf("TotalActive = %d, want 2", got.TotalActive)
	}
	if got.Members[0].ActiveCards != 1 || got.Members[0].TotalCards != 2 {
		t.Errorf("Ana active/total = %d/%d, want 1/2", got.Members[0].ActiveCards, got.Members[0].TotalCards)
	}
}

// The grid starts on Monday. Go's Weekday puts Sunday at 0, so a Sunday "now" must resolve to
// the Monday six days back, not the one the day after.
func TestBuildCalendarWeek_StartsOnMonday(t *testing.T) {
	for _, tc := range []struct{ name, now, wantFirst, wantLast string }{
		{"monday", "2026-09-14", "Mon Sep 14", "Sun Sep 20"},
		{"thursday", "2026-09-17", "Mon Sep 14", "Sun Sep 20"},
		{"sunday", "2026-09-20", "Mon Sep 14", "Sun Sep 20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse("2006-01-02", tc.now)
			if err != nil {
				t.Fatal(err)
			}
			days := buildCalendarWeek(nil, projectLabels, now)
			if len(days) != 7 {
				t.Fatalf("want 7 days, got %d", len(days))
			}
			if days[0].Label != tc.wantFirst || days[6].Label != tc.wantLast {
				t.Errorf("week = %s..%s, want %s..%s", days[0].Label, days[6].Label, tc.wantFirst, tc.wantLast)
			}
		})
	}
}

func TestBuildCalendarWeek_PlacesTasksAndMarksToday(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-09-17") // a Thursday
	tasks := []*data.Record{
		task("tsk_thu", "prj_1", "usr_ana", "todo", "2026-09-17"),
		task("tsk_sat", "prj_1", "usr_ana", "todo", "2026-09-19"),
		task("tsk_outside", "prj_1", "usr_ana", "todo", "2026-10-01"),
		task("tsk_undated", "prj_1", "usr_ana", "todo", ""),
	}

	days := buildCalendarWeek(tasks, projectLabels, now)

	todayCount := 0
	for i, d := range days {
		if d.IsToday {
			todayCount++
			if i != 3 {
				t.Errorf("Thursday is index 3 in a Monday-first week, got %d", i)
			}
		}
	}
	if todayCount != 1 {
		t.Errorf("exactly one day is today, got %d", todayCount)
	}
	if len(days[3].Tasks) != 1 || days[3].Tasks[0].Task.ID != "tsk_thu" {
		t.Errorf("Thursday tasks = %v, want [tsk_thu]", ids(days[3].Tasks))
	}
	if len(days[5].Tasks) != 1 {
		t.Errorf("Saturday tasks = %v, want one", ids(days[5].Tasks))
	}
	// A Task outside the week and one with no due date both simply do not appear.
	placed := 0
	for _, d := range days {
		placed += len(d.Tasks)
	}
	if placed != 2 {
		t.Errorf("placed %d Tasks in the grid, want 2", placed)
	}
}

func TestBuildActivityFeed_GroupsByDay(t *testing.T) {
	names := map[string]string{"usr_ana": "Ana Putri"}
	events := []*data.Record{
		{ID: "a1", Values: map[string]any{"fld_summary": "now", "fld_actor": "usr_ana"}, CreatedAt: at(10)},
		{ID: "a2", Values: map[string]any{"fld_summary": "yesterday", "fld_actor": "usr_ana"}, CreatedAt: at(9)},
		{ID: "a3", Values: map[string]any{"fld_summary": "older", "fld_actor": "usr_ana"}, CreatedAt: at(3)},
	}

	got := buildActivityFeed(events, names, at(10))

	if len(got.Today) != 1 || len(got.Yesterday) != 1 || len(got.Earlier) != 1 {
		t.Fatalf("today/yesterday/earlier = %d/%d/%d, want 1/1/1", len(got.Today), len(got.Yesterday), len(got.Earlier))
	}
	// Recent entries show a time; older ones need the date to be meaningful.
	if got.Today[0].When != "12:00" {
		t.Errorf("today's When = %q, want %q", got.Today[0].When, "12:00")
	}
	if got.Earlier[0].When != "2026-09-03 12:00" {
		t.Errorf("earlier When = %q, want a dated stamp", got.Earlier[0].When)
	}
	if got.Today[0].Actor != "Ana Putri" {
		t.Errorf("Actor = %q, want the resolved name", got.Today[0].Actor)
	}
}

// AutomationRules describes real Constraint metadata. The sequencing rule is appended because it
// lives in internal/action rather than in a Constraint, so it has nothing to be derived from.
func TestAutomationRules_DescribesRealConstraints(t *testing.T) {
	machines := []*domain.Machine{
		{
			ID:   "mch_project",
			Name: "Project",
			Fields: []domain.Field{
				{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus},
			},
			Constraints: []domain.Constraint{{
				ID:         "cst_project_done_no_open_tasks",
				On:         "fld_status",
				WhenEquals: "done",
				BlockIf: domain.RelationBlock{
					RelatedMachine: "mch_task",
					RelatedField:   "fld_project",
					Condition:      expression.Comparison{Field: "fld_status", Op: "!=", Value: "done"},
				},
			}},
		},
		{
			ID:     "mch_task",
			Name:   "Task",
			Fields: []domain.Field{{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus}},
		},
	}

	rules := AutomationRules(machines)

	if len(rules) != 2 {
		t.Fatalf("want one derived rule plus the appended sequencing rule, got %d", len(rules))
	}
	if rules[0].Name != "cst_project_done_no_open_tasks" {
		t.Errorf("Name = %q", rules[0].Name)
	}
	if want := `Project's Status becomes "done"`; rules[0].Trigger != want {
		t.Errorf("Trigger = %q, want %q", rules[0].Trigger, want)
	}
	// The related field is rendered by its human name, resolved through the related Machine.
	if want := `a related mch_task (via its fld_project field) has Status != "done"`; rules[0].Condition != want {
		t.Errorf("Condition = %q, want %q", rules[0].Condition, want)
	}
	if rules[1].Name != "Approval Step sequencing" {
		t.Errorf("last rule = %q, want the appended sequencing rule", rules[1].Name)
	}
}

// An Application with no Constraints still describes the sequencing rule, so the page is never
// blank.
func TestAutomationRules_AlwaysIncludesSequencing(t *testing.T) {
	rules := AutomationRules(nil)
	if len(rules) != 1 || rules[0].Name != "Approval Step sequencing" {
		t.Errorf("got %d rule(s), want only the sequencing rule", len(rules))
	}
}

func TestSameDay(t *testing.T) {
	if !SameDay(at(10), time.Date(2026, 9, 10, 23, 59, 0, 0, time.UTC)) {
		t.Error("same calendar day, different hours, must be same day")
	}
	if SameDay(at(10), at(11)) {
		t.Error("different days must not be same day")
	}
}

func ids(rows []rendering.TaskRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Task.ID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
