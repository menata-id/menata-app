package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

func boardMachine() (*domain.Machine, domain.View) {
	v := domain.View{ID: "vw_task_board", Name: "Board", Type: domain.ViewBoard, GroupBy: "fld_list"}
	m := &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson},
			{ID: "fld_due_date", Name: "Due Date", Type: domain.FieldTypeDate},
			{ID: "fld_list", Name: "List", Type: domain.FieldTypeRelation, RelatedMachine: "mch_list"},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_due_date", Role: domain.CardFieldRoleDate},
			{Field: "fld_assignee", Role: domain.CardFieldRolePerson},
		},
		Views: []domain.View{v, {ID: "vw_task_table", Name: "Table", Type: domain.ViewTable}},
	}
	return m, v
}

func boardFixture() (records []*data.Record, cards []RecordCard, columns []experience.Column) {
	columns = []experience.Column{{ID: "lst_a", Label: "Backlog"}, {ID: "lst_b", Label: "Shooting"}}
	add := func(id, title, list, due, person string) {
		r := &data.Record{ID: id, Values: map[string]any{"fld_title": title, "fld_list": list}}
		records = append(records, r)
		fields := []ProjectedField{{Label: "Title", Role: "title", Display: title}}
		if due != "" {
			fields = append(fields, ProjectedField{Label: "Due Date", Role: "date", Display: due})
		}
		if person != "" {
			fields = append(fields, ProjectedField{Label: "Assignee", Role: "person", Display: person})
		}
		card := RecordCard{Record: r, Fields: fields}
		if due != "" {
			card.Date = experience.CardDate{Label: due, Tone: domain.ToneNeutral, Present: true}
		}
		cards = append(cards, card)
	}
	add("rec_1", "Write the script", "lst_a", "12 Oct", "Silvia Rini")
	add("rec_2", "Book the studio", "lst_b", "", "")
	add("rec_3", "Orphan card", "", "", "")
	return
}

func renderBoard(t *testing.T) string {
	t.Helper()
	m, v := boardMachine()
	records, cards, columns := boardFixture()
	var buf bytes.Buffer
	if err := MachineBody(m, v, records, nil, nil, columns, cards, domain.Actor{ID: "usr_ana"}, time.Now()).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// A board is a screen of its own: a heading and summary computed from the records and the View's group_by,
// one column per group with a count, a card per record, and none of the generic Machine page's Fields
// table or "Add Record" row.
func TestBoardScreen_rendersColumnsCardsAndSummary(t *testing.T) {
	got := renderBoard(t)
	for _, want := range []string{
		"3 cards · grouped by List",
		`aria-label="Backlog"`, `aria-label="Shooting"`, `aria-label="Other"`,
		`href="/machines/mch_task/records/rec_1"`,
		"Write the script", "12 Oct",
		`aria-label="Silvia Rini"`, ">SR<",
		"New task", "Add a card",
		`name="fld_list" value="lst_a"`,
		`/machines/mch_task?view=vw_task_table`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("board is missing %q", want)
		}
	}
	for _, absent := range []string{">Fields<", "Add Record", "mch_task</code>", "&lt;nil&gt;"} {
		if strings.Contains(got, absent) {
			t.Errorf("board still renders %q, which belongs to the generic Machine page", absent)
		}
	}
}

// A card with no date and no person draws no footer at all, rather than an empty row under the title.
func TestBoardScreen_cardWithoutDateOrPersonHasNoFooter(t *testing.T) {
	var buf bytes.Buffer
	c := RecordCard{Record: &data.Record{ID: "rec_9"}, Fields: []ProjectedField{{Label: "Title", Role: "title", Display: "Bare"}}}
	m, _ := boardMachine()
	if err := boardCard(m, c, domain.Actor{}, CardMove{}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "justify-between") {
		t.Errorf("a card with neither date nor person rendered a footer:\n%s", buf.String())
	}
}

// The heading is what the navigation item routing to the Machine calls itself, and the Machine's own name
// only when none does.
func TestMachineHeading_prefersTheNavigationItemRoutingToIt(t *testing.T) {
	m, _ := boardMachine()
	if got := machineHeading(context.Background(), m); got != "Task" {
		t.Errorf("no navigation declared: heading = %q, want the Machine's own name", got)
	}
	ws := domain.Workspace{Navigation: []domain.NavigationItem{{ID: "nav_board", Label: "Board", Heading: "Board", Route: "/machines/mch_task"}}}
	ctx := WithCurrentWorkspace(context.Background(), ws, "Acme", false)
	if got := machineHeading(ctx, m); got != "Board" {
		t.Errorf("heading = %q, want the declared navigation item's", got)
	}
}

