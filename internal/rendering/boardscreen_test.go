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
		Name: "Task", Plural: "tasks",
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
	if err := MachineBody(m, v, records, nil, nil, columns, cards, domain.Actor{ID: "usr_ana"}, time.Now(), nil).Render(context.Background(), &buf); err != nil {
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
		"3 tasks · grouped by List",
		"Tasks with no list land here.",
		`aria-label="Backlog"`, `aria-label="Shooting"`, `aria-label="Other"`,
		`href="/machines/mch_task/records/rec_1"`,
		"Write the script", "12 Oct",
		`aria-label="Silvia Rini"`, ">SR<",
		"New task", "Add task",
		`name="fld_list" value="lst_a"`,
		`href="?view=vw_task_table"`,
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
		return resolveColourRoles(t, context.Background(), buf.String())
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
		// A Tag's hue is a property since dark mode (DK8); resolved to what it draws by default.
		return resolveColourRoles(t, context.Background(), buf.String())
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
		return resolveColourRoles(t, context.Background(), buf.String())
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
	// A touch screen has no hover to reveal the circle and the pencil (S0.2, mockup MP01): both carry the
	// coarse-pointer variant, and the title makes room for them the same way hover does.
	if got := strings.Count(open, "pointer-coarse:opacity-100"); got != 2 {
		t.Errorf("pointer-coarse:opacity-100 appears %d times, want 2 -- the completion circle and the pencil:\n%s", got, open)
	}
	if !strings.Contains(open, "pointer-coarse:pl-7") || !strings.Contains(open, "pointer-coarse:pr-6") {
		t.Errorf("on a coarse pointer the title must leave room for both controls:\n%s", open)
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

// The summary counts finished cards only for a Machine that declares a completion, and counts what the
// cards say is done rather than recomputing it.
func TestBoardSummary_countsCompleteOnlyWhereACompletionIsDeclared(t *testing.T) {
	m, v := boardMachine()
	cards := []RecordCard{{Complete: &CardComplete{Done: true}}, {Complete: &CardComplete{}}, {Complete: &CardComplete{Done: true}}}
	m.Completion = nil
	if got := boardSummary(m, v, 3, cards); strings.Contains(got, "complete") {
		t.Errorf("summary = %q, want no completion clause for a Machine that declares none", got)
	}
	m.Completion = &domain.Completion{Field: "fld_status", Done: "done"}
	if got := boardSummary(m, v, 3, cards); !strings.HasSuffix(got, "grouped by List · 2 complete") {
		t.Errorf("summary = %q, want it to end \"grouped by List · 2 complete\"", got)
	}
}

// A board whose columns are records of another Machine offers each its menu (rename, move either way, delete) only where that Machine's
// Permissions let the viewer, through that Machine's own routes; the first column cannot move earlier and the last cannot move later; the
// lane for tasks with no list has no menu and no composer and is dashed; and the board ends in a tile that names where lists are made.
func TestBoardScreen_columnMenuAddTileAndTheLeftoverLane(t *testing.T) {
	m, v := boardMachine()
	v.ColumnActions = []domain.ColumnAction{domain.ColumnRename, domain.ColumnMove, domain.ColumnDelete}
	v.AddColumn = &domain.AddColumn{To: "nav_lists", Label: "Add list", Note: "Lists are shared."}
	list := &domain.Machine{ID: "mch_list", Name: "List", Fields: []domain.Field{{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText}},
		CardFields: []domain.CardField{{Field: "fld_name", Role: domain.CardFieldRoleTitle}}}
	records, cards, columns := boardFixture()
	columns[0].Machine, columns[0].Values = list, map[string]any{}
	columns[1].Machine, columns[1].Values = list, map[string]any{}
	app := domain.Application{ID: "app_x", AllNavigation: []domain.NavigationItem{{ID: "nav_lists", Route: "/machines/mch_list"}}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Applications: []domain.Application{app}}, "WS", false)
	render := func(actor domain.Actor) string {
		var buf bytes.Buffer
		if err := MachineBody(m, v, records, nil, nil, columns, cards, actor, time.Now(), nil).Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	got := render(domain.Actor{ID: "usr_ana"})
	for _, want := range []string{
		`aria-label="List actions for Backlog"`, `aria-label="List actions for Shooting"`,
		`hx-delete="/machines/mch_list/records/lst_a"`, `hx-confirm="Delete this list? Its 1 task moves to Other. This cannot be undone."`,
		`hx-post="/machines/mch_list/records/lst_a/move?direction=down"`, `hx-post="/machines/mch_list/records/lst_b/move?direction=up"`,
		`hx-patch="/machines/mch_list/records/lst_a"`,
		`hx-headers="{&#34;` + domain.RefreshAnswerHeader + `&#34;:&#34;true&#34;}"`, `pattern=".*\S.*"`,
		`href="/machines/mch_list"`, "Add list", "Lists are shared.",
		"border-dashed", "Tasks with no list land here.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("board is missing %q", want)
		}
	}
	for _, absent := range []string{"List actions for Other", "/records/lst_a/move?direction=up", "/records/lst_b/move?direction=down"} {
		if strings.Contains(got, absent) {
			t.Errorf("board draws %q", absent)
		}
	}
	// Add task: one composer per real list, none under the leftover lane.
	if n := strings.Count(got, "Add task"); n != 2 {
		t.Errorf("%d composers, want one under each of the two lists and none under the leftover lane", n)
	}

	// Permissions of the List Machine apply: a viewer who may not edit or delete it sees no menu.
	list.Permissions = []domain.Permission{{Action: domain.ActionEdit, ActorField: "fld_owner"}, {Action: domain.ActionDelete, ActorField: "fld_owner"}}
	if out := render(domain.Actor{ID: "usr_ana", WorkspaceRole: "member"}); strings.Contains(out, "List actions for") {
		t.Errorf("a viewer who may neither edit nor delete a list is offered its menu")
	}
}

// A board says "3 tasks" from the plural its Machine declares, and a Machine that declares none is never given a made-up one:
// "3 categorys" and "statuss" were what adding an s did to the names of boards that had said "cards" before. The word the composer
// asks for is never empty either.
func TestBoardScreen_pluralIsDeclaredNeverGuessed(t *testing.T) {
	for _, c := range []struct{ name, plural, wantSummary, wantAdd string }{
		{"Task", "tasks", "3 tasks · grouped by List", "Add task"},
		{"Category", "categories", "3 categories · grouped by List", "Add category"},
		{"Category", "", "3 items · grouped by List", "Add category"},
		{"Status", "", "3 items · grouped by List", "Add status"},
		{"", "", "3 items · grouped by List", "Add item"},
	} {
		m, v := boardMachine()
		m.Name, m.Plural = c.name, c.plural
		records, cards, columns := boardFixture()
		var buf bytes.Buffer
		if err := MachineBody(m, v, records, nil, nil, columns, cards, domain.Actor{ID: "usr_ana"}, time.Now(), nil).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		got := buf.String()
		for _, want := range []string{c.wantSummary, c.wantAdd} {
			if !strings.Contains(got, want) {
				t.Errorf("%q/%q: board is missing %q", c.name, c.plural, want)
			}
		}
		if strings.Contains(got, "categorys") || strings.Contains(got, "statuss") || strings.Contains(got, "Add </") || strings.Contains(got, "Add \n") {
			t.Errorf("%q/%q: the board guessed a plural or left the noun empty", c.name, c.plural)
		}
	}
}

// The question before deleting a lane says what becomes of what is in it, from the lane's count and the board's own relation: they move to the
// leftover lane, or -- where the relation refuses while it is in use -- the delete is only possible once it is empty.
func TestDeleteQuestion_saysWhatBecomesOfWhatIsInTheLane(t *testing.T) {
	m, v := boardMachine()
	list := &domain.Machine{ID: "mch_list", Name: "List"}
	records, _, _ := boardFixture()
	lane := func(n int) experience.Column { return experience.Column{Machine: list, Records: records[:n]} }
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "Delete this list? This cannot be undone."},
		{1, "Delete this list? Its 1 task moves to Other. This cannot be undone."},
		{2, "Delete this list? Its 2 tasks move to Other. This cannot be undone."},
	} {
		if got := deleteQuestion(m, v, lane(c.n)); got != c.want {
			t.Errorf("%d held: %q, want %q", c.n, got, c.want)
		}
	}
	for i := range m.Fields {
		if m.Fields[i].ID == v.GroupBy {
			m.Fields[i].OnParentDelete = domain.ParentDeleteRestrict
		}
	}
	want := "Delete this list? It can only be deleted while no tasks are in it. This cannot be undone."
	if got := deleteQuestion(m, v, lane(2)); got != want {
		t.Errorf("restricted: %q, want %q", got, want)
	}
}

