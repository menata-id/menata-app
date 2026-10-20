package rendering

import (
	"bytes"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
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

// A create form asks for what the Machine requires and says so in the control itself, so a bare submit is stopped by the browser
// instead of answered with a raw 422 (the review of slice 5: "New project" with only a name).
func TestCreateForm_marksRequiredFieldsAndStartsAStatusAtItsDefault(t *testing.T) {
	m := &domain.Machine{ID: "mch_project", Name: "Project", Fields: []domain.Field{
		{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText, Required: true},
		{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Required: true, Options: []string{"planning", "active", "done"}, Default: "planning"},
		{ID: "fld_note", Name: "Note", Type: domain.FieldTypeText},
	}}
	var b bytes.Buffer
	if err := createForm(m, nil, nil, nil, "Create", false).Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`name="fld_name" required`, `name="fld_status" required`, `<option value="planning" selected>`} {
		if !strings.Contains(out, want) {
			t.Errorf("the create form lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `name="fld_note" required`) {
		t.Errorf("an optional Field is marked required:\n%s", out)
	}
	if strings.Contains(out, `<option value=""></option>`) && strings.Index(out, `name="fld_status"`) < strings.Index(out, `<option value=""></option>`) && strings.Index(out, `<option value=""></option>`) < strings.Index(out, `<option value="planning"`) {
		t.Errorf("a required status that has a default offers a blank choice first:\n%s", out)
	}
}

// A link may read in the page's strong ink instead of the accent's (`variant: plain`): what a list of records is read for, the way the
// mockups draw a Table's first column; it takes the accent on hover so it still says it is a link.
func TestUINode_aPlainLinkReadsInTheStrongInk(t *testing.T) {
	var b bytes.Buffer
	n := ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticLink), Props: map[string]string{"href": "/x", "text": "Task", "variant": string(domain.LinkPlain)}}
	if err := uiNode(n).Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `href="/x"`) || !strings.Contains(out, "text-ink-strong") || strings.Contains(out, "text-accent") && !strings.Contains(out, "hover:text-accent") {
		t.Errorf("a plain link: %s", out)
	}
	if strings.Contains(out, "bg-") {
		t.Errorf("a plain link is drawn as a button: %s", out)
	}
}

// A panel may be compact (`density: compact`): the small card a calendar day and a board column hold -- the tile's edge and radius, the raised fill, 10px and
// 8px of room and the meta size for its words -- instead of the 20px panel. The board's card draws the same recipe (compactCardClasses), so the two
// cannot drift apart.
func TestPanelLayout_aCompactPanelIsTheBoardsCard(t *testing.T) {
	compact := ir.UINode{Kind: ir.NodeLayout, Type: string(domain.LayoutPanel), Props: map[string]string{"density": string(domain.PanelCompact)},
		Children: []ir.UINode{{Kind: ir.NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": "Day 1"}}}}
	regular := compact
	regular.Props = nil
	var c, r bytes.Buffer
	if err := uiNode(compact).Render(assignedNavFixture(), &c); err != nil {
		t.Fatal(err)
	}
	if err := uiNode(regular).Render(assignedNavFixture(), &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.String(), "px-2.5 py-2") || strings.Contains(c.String(), "p-5") {
		t.Errorf("a compact panel: %s", c.String())
	}
	if !strings.Contains(r.String(), "p-5") || strings.Contains(r.String(), "px-2.5 py-2") {
		t.Errorf("a regular panel moved: %s", r.String())
	}
	if !strings.Contains(compactCardClasses(assignedNavFixture()), "px-2.5 py-2") {
		t.Error("the shared card recipe lost its room")
	}
	if issues := ir.Validate(compact); len(issues) != 0 {
		t.Errorf("density: compact is a declared property of a panel: %v", issues)
	}
	bad := compact
	bad.Props = map[string]string{"density": "huge"}
	if issues := ir.Validate(bad); len(issues) == 0 {
		t.Error("density: huge was accepted")
	}
}

// A row may keep its children on one line (`wrap: nowrap`): a circle beside a title that wraps within its own column instead of the title dropping
// under the circle -- a calendar card, as the mockup draws it. A row wraps unless it says so.
func TestRowLayout_noWrapKeepsTheCircleBesideItsTitle(t *testing.T) {
	row := ir.UINode{Kind: ir.NodeLayout, Type: string(domain.LayoutRow), Props: map[string]string{"wrap": string(domain.RowNoWrap)},
		Children: []ir.UINode{{Kind: ir.NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": "x"}}}}
	plain := row
	plain.Props = nil
	var a, b bytes.Buffer
	if err := uiNode(row).Render(assignedNavFixture(), &a); err != nil {
		t.Fatal(err)
	}
	if err := uiNode(plain).Render(assignedNavFixture(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.String(), "flex-nowrap") || strings.Contains(a.String(), "flex-wrap") {
		t.Errorf("a no-wrap row: %s", a.String())
	}
	if !strings.Contains(b.String(), "flex-wrap") || strings.Contains(b.String(), "flex-nowrap") {
		t.Errorf("a row that says nothing stopped wrapping: %s", b.String())
	}
	if issues := ir.Validate(row); len(issues) != 0 {
		t.Errorf("wrap: nowrap is a declared property of a row: %v", issues)
	}
	row.Props = map[string]string{"wrap": "sideways"}
	if issues := ir.Validate(row); len(issues) == 0 {
		t.Error("wrap: sideways was accepted")
	}
}