// The date pill has three looks and the card does not choose among them: composition resolves the tone and
// whether the record is finished, the card draws what it is handed. Overdue is red, finished is green with
// a check in place of the calendar.
func TestBoardScreen_datePillStates(t *testing.T) {
	m, _ := boardMachine()
	render := func(d experience.CardDate) string {
		var buf bytes.Buffer
		c := RecordCard{Record: &data.Record{ID: "rec_9"}, Date: d, Fields: []ProjectedField{{Label: "Title", Role: "title", Display: "T"}}}
		if err := boardCard(m, c, domain.Actor{}, CardMove{}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	normal := render(experience.CardDate{Label: "12 Oct", Tone: domain.ToneNeutral, Present: true})
	overdue := render(experience.CardDate{Label: "1 Oct", Tone: domain.ToneBad, Present: true})
	done := render(experience.CardDate{Label: "1 Oct", Tone: domain.ToneGood, Done: true, Present: true})

	if !strings.Contains(normal, "bg-slate-100") || strings.Contains(normal, "bg-red-50") {
		t.Errorf("a normal date should be the grey pill:\n%s", normal)
	}
	if !strings.Contains(overdue, "bg-red-50 text-red-700") {
		t.Errorf("an overdue date should be the red pill:\n%s", overdue)
	}
	if !strings.Contains(done, "bg-emerald-50 text-emerald-700") || strings.Contains(done, "bg-red-50") {
		t.Errorf("a finished card's date should be the green pill, never red:\n%s", done)
	}
	if strings.Count(done, "<svg") != strings.Count(normal, "<svg") || done == normal {
		t.Errorf("a finished card should swap the calendar icon for a check")
	}
	if got := strings.Count(done, "M5 12.5l4.5 4.5L19 7.5"); got != 1 {
		t.Errorf("finished card should draw the check icon once, drew it %d times", got)
	}
}

func TestBoardScreen_tagChips(t *testing.T) {
	m, _ := boardMachine()
	render := func(tags []CardTag) string {
		var buf bytes.Buffer
		c := RecordCard{Record: &data.Record{ID: "rec_9"}, Tags: tags, Fields: []ProjectedField{{Label: "Title", Role: "title", Display: "T"}}}
		if err := boardCard(m, c, domain.Actor{}, CardMove{}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if out := render(nil); strings.Contains(out, "rounded-full border") {
		t.Errorf("an untagged card must draw no chip row:\n%s", out)
	}
	out := render([]CardTag{{Label: "Bug", Color: domain.TagRose}, {Label: "Legacy", Color: "chartreuse"}})
	for _, want := range []string{"Bug", "bg-rose-700/10 text-rose-700", "Legacy", "bg-slate-600/10 text-slate-600"} {
		if !strings.Contains(out, want) {
			t.Errorf("chips missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "Bug") > strings.Index(out, ">T<") {
		t.Errorf("chips belong above the title")
	}
	for _, c := range domain.KnownTagColors {
		if chip, dot := tagClasses(c); strings.Contains(chip, "slate") && c != domain.TagSlate || dot == "" {
			t.Errorf("palette entry %q has no distinct classes (%q, %q)", c, chip, dot)
		}
	}
}

// The completion circle writes the opposite of what the record holds, to the Field the Machine declared, and
// only for someone who may edit; a finished card keeps its circle visible and greys its title.
func TestBoardScreen_completionCircleAndQuickEdit(t *testing.T) {
	m, _ := boardMachine()
	render := func(done bool, actor domain.Actor) string {
		var buf bytes.Buffer
		next := "done"
		if done {
			next = "todo"
		}
		c := RecordCard{
			Record:   &data.Record{ID: "rec_9"},
			Complete: &CardComplete{Field: "fld_status", Next: next, Done: done},
			Fields:   []ProjectedField{{Label: "Title", Role: "title", Display: "Ship it"}},
		}
		if err := boardCard(m, c, actor, CardMove{}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	open := render(false, domain.Actor{ID: "usr_ana"})
	for _, want := range []string{
		`hx-patch="/machines/mch_task/records/rec_9"`,
		`<input type="hidden" name="fld_status" value="done">`,
		`aria-pressed="false"`, "Mark \u201cShip it\u201d complete",
		`name="fld_title" value="Ship it"`,
	} {
		if !strings.Contains(open, want) {
			t.Errorf("open card is missing %q:\n%s", want, open)
		}
	}
	if strings.Contains(open, "text-slate-500 after") || strings.Contains(open, "text-slate-500 block") {
		t.Errorf("an open card must not grey its title")
	}
	done := render(true, domain.Actor{ID: "usr_ana"})
	for _, want := range []string{`<input type="hidden" name="fld_status" value="todo">`, `aria-pressed="true"`, "not complete", "opacity-100", "bg-emerald-600"} {
		if !strings.Contains(done, want) {
			t.Errorf("finished card is missing %q:\n%s", want, done)
		}
	}
	if !strings.Contains(done, "text-slate-500") {
		t.Errorf("a finished card greys its title")
	}
}

// Moving a card has two doors, one contract: the Move panel and the drag script both write the View's group
// Field and a `position`. The board declares where each column's value lives (data-*), the panel offers every
// real list but never the synthetic "Other", and a card is draggable only for someone who may edit it.
func TestBoardScreen_moveOffersEveryRealListAndNeverOther(t *testing.T) {
	got := renderBoard(t)
	for _, want := range []string{
		`data-board-column`, `data-field="fld_list"`, `data-value="lst_a"`, `data-cards`,
		`data-card`, `data-url="/machines/mch_task/records/rec_1"`, `draggable="true"`,
		`Move to list`, `name="position"`, `<option value="lst_a" selected>Backlog</option>`,
		`htmx.ajax("PATCH"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("board is missing %q", want)
		}
	}
	if strings.Contains(got, `<option value="">Other`) || strings.Contains(got, `<option value="Other"`) {
		t.Errorf("the synthetic Other column must not be a move target")
	}
	var buf bytes.Buffer
	m, _ := boardMachine()
	c := RecordCard{Record: &data.Record{ID: "rec_9"}, Fields: []ProjectedField{{Label: "Title", Role: "title", Display: "T"}}}
	if err := boardCard(m, c, domain.Actor{}, CardMove{Field: "fld_list", Targets: []MoveTarget{{Value: "a", Label: "A"}}}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Move to list") != authorizationAllowsEdit(m, c, domain.Actor{}) {
		t.Errorf("the Move panel must follow the edit permission:\n%s", buf.String())
	}
}

func authorizationAllowsEdit(m *domain.Machine, c RecordCard, a domain.Actor) bool {
	return authorization.AllowsAction(m, domain.ActionEdit, c.Record.Values, a)
}
