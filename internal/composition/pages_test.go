package composition

import (
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
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

// The grid starts on Monday. Go's Weekday puts Sunday at 0, so a Sunday "now" must resolve to
// the Monday six days back, not the one the day after.
func TestBuildCalendarWeek_StartsOnMonday(t *testing.T) {
	for _, tc := range []struct{ name, now, wantFirst, wantLast, wantRange string }{
		{"monday", "2026-09-14", "14 Sep", "20 Sep", "14 \u2013 20 Sep 2026"},
		{"thursday", "2026-09-17", "14 Sep", "20 Sep", "14 \u2013 20 Sep 2026"},
		{"sunday", "2026-09-20", "14 Sep", "20 Sep", "14 \u2013 20 Sep 2026"},
		{"across a month", "2026-09-30", "28 Sep", "04 Oct", "28 Sep \u2013 04 Oct 2026"},
		{"across a year", "2026-12-31", "28 Dec", "03 Jan", "28 Dec 2026 \u2013 03 Jan 2027"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse("2006-01-02", tc.now)
			if err != nil {
				t.Fatal(err)
			}
			c := buildCalendarWeek(nil, projectLabels, nil, nil, now, 0, taskMachineForTest())
			if len(c.Days) != 7 {
				t.Fatalf("want 7 days, got %d", len(c.Days))
			}
			if c.Days[0].Weekday != "Mon" || c.Days[6].Weekday != "Sun" {
				t.Errorf("weekdays = %s..%s, want Mon..Sun", c.Days[0].Weekday, c.Days[6].Weekday)
			}
			if c.Days[0].Date != tc.wantFirst || c.Days[6].Date != tc.wantLast {
				t.Errorf("week = %s..%s, want %s..%s", c.Days[0].Date, c.Days[6].Date, tc.wantFirst, tc.wantLast)
			}
			if c.Range != tc.wantRange {
				t.Errorf("range = %q, want %q", c.Range, tc.wantRange)
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

	days := buildCalendarWeek(tasks, projectLabels, nil, nil, now, 0, taskMachineForTest()).Days

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

// Stepping a week moves the grid and nothing else: "today" is a fact about now, so it is not marked in a week
// that does not contain it, and a Task is placed in the week its date is in.
func TestBuildCalendarWeek_StepsByWeekOffset(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-09-17")
	tasks := []*data.Record{
		task("tsk_now", "prj_1", "usr_ana", "todo", "2026-09-17"),
		task("tsk_next", "prj_1", "usr_ana", "todo", "2026-09-22"), // the Tuesday after
	}

	next := buildCalendarWeek(tasks, projectLabels, nil, nil, now, 1, taskMachineForTest())
	if next.Days[0].Date != "21 Sep" || next.WeekOffset != 1 {
		t.Fatalf("next week starts %s at offset %d, want 21 Sep at 1", next.Days[0].Date, next.WeekOffset)
	}
	for _, d := range next.Days {
		if d.IsToday {
			t.Errorf("%s is marked today in a week that does not contain now", d.Date)
		}
	}
	if len(next.Days[1].Tasks) != 1 || next.Days[1].Tasks[0].Task.ID != "tsk_next" {
		t.Errorf("Tuesday of next week = %v, want [tsk_next]", ids(next.Days[1].Tasks))
	}

	prev := buildCalendarWeek(tasks, projectLabels, nil, nil, now, -1, taskMachineForTest())
	if prev.Days[0].Date != "07 Sep" {
		t.Errorf("last week starts %s, want 07 Sep", prev.Days[0].Date)
	}
}

// A card carries what the board's card does: the completion toggle, the date pill resolved from the
// completion (a finished Task is never overdue), its chips, and the assignee's name.
func TestBuildCalendarWeek_CardCarriesCompletionTagsAndAssignee(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-09-17")
	m := taskMachineForTest()
	open := task("tsk_open", "prj_1", "usr_ana", "todo", "2026-09-15")
	done := task("tsk_done", "prj_1", "usr_ana", "done", "2026-09-15")
	tags := map[string][]rendering.CardTag{"tsk_open": {{Label: "Design", Color: domain.TagColor("blue")}}}

	days := buildCalendarWeek([]*data.Record{open, done}, projectLabels, map[string]string{"usr_ana": "Ana Putri"}, tags, now, 0, m).Days
	tue := days[1].Tasks
	if len(tue) != 2 {
		t.Fatalf("Tuesday has %d tasks, want 2", len(tue))
	}
	byID := map[string]rendering.TaskRow{tue[0].Task.ID: tue[0], tue[1].Task.ID: tue[1]}

	if got := byID["tsk_open"]; got.Complete == nil || got.Complete.Done || got.Date.Tone != domain.ToneBad {
		t.Errorf("open past-due card = complete %+v tone %q, want an unticked circle and the overdue tone", got.Complete, got.Date.Tone)
	}
	if got := byID["tsk_done"]; got.Complete == nil || !got.Complete.Done || got.Date.Tone == domain.ToneBad {
		t.Errorf("finished card = complete %+v tone %q, want a ticked circle and never overdue", got.Complete, got.Date.Tone)
	}
	if got := byID["tsk_open"]; got.Assignee != "Ana Putri" || len(got.Tags) != 1 {
		t.Errorf("open card assignee %q tags %d, want Ana Putri and one chip", got.Assignee, len(got.Tags))
	}
}

func TestClampWeekOffset(t *testing.T) {
	for in, want := range map[int]int{0: 0, 3: 3, -3: -3, 1 << 40: maxCalendarWeeks, -(1 << 40): -maxCalendarWeeks} {
		if got := clampWeekOffset(in); got != want {
			t.Errorf("clampWeekOffset(%d) = %d, want %d", in, got, want)
		}
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

// taskMachineForTest is mch_task reduced to the declaration My Tasks reads: its own card_fields, which
// is where the title, the status and the due Field come from since 2026-09-28. A fixture without them
// would exercise a Machine that declares less than the one that runs, and every row would come out
// blank -- which is exactly what the screens did before the declaration existed.
func taskMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: "mch_task", Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"todo", "in_progress", "done"}},
			{ID: "fld_due_date", Name: "Due", Type: domain.FieldTypeDate},
			{ID: "fld_project", Name: "Project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"},
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: "mch_user"},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_status", Role: domain.CardFieldRoleStatus},
			{Field: "fld_due_date", Role: domain.CardFieldRoleDate},
			{Field: "fld_assignee", Role: domain.CardFieldRolePerson},
		},
		Completion: &domain.Completion{Field: "fld_status", Done: "done"},
	}
}

// projectMachineForTest is the same reduction as taskMachineForTest, for the Machine the dashboard
// composes its Project summaries from: it reads a declared display shape, not a Field id.
func projectMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: "mch_project", Name: "Project",
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"active", "archived"}},
		},
		CardFields: []domain.CardField{
			{Field: "fld_name", Role: domain.CardFieldRoleTitle},
			{Field: "fld_status", Role: domain.CardFieldRoleStatus},
		},
	}
}
