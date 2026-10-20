package rendering

import (
	"bytes"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
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

// A scope that declares `new:` offers to make one more of what it switches between: the "New project" form is in the menu, below the choices, and posts to the
// scope Machine's own create route. A Workspace with no project yet has nothing to switch between and is offered the form alone, as the pill itself.
func TestViewHeader_theScopeMenuOffersToCreateOne(t *testing.T) {
	m := &domain.Machine{ID: "mch_project", Name: "Project", Fields: []domain.Field{{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText, Required: true}}}
	create := &ScopeCreate{Label: "New project", Machine: m}
	with := renderHeader(t, ViewHeader{Title: "Tasks", Tabs: []SegmentTab{{Label: "Board", Current: true}, {Label: "Table"}},
		Scope: &ViewScopeMenu{Label: "Switch project", Items: []ViewScopeItem{{Label: "All projects", Href: "/x", Current: true}, {Label: "Season 1", Href: "/x?project=p1"}}, Create: create}}, "")
	for _, want := range []string{"Switch project", ">All projects</a>", "New project", `hx-post="/machines/mch_project/records"`, `name="fld_name"`} {
		if !strings.Contains(with, want) {
			t.Errorf("the menu lacks %q:\n%s", want, with)
		}
	}
	if strings.Index(with, ">Season 1</a>") > strings.Index(with, "New project") {
		t.Errorf("the form belongs below the choices:\n%s", with)
	}
	alone := renderHeader(t, ViewHeader{Title: "Tasks", Tabs: []SegmentTab{{Label: "Board", Current: true}, {Label: "Table"}}, Scope: &ViewScopeMenu{Label: "Switch project", Create: create}}, "")
	if !strings.Contains(alone, "New project") || strings.Contains(alone, "Switch project") || strings.Contains(alone, "All projects") {
		t.Errorf("with no project the pill is the form alone:\n%s", alone)
	}
	none := renderHeader(t, ViewHeader{Title: "Tasks", Tabs: []SegmentTab{{Label: "Board", Current: true}, {Label: "Table"}}, Scope: &ViewScopeMenu{Label: "Switch project", Items: []ViewScopeItem{{Label: "All projects", Href: "/x", Current: true}}}}, "")
	if strings.Contains(none, "New project") || strings.Contains(none, "hx-post") {
		t.Errorf("a scope that declares no `new:` offers no form:\n%s", none)
	}
}
