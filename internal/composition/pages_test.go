package composition

import (
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
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

var dashboardNow = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func dashboardFrom(tasks, projects []*data.Record, people map[string]string) rendering.DashboardContent {
	return buildDashboard(tasks, projects, people, taskMachineForTest(), projectMachineForTest(), dashboardNow)
}

func tile(t *testing.T, d rendering.DashboardContent, label string) rendering.SummaryItem {
	t.Helper()
	for _, it := range d.Tiles {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("no %q tile in %+v", label, d.Tiles)
	return rendering.SummaryItem{}
}

// A Project's rollup counts its own Tasks only, and "open" means not finished by the Machine's declared
// completion -- not "status is not done" restated here, which is what the old Dataset did.
func TestBuildDashboard_ProjectRollups(t *testing.T) {
	projects := []*data.Record{
		rec("prj_1", map[string]any{"fld_name": "Apollo", "fld_status": "active"}),
		rec("prj_2", map[string]any{"fld_name": "Borneo"}),
	}
	tasks := []*data.Record{
		task("tsk_1", "prj_1", "usr_ana", "todo", ""),
		task("tsk_2", "prj_1", "usr_ana", "in_progress", ""),
		task("tsk_3", "prj_1", "usr_ana", "done", ""),
		task("tsk_4", "prj_2", "usr_ana", "todo", ""),
	}

	got := dashboardFrom(tasks, projects, nil)
	if len(got.Projects) != 2 {
		t.Fatalf("want a summary per Project, got %d", len(got.Projects))
	}
	if p := got.Projects[0]; p.Name != "Apollo" || p.Status != "active" || p.OpenTasks != 2 || p.TotalTasks != 3 {
		t.Errorf("Apollo = %+v, want Apollo/active open 2 total 3", p)
	}
	if p := got.Projects[1]; p.Name != "Borneo" || p.OpenTasks != 1 || p.TotalTasks != 1 {
		t.Errorf("Borneo = %+v, want open 1 total 1", p)
	}
}

// A Task belonging to no Project must not be attributed to one, and must not crash the rollup -- but it is
// still a card, so the headline tiles count it.
func TestBuildDashboard_OrphanTaskCountsAgainstNoProject(t *testing.T) {
	projects := []*data.Record{rec("prj_1", map[string]any{"fld_name": "Apollo"})}
	tasks := []*data.Record{task("tsk_orphan", "", "usr_ana", "todo", "")}

	got := dashboardFrom(tasks, projects, nil)
	if got.Projects[0].TotalTasks != 0 {
		t.Errorf("orphan Task must not be counted against Apollo, got total %d", got.Projects[0].TotalTasks)
	}
	if v := tile(t, got, "All cards").Value; v != "1" {
		t.Errorf("All cards = %s, want the orphan counted", v)
	}
}

// The four tiles are one partition of the same selection: Open + Completed = All cards, and Overdue is a
// subset of Open. A finished card is never overdue, whatever its date says.
func TestBuildDashboard_TilesPartitionOneSelection(t *testing.T) {
	tasks := []*data.Record{
		task("tsk_done_late", "prj_1", "usr_ana", "done", "2026-09-01"),
		task("tsk_overdue", "prj_1", "usr_ana", "todo", "2026-09-09"),
		task("tsk_today", "prj_2", "usr_ana", "in_progress", "2026-09-10"),
		task("tsk_undated", "prj_2", "usr_ana", "todo", ""),
	}
	got := dashboardFrom(tasks, nil, nil)

	for label, want := range map[string]string{"All cards": "4", "Open": "3", "Overdue": "1", "Completed": "1"} {
		if v := tile(t, got, label).Value; v != want {
			t.Errorf("%s = %s, want %s", label, v, want)
		}
	}
	if h := tile(t, got, "All cards").Hint; h != "In 2 projects" {
		t.Errorf("All cards hint = %q", h)
	}
	if h := tile(t, got, "Completed").Hint; h != "25% of all cards" {
		t.Errorf("Completed hint = %q", h)
	}
	if tone := tile(t, got, "Overdue").Tone; tone != domain.ToneBad {
		t.Errorf("a non-zero Overdue is red, got %q", tone)
	}
}

// A zero is not a signal: no overdue cards must not paint the tile red, and an empty Workspace must not
// divide by zero.
func TestBuildDashboard_EmptyWorkspaceIsQuietNotRed(t *testing.T) {
	got := dashboardFrom(nil, nil, nil)
	if tone := tile(t, got, "Overdue").Tone; tone == domain.ToneBad {
		t.Errorf("zero overdue drawn red")
	}
	if tone := tile(t, got, "Completed").Tone; tone == domain.ToneGood {
		t.Errorf("zero completed drawn green")
	}
	if h := tile(t, got, "Completed").Hint; h != "0% of all cards" {
		t.Errorf("hint = %q", h)
	}
}

// Open cards per person: most open first, ties by name, unassigned cards belong to nobody, and an assignee
// with no resolvable name is left off rather than shown as an id.
func TestBuildDashboard_OpenCardsPerPerson(t *testing.T) {
	people := map[string]string{"usr_ana": "Ana", "usr_budi": "Budi", "usr_cici": "Cici"}
	tasks := []*data.Record{
		task("t1", "prj_1", "usr_budi", "todo", ""),
		task("t2", "prj_1", "usr_budi", "done", ""),
		task("t3", "prj_1", "usr_ana", "todo", ""),
		task("t4", "prj_1", "usr_ana", "todo", ""),
		task("t5", "prj_1", "usr_cici", "done", ""),
		task("t6", "prj_1", "", "todo", ""),
		task("t7", "prj_1", "usr_gone", "todo", ""),
	}
	got := dashboardFrom(tasks, nil, people).People

	want := []rendering.PersonLoad{{Name: "Ana", Open: 2, Total: 2}, {Name: "Budi", Open: 1, Total: 2}, {Name: "Cici", Open: 0, Total: 1}}
	if len(got) != len(want) {
		t.Fatalf("people = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("people[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// My Tasks buckets by day and by completion: a finished Task is Completed whatever its date says, a passed
// date is Overdue, a date from today through seven days ahead is Next 7 days, and anything later or
// undated is Later (an unparseable date is "someday", not dropped).
//
// **It no longer receives another identity's Tasks, because it no longer filters** (2026-09-29): the
// assignee predicate moved into ds_my_tasks and runs in the database. That property did not stop
// mattering, so it did not stop being tested -- TestPersonalTasks_returnsOnlyTheViewersTasks in
// select_test.go asserts it end to end against a real database, which is where it now lives.
func TestBuildMyTasks_Buckets(t *testing.T) {
	tasks := []*data.Record{
		task("tsk_done", "prj_1", "usr_ana", "done", "2026-09-01"),
		task("tsk_overdue", "prj_1", "usr_ana", "todo", "2026-09-09"),
		task("tsk_today", "prj_1", "usr_ana", "in_progress", "2026-09-10"),
		task("tsk_edge", "prj_2", "usr_ana", "todo", "2026-09-17"),
		task("tsk_later", "prj_2", "usr_ana", "todo", "2026-09-18"),
		task("tsk_undated", "prj_2", "usr_ana", "todo", ""),
	}
	tags := map[string][]rendering.CardTag{"tsk_today": {{Label: "Set"}}}

	got := buildMyTasks(tasks, projectLabels, at(10), taskMachineForTest(), tags, nil)

	for name, c := range map[string]struct{ got, want []string }{
		"Completed": {ids(got.Completed), []string{"tsk_done"}},
		"Overdue":   {ids(got.Overdue), []string{"tsk_overdue"}},
		"Next 7":    {ids(got.Next7Days), []string{"tsk_today", "tsk_edge"}},
		"Later":     {ids(got.Later), []string{"tsk_later", "tsk_undated"}},
	} {
		if !equal(c.got, c.want) {
			t.Errorf("%s = %v, want %v", name, c.got, c.want)
		}
	}
	if got.Overdue[0].ProjectName != "Apollo" {
		t.Errorf("ProjectName = %q, want the resolved name", got.Overdue[0].ProjectName)
	}
	if c := got.Completed[0].Complete; c == nil || !c.Done || c.Next != "todo" {
		t.Errorf("a finished row must offer to reopen: %+v", c)
	}
	if c := got.Overdue[0].Complete; c == nil || c.Done || c.Next != "done" {
		t.Errorf("an open row must offer to finish: %+v", c)
	}
	if !got.Overdue[0].Date.Present || got.Overdue[0].Date.Tone != domain.ToneBad {
		t.Errorf("an overdue row's date must be drawn as overdue: %+v", got.Overdue[0].Date)
	}
	if len(got.Next7Days[0].Tags) != 1 {
		t.Errorf("tags did not reach the row: %+v", got.Next7Days[0].Tags)
	}
}

// listName reads the default View's group_by: a relation-grouped board names the list through its columns,
// an option-grouped one is the value itself, and an ungrouped Machine has none.
func TestListName(t *testing.T) {
	grouped := &domain.Machine{
		Fields: []domain.Field{{ID: "fld_list", Type: domain.FieldTypeRelation, RelatedMachine: "mch_list"}},
		Views:  []domain.View{{ID: "v", Type: domain.ViewBoard, GroupBy: "fld_list"}},
	}
	r := rec("t1", map[string]any{"fld_list": "lst_1"})
	cols := []experience.Column{{ID: "lst_1", Label: "Shooting"}}
	if got := listName(grouped, r, cols); got != "Shooting" {
		t.Errorf("relation-grouped = %q, want Shooting", got)
	}
	if got := listName(grouped, rec("t2", map[string]any{"fld_list": "gone"}), cols); got != "" {
		t.Errorf("a list that no longer exists = %q, want none", got)
	}
	byStatus := &domain.Machine{
		Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo"}}},
		Views:  []domain.View{{ID: "v", Type: domain.ViewBoard, GroupBy: "fld_status"}},
	}
	if got := listName(byStatus, rec("t3", map[string]any{"fld_status": "todo"}), nil); got != "todo" {
		t.Errorf("option-grouped = %q, want the value", got)
	}
	if got := listName(taskMachineForTest(), r, nil); got != "" {
		t.Errorf("ungrouped = %q, want none", got)
	}
}

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

func listMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: "mch_list", Name: "List",
		Fields:     []domain.Field{{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText}},
		CardFields: []domain.CardField{{Field: "fld_name", Role: domain.CardFieldRoleTitle}},
	}
}

func labelMachineForTest() *domain.Machine {
	return &domain.Machine{
		ID: "mch_label", Name: "Label",
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			{ID: "fld_color", Name: "Color", Type: domain.FieldTypeStatus, Options: []string{"blue", "rose"}},
		},
		CardFields: []domain.CardField{
			{Field: "fld_name", Role: domain.CardFieldRoleTitle},
			{Field: "fld_color", Role: domain.CardFieldRoleColor},
		},
	}
}

func TestBuildBoardSettings_ResolvesNamesColoursAndUsage(t *testing.T) {
	lists := Selection{Records: []*data.Record{
		rec("lst_1", map[string]any{"fld_name": "Backlog"}),
		rec("lst_2", map[string]any{"fld_name": "Done"}),
	}, Limit: 200}
	labels := Selection{Records: []*data.Record{
		rec("lbl_1", map[string]any{"fld_name": "Location", "fld_color": "blue"}),
		rec("lbl_2", map[string]any{"fld_name": "Legal", "fld_color": "rose"}),
		rec("lbl_3", map[string]any{"fld_name": "Old", "fld_color": "chartreuse"}),
		rec("lbl_4", map[string]any{"fld_name": "Unused", "fld_color": "blue"}),
	}, Limit: 200}
	usage := Aggregation{ByDimension: map[string]map[string]float64{
		"lbl_1": {measureTotal: 3},
		"lbl_2": {measureTotal: 1},
		"lbl_3": {measureTotal: 2},
	}}

	c := buildBoardSettings(listMachineForTest(), lists, labelMachineForTest(), labels, usage)

	if len(c.Lists) != 2 || c.Lists[0].Name != "Backlog" || c.Lists[1].Name != "Done" {
		t.Errorf("lists = %+v, want Backlog then Done in the selection's order", c.Lists)
	}
	if c.ListsHref != "/machines/mch_list" || c.LabelsHref != "/machines/mch_label" {
		t.Errorf("hrefs = %q, %q; want each Machine's own page", c.ListsHref, c.LabelsHref)
	}
	want := []rendering.SettingsLabel{
		{Name: "Location", Color: domain.TagBlue, Records: 3},
		{Name: "Legal", Color: domain.TagRose, Records: 1},
		{Name: "Old", Color: domain.TagSlate, Records: 2},
		{Name: "Unused", Color: domain.TagBlue, Records: 0},
	}
	if len(c.Labels) != len(want) {
		t.Fatalf("labels = %+v, want %d", c.Labels, len(want))
	}
	for i, w := range want {
		if c.Labels[i] != w {
			t.Errorf("label %d = %+v, want %+v (a colour outside the palette draws neutral, an unused label counts zero)", i, c.Labels[i], w)
		}
	}
}

func TestBuildBoardSettings_ReportsATruncatedSelection(t *testing.T) {
	c := buildBoardSettings(listMachineForTest(), Selection{Limit: 200, Truncated: true}, labelMachineForTest(), Selection{Limit: 200}, Aggregation{})
	if !c.ListsTruncation.Hit || c.ListsTruncation.Limit != 200 {
		t.Errorf("ListsTruncation = %+v, want the bound that bit", c.ListsTruncation)
	}
	if c.LabelsTruncation.Hit {
		t.Errorf("LabelsTruncation = %+v, but that selection stayed inside its bound", c.LabelsTruncation)
	}
}
