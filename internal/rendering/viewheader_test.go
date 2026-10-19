package rendering

import (
	"bytes"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderHeader(t *testing.T, h ViewHeader, action string) string {
	t.Helper()
	var b bytes.Buffer
	ctx := templ.WithChildren(assignedNavFixture(), templ.Raw(action))
	if err := viewHeader(h).Render(ctx, &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestViewHeader_saysWhatTheRecordsAreAndHowToChangeTheView: the title, the one-line summary, the switch between the group's screens
// with the viewer's own marked and the others links, the scope control, and the screen's own actions beside the switch.
func TestViewHeader_saysWhatTheRecordsAreAndHowToChangeTheView(t *testing.T) {
	out := renderHeader(t, ViewHeader{
		Title: "Season 1", Summary: "11 tasks · 2 complete",
		Tabs: []SegmentTab{{Label: "Board", Href: "/machines/mch_task?project=p1"}, {Label: "Table", Href: "/pages/nav_table?project=p1", Current: true}},
		Scope: &ViewScopeMenu{Label: "Switch project", Items: []ViewScopeItem{
			{Label: "All projects", Href: "/pages/nav_table"}, {Label: "Season 1", Href: "/pages/nav_table?project=p1", Current: true}}},
	}, "<button>New task</button>")
	for _, want := range []string{">Season 1</h1>", "11 tasks · 2 complete", `<a class="`, `href="/machines/mch_task?project=p1"`, `aria-current="true">Table</span>`,
		"Switch project", `href="/pages/nav_table"`, ">All projects</a>", "<button>New task</button>"} {
		if !strings.Contains(out, want) {
			t.Errorf("the header lacks %q:\n%s", want, out)
		}
	}
	if strings.Count(out, `aria-current="true"`) != 2 { // the viewer's screen and the viewer's project
		t.Errorf("want the current screen and the current project marked, once each: %s", out)
	}
}

// TestViewHeader_aGroupOfOneScreenDrawsNoSwitch and with no scope no control: nothing is drawn that has nothing to choose.
func TestViewHeader_drawsNothingThereIsNothingToChoose(t *testing.T) {
	out := renderHeader(t, ViewHeader{Title: "Tasks", Tabs: []SegmentTab{{Label: "Board", Current: true}}}, "")
	for _, not := range []string{"<nav", "Switch", "<details"} {
		if strings.Contains(out, not) {
			t.Errorf("a one-screen group with no scope draws %q: %s", not, out)
		}
	}
}

func TestMenuLink_marksTheCurrentChoice(t *testing.T) {
	var cur, other bytes.Buffer
	_ = menuLink("A", "/a", true).Render(assignedNavFixture(), &cur)
	_ = menuLink("B", "/b", false).Render(assignedNavFixture(), &other)
	if !strings.Contains(cur.String(), `aria-current="true"`) || strings.Contains(other.String(), "aria-current") {
		t.Errorf("current %s / other %s", cur.String(), other.String())
	}
}
