package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

func calendarCtx() context.Context {
	return WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: resolvedNav([]domain.NavigationItem{
		{ID: "nav_calendar", Label: "Calendar", Description: "Cards sit on the day they are due.", Route: "/calendar"},
	})}, "Test Workspace", false)
}

func renderCalendar(t *testing.T, c CalendarContent) string {
	t.Helper()
	var buf bytes.Buffer
	if err := CalendarPage(c, "Acme", Viewer{Initials: "AN"}, "").Render(calendarCtx(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func calendarTask(id, title string, done bool) TaskRow {
	return TaskRow{
		Task:     &data.Record{ID: id, MachineID: "mch_task"},
		Title:    title,
		Complete: &CardComplete{Field: "fld_status", Next: "done", Done: done},
		Date:     experience.CardDate{Present: true, Label: "17 Sep", Done: done},
		Tags:     []CardTag{{Label: "Design", Color: domain.TagColor("blue")}},
		Assignee: "Ana Putri",
	}
}

// Week navigation is links to the navigation item's own route, never a retyped path, and "This week" only
// appears when the grid is somewhere else.
func TestCalendarPage_weekLinksComeFromTheRoute(t *testing.T) {
	days := []CalendarDay{{Weekday: "Mon", Date: "14 Sep"}}

	here := renderCalendar(t, CalendarContent{Days: days, Range: "14 – 20 Sep 2026"})
	for _, want := range []string{`href="/calendar?week=-1"`, `href="/calendar?week=1"`, "14 – 20 Sep 2026", `aria-label="Previous week"`, `aria-label="Next week"`} {
		if !strings.Contains(here, want) {
			t.Errorf("this week's page lacks %q", want)
		}
	}
	if strings.Contains(here, "This week") {
		t.Error("the current week offered a link back to itself")
	}

	away := renderCalendar(t, CalendarContent{Days: days, Range: "28 Sep – 04 Oct 2026", WeekOffset: 2})
	for _, want := range []string{`href="/calendar?week=1"`, `href="/calendar?week=3"`, `href="/calendar"`, "This week"} {
		if !strings.Contains(away, want) {
			t.Errorf("a later week's page lacks %q", want)
		}
	}
}

func TestCalendarPage_drawsDaysAndCards(t *testing.T) {
	html := renderCalendar(t, CalendarContent{Range: "14 – 20 Sep 2026", Days: []CalendarDay{
		{Weekday: "Mon", Date: "14 Sep"},
		{Weekday: "Tue", Date: "15 Sep", IsToday: true, Tasks: []TaskRow{
			calendarTask("tsk_open", "Day 1 clinic set", false),
			calendarTask("tsk_done", "Scout rooftop", true),
		}},
	}})

	for _, want := range []string{
		`aria-label="Mon 14 Sep"`, `aria-label="Nothing due"`, // an empty day says so to a reader, not only by a dash
		"border-blue-200",                          // today's column
		"Design", `aria-label="Ana Putri"`, ">AP<", // chip and avatar
		`hx-patch="/machines/mch_task/records/tsk_open"`, `value="done"`, // the circle writes the Task's own route
		`aria-pressed="false"`, `aria-pressed="true"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("calendar lacks %q", want)
		}
	}
	// A finished card keeps its place and turns quiet: grey title, the open one stays dark.
	if !strings.Contains(html, `text-slate-500">Scout rooftop<`) || !strings.Contains(html, `text-slate-900">Day 1 clinic set<`) {
		t.Errorf("a finished card's title is not quiet, or an open one is: %s", html)
	}
	if strings.Contains(html, "Month") {
		t.Error("the page offers a Month view the runtime has no primitive for")
	}
}
