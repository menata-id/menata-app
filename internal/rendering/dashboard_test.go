package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderMetric(t *testing.T, hint string, tone domain.BadgeTone) string {
	t.Helper()
	var buf bytes.Buffer
	if err := metric("Overdue", "3", hint, "", tone).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// A Metric's value is drawn in its tone's ink and the hint line is optional: the three older screens pass
// neither and must not grow an empty hint element.
func TestMetric_hintAndToneAreOptional(t *testing.T) {
	plain := renderMetric(t, "", "")
	if strings.Contains(plain, "text-2xs text-slate-400") {
		t.Errorf("a Metric with no hint drew a hint line: %s", plain)
	}
	if strings.Contains(plain, "text-red-700") || strings.Contains(plain, "text-emerald-700") {
		t.Errorf("a Metric with no tone drew a signal colour: %s", plain)
	}

	red := renderMetric(t, "Past its due date", domain.ToneBad)
	if !strings.Contains(red, "text-red-700") || !strings.Contains(red, ">Past its due date<") {
		t.Errorf("a bad-tone Metric with a hint did not draw both: %s", red)
	}
	if green := renderMetric(t, "", domain.ToneGood); !strings.Contains(green, "text-emerald-700") {
		t.Errorf("a good-tone Metric did not draw green: %s", green)
	}
}

func TestDashboardPage_drawsTheMockupSections(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: resolvedNav([]domain.NavigationItem{
		{ID: "nav_dashboard", Label: "Dashboard", Description: "How much work is open.", Route: "/dashboard"},
		{ID: "nav_activity", Label: "Activity", Route: "/activity"},
	})}, "Test Workspace", false)
	c := DashboardContent{
		Tiles:    []SummaryItem{{Label: "Overdue", Value: "1", Hint: "Past its due date", Tone: domain.ToneBad}},
		Projects: []ProjectSummary{{Name: "Apollo", Status: "active", TotalTasks: 3, OpenTasks: 2}},
		People:   []PersonLoad{{Name: "Ana", Open: 2, Total: 3}},
	}
	var buf bytes.Buffer
	if err := DashboardPage(c, "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"Tasks by project", "Apollo", "Open cards per person",
		"Bar shows cards still open out of everything assigned.",
		`role="progressbar"`, `aria-valuetext="2 of 3 open"`, ">2 of 3<", "width:66%",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard is missing %q", want)
		}
	}
	if strings.Contains(html, "Showing the first") {
		t.Error("a selection inside its bound drew a truncation notice")
	}

	c.TasksTruncation = Truncation{Limit: 1000, Hit: true}
	buf.Reset()
	if err := DashboardPage(c, "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Showing the first") || !strings.Contains(buf.String(), "1000") {
		t.Error("a truncated selection did not say so, so every figure above it would read as complete")
	}
}