// The keyboard move reports in a live region that is on the page before anything is lifted (one that appears with its text is often not read),
// a card in the leftover lane can be lifted out of it (but not dropped back into it as if it were a list), and focus returns to the moved card
// after the board is swapped. The script is not run here; the assertions name the pieces whose absence the review found.
func TestBoardScreen_keyboardMoveAnnouncesLiftsFromOtherAndRefocuses(t *testing.T) {
	got := renderBoard(t)
	for _, want := range []string{
		`data-board-live role="status" aria-live="polite" class="sr-only"`,
		`data-board-status class="hidden" aria-hidden="true"`,
		`if (!lifted.col.dataset.value) { cancel(true); return; }`,
		`htmx:afterSettle`,
		`e.key === "ArrowLeft" && at === -1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("board is missing %q", want)
		}
	}
	if strings.Contains(got, `if (!col || !col.dataset.value) return;
					e.preventDefault();
					var cards`) {
		t.Error("a card in the leftover lane still cannot be lifted")
	}
}

// The menu follows what the page's reads resolved the way the routes do (experience.Column.Can), not only the Machine's own Permissions: a lane
// the routes would refuse draws no menu, and one they would allow does, whatever a Permission that reads a parent would have said here.
func TestBoardScreen_columnMenuFollowsWhatTheRoutesResolved(t *testing.T) {
	m, v := boardMachine()
	v.ColumnActions = []domain.ColumnAction{domain.ColumnRename, domain.ColumnMove, domain.ColumnDelete}
	list := &domain.Machine{ID: "mch_list", Name: "List", Fields: []domain.Field{{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText}}}
	records, cards, columns := boardFixture()
	for i := 0; i < 2; i++ {
		columns[i].Machine, columns[i].Values = list, map[string]any{}
	}
	columns[0].Can = map[string]bool{domain.ActionEdit: false, domain.ActionDelete: false}
	columns[1].Can = map[string]bool{domain.ActionEdit: true, domain.ActionDelete: false}
	var buf bytes.Buffer
	if err := MachineBody(m, v, records, nil, nil, columns, cards, domain.Actor{ID: "usr_ana"}, time.Now(), nil).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, `aria-label="List actions for Backlog"`) {
		t.Error("a lane the routes refuse everything on drew a menu")
	}
	if !strings.Contains(got, `aria-label="List actions for Shooting"`) || strings.Contains(got, `hx-delete="/machines/mch_list/records/lst_b"`) {
		t.Error("a lane the routes let the viewer rename but not delete must draw a menu without Delete")
	}
}

// Dragging with the mouse draws what the mockup's PM01b shows: the lifted card tilts and casts a shadow, the place it came from collapses to
// a "moving…" gap, the list it is over shows its count going up ("1 → 2"), and the board's status line says where it will land. Every
// column's count is a marked node so the script can say it; the keyboard's status line and this one are the same.
func TestBoardScreen_aMouseDragIsDrawnLikeTheMockupsLiftedCard(t *testing.T) {
	got := renderBoard(t)
	if !strings.Contains(got, `data-board-count`) {
		t.Errorf("a column's count is not marked for the drag script:\n%s", got)
	}
	for _, want := range []string{"moving\\u2026", "setDragImage", "rotate-2", "shadow-lg", "Release to drop", "\\u2192"} {
		if !strings.Contains(got, want) {
			t.Errorf("the board's drag script lacks %q", want)
		}
	}
}

// A column's composer is "a smaller form, not a second way to write" -- but a smaller form must still be one that can be
// submitted. The log of 2026-10-22 (session aise_de84ee9b): a Card board grouped by Status, with a required List and no
// default, answered every "Add card" with `field "fld_card_list" is required`, because the composer asked only for a
// title and the group. It now also asks for each Field the Machine requires that the composer cannot fill itself.
func TestBoardScreen_composerAsksForWhatTheMachineRequiresAndItCannotFill(t *testing.T) {
	v := domain.View{ID: "vw_card_board", Name: "Board", Type: domain.ViewBoard, GroupBy: "fld_status"}
	m := &domain.Machine{
		ID: "mch_card", Name: "Card", Plural: "cards",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_list", Name: "List", Type: domain.FieldTypeRelation, RelatedMachine: "mch_list", Required: true},
			{ID: "fld_kind", Name: "Kind", Type: domain.FieldTypeStatus, Options: []string{"a", "b"}, Required: true, Default: "a"},
			{ID: "fld_note", Name: "Note", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"To Do", "Done"}, Required: true},
		},
		CardFields: []domain.CardField{{Field: "fld_title", Role: domain.CardFieldRoleTitle}},
		Views:      []domain.View{v},
	}
	columns := []experience.Column{{ID: "To Do", Label: "To Do"}, {ID: "Done", Label: "Done"}}
	relations := RelationOptions{"mch_list": {{ID: "lst_a", Label: "Backlog"}}}
	var buf bytes.Buffer
	if err := MachineBody(m, v, nil, relations, nil, columns, nil, domain.Actor{ID: "usr_ana"}, time.Now(), nil).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if n := strings.Count(got, `<select name="fld_list"`); n != 3 {
		t.Errorf("%d List selects, want one in each of the two composers (plus the header form's): there should be 3", n)
	}
	if !strings.Contains(got, `Backlog`) {
		t.Errorf("the List select offers no list")
	}
	// The header's own "New card" form asks for everything; only the composers (their <details>) are held to the narrower rule.
	var composers string
	for _, part := range strings.Split(got, `<details name="board-edit"`)[1:] {
		composers += part[:strings.Index(part, "</details>")]
	}
	for _, absent := range []string{`name="fld_note"`, `<select name="fld_kind"`, `<select name="fld_status"`} {
		if strings.Contains(composers, absent) {
			t.Errorf("a composer asks for %s: it is optional, has a default, or is the group the column already sets", absent)
		}
	}
}
