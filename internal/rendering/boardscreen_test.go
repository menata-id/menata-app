package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

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
		cards = append(cards, RecordCard{Record: r, Fields: fields})
	}
	add("rec_1", "Write the script", "lst_a", "12 Oct 2026", "Silvia Rini")
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
		"Write the script", "12 Oct 2026",
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
	if err := boardCard(m, c).Render(context.Background(), &buf); err != nil {
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
