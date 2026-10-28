package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func tableNode(cols ...domain.PageColumn) domain.PageNode {
	return domain.PageNode{Kind: "component", Type: "Table", Props: map[string]string{"label": "Tickets"}, Columns: cols,
		Binding: &domain.PageBinding{Dataset: "ds_x", Rows: domain.PageRowsRecords}}
}

func tableRecords(recs ...map[string]string) Resolver {
	return Resolver{Records: func(domain.PageBinding) (RecordSet, error) { return RecordSet{Records: recs}, nil }}
}

func ticket(title, status, due string) map[string]string {
	return map[string]string{"title": title, "status": status, "date": due, domain.PageRecordRole: "/machines/mch_t/records/" + title}
}

func TestLowerTableBuildsHeadersAndRowMajorCells(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentTable), string(domain.ComponentStatusBadge)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
	root := domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{tableNode(
		domain.PageColumn{Header: "Ticket", From: "title", Href: domain.PageRecordRole},
		domain.PageColumn{Header: "Status", From: "status"},
		domain.PageColumn{Header: "Due", From: "date", Align: "end"},
	)}}
	tree, err := Lower(root, tableRecords(ticket("a", "open", "1 Oct"), ticket("b", "done", "2 Oct")))
	if err != nil {
		t.Fatal(err)
	}
	table := tree.Children[0]
	if table.Type != "Table" || table.Props["headers"] != "Ticket\nStatus\nDue" || table.Props["aligns"] != "start,start,end" || table.Props["label"] != "Tickets" {
		t.Fatalf("table props = %v", table.Props)
	}
	if len(table.Children) != 6 {
		t.Fatalf("%d cells, want 3 columns x 2 records", len(table.Children))
	}
	link, badge, due := table.Children[0], table.Children[1], table.Children[2]
	if link.Type != "link" || link.Props["text"] != "a" || link.Props["href"] != "/machines/mch_t/records/a" {
		t.Errorf("the title cell = %+v, want a link to the record", link)
	}
	if badge.Kind != NodeComponent || badge.Type != "StatusBadge" || badge.Props["label"] != "open" {
		t.Errorf("the status cell = %+v, want a StatusBadge", badge)
	}
	if due.Type != "paragraph" || due.Props["text"] != "1 Oct" || table.Children[3].Props["text"] != "b" {
		t.Errorf("cells are not row-major: %+v / %+v", due, table.Children[3])
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a lowered Table is not a valid tree: %v", issues)
	}
}

func TestLowerTableRefusesEachMisuseForItsOwnReason(t *testing.T) {
	good := func() domain.PageNode {
		return tableNode(domain.PageColumn{Header: "Ticket", From: "title"})
	}
	many := make([]domain.PageColumn, MaxTableColumns+1)
	for i := range many {
		many[i] = domain.PageColumn{Header: "H", From: "title"}
	}
	for name, tc := range map[string]struct {
		edit func(n *domain.PageNode)
		want string
	}{
		"a total binding":         {func(n *domain.PageNode) { n.Binding.Rows = domain.PageRowsTotal }, "takes a binding with rows: records or dimension"},
		"a measure":               {func(n *domain.PageNode) { n.Binding.Measure = "msr" }, "no measure and no group"},
		"a group":                 {func(n *domain.PageNode) { n.Binding.Group = domain.PageGroupDate }, "no measure and no group"},
		"from on the table":       {func(n *domain.PageNode) { n.From = map[string]string{"label": "title"} }, "do not belong on a Table"},
		"written headers":         {func(n *domain.PageNode) { n.Props["headers"] = "X" }, "never written"},
		"written aligns":          {func(n *domain.PageNode) { n.Props["aligns"] = "end" }, "never written"},
		"no columns":              {func(n *domain.PageNode) { n.Columns = nil }, "declares columns"},
		"too many columns":        {func(n *domain.PageNode) { n.Columns = many }, "at most"},
		"a blank header":          {func(n *domain.PageNode) { n.Columns[0].Header = " " }, "header"},
		"a header with a newline": {func(n *domain.PageNode) { n.Columns[0].Header = "A\nB" }, "header"},
		"no role":                 {func(n *domain.PageNode) { n.Columns[0].From = "" }, "exactly one of from:"},
		"a role and a field":      {func(n *domain.PageNode) { n.Columns[0].Field = "fld_title" }, "exactly one of from:"},
		"a field nobody resolved": {func(n *domain.PageNode) { n.Columns[0] = domain.PageColumn{Header: "H", Field: "fld_party"} }, "does not project"},
		"an unknown address":      {func(n *domain.PageNode) { n.Columns[0].Href = "/typed" }, "only address"},
		"the address as text":     {func(n *domain.PageNode) { n.Columns[0].From = domain.PageRecordRole }, "href:"},
		"a bad alignment":         {func(n *domain.PageNode) { n.Columns[0].Align = "centre" }, "start or end"},
		"a role nobody projects":  {func(n *domain.PageNode) { n.Columns[0].From = "owner" }, "does not project"},
		"a write binding":         {func(n *domain.PageNode) { n.Binding.Write = domain.PageWriteCreate }, "rows: records"},
		"no binding":              {func(n *domain.PageNode) { n.Binding = nil }, "rows: records"},
	} {
		n := good()
		tc.edit(&n)
		_, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}, tableRecords(ticket("a", "open", "1 Oct")))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
	// columns: belongs to a Table and nothing else.
	other := domain.PageNode{Kind: "component", Type: "Collection", Columns: []domain.PageColumn{{Header: "H", From: "title"}}}
	if _, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{other}}, Resolver{}); err == nil || !strings.Contains(err.Error(), "only on a Table") {
		t.Errorf("columns on a Collection: %v", err)
	}
	// A link cell needs an address; a resolver that gives none is a fault of the resolver, said by name.
	noAddr := tableNode(domain.PageColumn{Header: "Ticket", From: "title", Href: domain.PageRecordRole})
	_, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{noAddr}}, tableRecords(map[string]string{"title": "a"}))
	if err == nil || !strings.Contains(err.Error(), "names no address") {
		t.Errorf("a record with no address: %v", err)
	}
}

// TestTableCellsTakeTheToneTheDataPlaneResolved: a status cell -- by role or by Field -- is a badge in the tone the
// Machine's status_categories gave its option (resolved before lowering), neutral when none was given, and a Field
// column shows the Field's display, not its stored value.
func TestTableCellsTakeTheToneTheDataPlaneResolved(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentTable), string(domain.ComponentStatusBadge)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
	rec := map[string]string{"status": "Overdue", domain.PageStatusToneRole: "bad",
		domain.PageFieldRole("fld_party"): "Reza Anggara", domain.PageFieldRole("fld_pay"): "Settled", domain.PageFieldToneRole("fld_pay"): "good",
		domain.PageFieldRole("fld_value"): "180,000,000"}
	plain := map[string]string{"status": "Draft", domain.PageStatusToneRole: "",
		domain.PageFieldRole("fld_party"): "", domain.PageFieldRole("fld_pay"): "", domain.PageFieldRole("fld_value"): ""}
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{tableNode(
		domain.PageColumn{Header: "Status", From: "status"},
		domain.PageColumn{Header: "Party", Field: "fld_party"},
		domain.PageColumn{Header: "Payment", Field: "fld_pay"},
		domain.PageColumn{Header: "Value", Field: "fld_value", Align: "end"},
	)}}
	tree, err := Lower(root, tableRecords(rec, plain))
	if err != nil {
		t.Fatal(err)
	}
	c := tree.Children[0].Children
	if c[0].Type != "StatusBadge" || c[0].Props["tone"] != "bad" {
		t.Errorf("status role cell = %+v, want a bad-toned badge", c[0])
	}
	if c[1].Type != "paragraph" || c[1].Props["text"] != "Reza Anggara" {
		t.Errorf("a Field column = %+v, want its display as text", c[1])
	}
	if c[2].Type != "StatusBadge" || c[2].Props["tone"] != "good" || c[2].Props["label"] != "Settled" {
		t.Errorf("a status Field column = %+v, want a good-toned badge", c[2])
	}
	if c[3].Props["text"] != "180,000,000" {
		t.Errorf("a money column = %+v", c[3])
	}
	if c[4].Type != "StatusBadge" || c[4].Props["tone"] != string(domain.ToneNeutral) {
		t.Errorf("an option with no category = %+v, want neutral", c[4])
	}
	if c[6].Type != "paragraph" {
		t.Errorf("an empty status Field = %+v, want no badge", c[6])
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("not a valid tree: %v", issues)
	}
}

// TestTableRowActionsAreOneMoreCellPerRecord: a Table's children are its row actions, lowered per record with that
// record's writes -- a Delete drawn only where the record's rules permit it -- under an unnamed, end-aligned header.
func TestTableRowActionsAreOneMoreCellPerRecord(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentTable), string(domain.ComponentButton)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
	n := tableNode(domain.PageColumn{Header: "Ticket", From: "title"})
	n.Children = []domain.PageNode{{Kind: "component", Type: "Button", Props: map[string]string{"label": "Delete", "variant": "danger", "confirm": "Delete it?"},
		Binding: &domain.PageBinding{Write: domain.PageWriteDelete}}}
	r := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{{"title": "a"}, {"title": "b"}},
			Deletes: []RecordAction{{Route: "/machines/mch_t/records/a", Permitted: true}, {Route: "/machines/mch_t/records/b", Permitted: false}}}, nil
	}}
	tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}, r)
	if err != nil {
		t.Fatal(err)
	}
	table := tree.Children[0]
	if table.Props["headers"] != "Ticket\n" || table.Props["aligns"] != "start,end" {
		t.Fatalf("headers %q aligns %q, want an unnamed end-aligned actions column", table.Props["headers"], table.Props["aligns"])
	}
	if len(table.Children) != 4 {
		t.Fatalf("%d cells, want 2 columns x 2 records", len(table.Children))
	}
	first, second := table.Children[1], table.Children[3]
	if len(first.Children) != 1 || first.Children[0].Props["action"] != "/machines/mch_t/records/a" {
		t.Errorf("the permitted record's actions = %+v, want its Delete", first)
	}
	if len(second.Children) != 0 {
		t.Errorf("the refused record's actions = %+v, want none drawn", second)
	}
}

func TestLowerTableWithNoRecordsHasHeadersAndNoCells(t *testing.T) {
	tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{tableNode(domain.PageColumn{Header: "Ticket", From: "title"})}}, tableRecords())
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Children[0]; got.Props["headers"] != "Ticket" || len(got.Children) != 0 {
		t.Errorf("an empty table = %+v", got)
	}
}

func viewerNode(c domain.PageCondition) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "always"}},
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "gated"}, When: &c},
	}}
}

func TestLowerViewerConditionShowsANodeOnlyToTheRoleItNames(t *testing.T) {
	adminOnly := domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole, Is: domain.WorkspaceRoleAdmin}
	for name, tc := range map[string]struct {
		role string
		want int
	}{"an admin": {"admin", 2}, "a member": {"member", 1}, "a viewer the runtime cannot place": {"", 1}} {
		tree, err := Lower(viewerNode(adminOnly), Resolver{Viewer: func(string) string { return tc.role }})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(tree.Children) != tc.want {
			t.Errorf("%s sees %d nodes, want %d", name, len(tree.Children), tc.want)
		}
	}
	// With no viewer at all (the load-time placeholder) the node is kept, so a hidden node is still validated.
	if tree, err := Lower(viewerNode(adminOnly), Resolver{}); err != nil || len(tree.Children) != 2 {
		t.Errorf("the placeholder lowered to %d nodes, %v, want both", len(tree.Children), err)
	}
	// A node the condition hides is never lowered, so what it binds is never read for that viewer.
	reads := 0
	bound := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
		Kind: "component", Type: "Metric", Props: map[string]string{"label": "N"}, When: &adminOnly,
		Binding: &domain.PageBinding{Dataset: "ds_x", Measure: "m", Rows: domain.PageRowsTotal}}}}
	_, _ = Lower(bound, Resolver{Viewer: func(string) string { return "member" }, Total: func(domain.PageBinding) (string, error) { reads++; return "1", nil }})
	if reads != 0 {
		t.Errorf("a hidden Metric still read its Dataset %d time(s)", reads)
	}
}

func TestLowerViewerConditionRefusesEachMisuseForItsOwnReason(t *testing.T) {
	for name, tc := range map[string]struct {
		c    domain.PageCondition
		want string
	}{
		"an unknown fact":          {domain.PageCondition{Viewer: "age", Is: "admin"}, "not a fact a page may test"},
		"a negation":               {domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole, IsNot: "member"}, "never is_not"},
		"no operand":               {domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole}, "never is_not"},
		"both operands":            {domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole, Is: "admin", IsNot: "member"}, "never is_not"},
		"a role that is not one":   {domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole, Is: "owner"}, "not a Workspace role"},
		"a viewer and a role":      {domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole, Role: "status", Is: "admin"}, "not more than one"},
		"a viewer and a parameter": {domain.PageCondition{Viewer: domain.PageViewerWorkspaceRole, Parameter: "week", Is: "admin"}, "not more than one"},
	} {
		_, err := Lower(viewerNode(tc.c), Resolver{Viewer: func(string) string { return "admin" }})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// TestAFormNeverTypesWhatLoweringDerives: `carries` and `multipart` come from the edge a form takes and the Fields it asks;
// a page that types them would plant a hidden field the form never asks (the review of step 2), and `then:` belongs to
// a create form only.
func TestAFormNeverTypesWhatLoweringDerives(t *testing.T) {
	for name, props := range map[string]map[string]string{
		"carries":   {"submit": "Save", "carries": "fld_status=Paid"},
		"multipart": {"submit": "Save", "multipart": "true"},
	} {
		n := domain.PageNode{Kind: "component", Type: "Form", Props: props, Binding: &domain.PageBinding{Dataset: "ds_x", Write: domain.PageWriteCreate}}
		if err := checkFormNode(n, "p"); err == nil {
			t.Errorf("%s typed on a Form was accepted", name)
		}
	}
	update := domain.PageNode{Kind: "component", Type: "Form", Props: map[string]string{"submit": "Save", "then": "record"}, Binding: &domain.PageBinding{Write: domain.PageWriteUpdate}}
	if err := checkFormNode(update, "p"); err == nil {
		t.Error("then: on an update Form was accepted")
	}
}

// TestTableFooterAddsUpTheRowsShown (the review of step 4): the total row is the rows above it -- a search narrows both
// -- written as the cells are ("Rp" and grouping read from them); count says how many; a cut list has no total row.
func TestTableFooterAddsUpTheRowsShown(t *testing.T) {
	cols := []domain.PageColumn{
		{Header: "Contract", Field: "fld_no", Total: TotalCount, TotalLabel: "contracts"},
		{Header: "Paid", Field: "fld_paid", Of: "fld_terms", Total: TotalSum},
		{Header: "Value", Field: "fld_value", Total: TotalSum},
	}
	row := func(paid, terms, value, shown string) map[string]string {
		return map[string]string{"num:fld_paid": paid, "num:fld_terms": terms, "num:fld_value": value, "field:fld_value": shown}
	}
	set := RecordSet{Records: []map[string]string{row("1", "3", "180000000", "Rp 180,000,000.00"), row("0", "2", "84000000", "Rp 84,000,000.00")}}
	got, err := tableFooter(cols, set, false, "p", true, domain.NumberFormatInternational)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2 contracts\t1/5\tRp 264,000,000.00\t"; got != want {
		t.Errorf("footer %q, want %q", got, want)
	}
	set.Truncated = true
	if got, _ := tableFooter(cols, set, false, "p", false, domain.NumberFormatInternational); got != "" {
		t.Errorf("a cut list has a total row %q; it would add up only what is shown", got)
	}
}

// TestFigureIsReadsPastTheCurrency: "Rp 0" is zero, so a Metric's alarm tone goes when nothing is overdue.
func TestFigureIsReadsPastTheCurrency(t *testing.T) {
	for written, want := range map[string]bool{"0": true, "Rp 0": true, "0.00": true, "Rp 10": false, "": false} {
		if got := figureIs(written, 0); got != want {
			t.Errorf("figureIs(%q, 0) = %v, want %v", written, got, want)
		}
	}
}

// TestAMoneyTotalOfZeroDropsItsAlarm: a Metric's tone is a signal drawn only when the figure is not zero, and a money
// total writes its zero with its currency ("Rp 0").
func TestAMoneyTotalOfZeroDropsItsAlarm(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentMetric)})
	metric := domain.PageNode{Kind: "component", Type: "Metric", Props: map[string]string{"label": "Overdue, gross", "tone": "bad"},
		Binding: &domain.PageBinding{Dataset: "ds", Measure: "msr", Rows: domain.PageRowsTotal}}
	for value, toned := range map[string]bool{"Rp 0": false, "Rp 110,000,000": true} {
		r := Resolver{Total: func(domain.PageBinding) (string, error) { return value, nil }}
		tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{metric}}, r)
		if err != nil {
			t.Fatal(err)
		}
		if _, has := tree.Children[0].Props["tone"]; has != toned {
			t.Errorf("%s: tone kept = %v, want %v", value, has, toned)
		}
	}
}

// TestATableCellMaySayASecondLineAndARowItsPlace: `under:` draws another Field of the record as a caption under the
// cell ("Dimas Prayoga -- Week 2" over "CTR/K/031", K02a), `from: row:number` the row's own place (K01d's "#"); an empty
// second line leaves the cell as it was, and `under:` beside anything but `field:` is refused.
func TestATableCellMaySayASecondLineAndARowItsPlace(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentTable)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
	rec := func(title, ref string) map[string]string {
		return map[string]string{"field:fld_title": title, "field:fld_ref": ref}
	}
	root := func(cols ...domain.PageColumn) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{tableNode(cols...)}}
	}
	tree, err := Lower(root(
		domain.PageColumn{Header: "#", From: domain.PageRowPositionColumn},
		domain.PageColumn{Header: "Instalment", Field: "fld_title", Under: "fld_ref"},
	), tableRecords(rec("Dimas — Week 2", "CTR/K/031"), rec("Nadia — Week 2", "")))
	if err != nil {
		t.Fatal(err)
	}
	cells := tree.Children[0].Children
	if cells[0].Props["text"] != "1" || cells[2].Props["text"] != "2" {
		t.Errorf("row places %q and %q, want 1 and 2", cells[0].Props["text"], cells[2].Props["text"])
	}
	two := cells[1]
	if two.Type != string(domain.LayoutStack) || len(two.Children) != 2 || two.Children[0].Props["text"] != "Dimas — Week 2" ||
		two.Children[1].Type != string(domain.StaticCaption) || two.Children[1].Props["text"] != "CTR/K/031" {
		t.Errorf("the second line is %+v, want the cell over a caption of the contract no.", two)
	}
	if one := cells[3]; one.Type != string(domain.StaticParagraph) || one.Props["text"] != "Nadia — Week 2" {
		t.Errorf("a cell with nothing under it is %+v, want the plain cell", one)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a lowered Table is not a valid tree: %v", issues)
	}
	if _, err := Lower(root(domain.PageColumn{Header: "X", From: "title", Under: "fld_ref"}), tableRecords(rec("a", "b"))); err == nil || !strings.Contains(err.Error(), "under:") {
		t.Errorf("under: beside from: was not refused: %v", err)
	}
}

// A total is written the way the Workspace writes money: its marks, two decimals, the cells' own currency symbol.
func TestFormatTotalFollowsTheWorkspacesMarks(t *testing.T) {
	rows := []map[string]string{{"field:fld_v": "Rp 1.800,00"}, {"field:fld_v": "Rp 600,50"}}
	if got := formatTotal(2400.5, rows, domain.PageColumn{}, "fld_v", false, domain.NumberFormatIndonesian); got != "Rp 2.400,50" {
		t.Errorf("indonesian total = %q", got)
	}
	plain := []map[string]string{{"field:fld_n": "2.5"}, {"field:fld_n": "3"}}
	if got := formatTotal(5.5, plain, domain.PageColumn{}, "fld_n", false, domain.NumberFormatInternational); got != "5.5" {
		t.Errorf("a plain number was written as money: %q", got)
	}
}

func TestLowerWorkspaceConditionShowsANodeOnlyWhileTheWorkspaceHoldsOrLacksApplications(t *testing.T) {
	none := domain.PageCondition{Workspace: domain.PageWorkspaceApplications, Is: domain.PageApplicationsNone}
	some := domain.PageCondition{Workspace: domain.PageWorkspaceApplications, IsNot: domain.PageApplicationsNone}
	for name, tc := range map[string]struct {
		c     domain.PageCondition
		fact  string
		shown int
	}{
		"none, with none":        {none, domain.PageApplicationsNone, 2},
		"none, with some":        {none, domain.PageApplicationsSome, 1},
		"is_not none, with some": {some, domain.PageApplicationsSome, 2},
		"is_not none, with none": {some, domain.PageApplicationsNone, 1},
	} {
		tree, err := Lower(viewerNode(tc.c), Resolver{Workspace: func(string) string { return tc.fact }})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(tree.Children) != tc.shown {
			t.Errorf("%s: %d nodes, want %d", name, len(tree.Children), tc.shown)
		}
	}
	// The load-time placeholder has no Workspace: the node is kept, so a hidden node is still validated.
	if tree, err := Lower(viewerNode(none), Resolver{}); err != nil || len(tree.Children) != 2 {
		t.Errorf("the placeholder lowered to %d nodes, %v, want both", len(tree.Children), err)
	}
	// A node the condition hides is never lowered, so what it binds is never read for that Workspace.
	reads := 0
	bound := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
		Kind: "component", Type: "Metric", Props: map[string]string{"label": "N"}, When: &none,
		Binding: &domain.PageBinding{Dataset: "ds_x", Measure: "m", Rows: domain.PageRowsTotal}}}}
	_, _ = Lower(bound, Resolver{Workspace: func(string) string { return domain.PageApplicationsSome }, Total: func(domain.PageBinding) (string, error) { reads++; return "1", nil }})
	if reads != 0 {
		t.Errorf("a hidden Metric still read its Dataset %d time(s)", reads)
	}
}

func TestLowerWorkspaceConditionRefusesEachMisuseForItsOwnReason(t *testing.T) {
	for name, tc := range map[string]struct {
		c    domain.PageCondition
		want string
	}{
		"an unknown fact":          {domain.PageCondition{Workspace: "members", Is: "none"}, "not a fact a page may test"},
		"a value that is not one":  {domain.PageCondition{Workspace: domain.PageWorkspaceApplications, Is: "many"}, "is neither"},
		"no operand":               {domain.PageCondition{Workspace: domain.PageWorkspaceApplications}, "exactly one of is:"},
		"both operands":            {domain.PageCondition{Workspace: domain.PageWorkspaceApplications, Is: "none", IsNot: "some"}, "exactly one of is:"},
		"a workspace and a viewer": {domain.PageCondition{Workspace: domain.PageWorkspaceApplications, Viewer: domain.PageViewerWorkspaceRole, Is: "none"}, "not more than one"},
		"a workspace and a role":   {domain.PageCondition{Workspace: domain.PageWorkspaceApplications, Role: "status", Is: "none"}, "not more than one"},
	} {
		_, err := Lower(viewerNode(tc.c), Resolver{Workspace: func(string) string { return "none" }})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// group_by lays the rows group by group in the order values first appear, row_tone gives each row its record's tone in that new order, and a page may not write what lowering produces.
func TestLowerTableGroupsRowsAndCarriesTheirTones(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentTable), string(domain.ComponentStatusBadge)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
	rec := func(title, state, tone string) map[string]string {
		return map[string]string{"title": title, domain.PageFieldRole("fld_state"): state, domain.PageDateToneRole: tone, domain.PageRecordRole: "/r/" + title}
	}
	node := tableNode(domain.PageColumn{Header: "Ticket", From: "title"})
	node.Props["group_by"], node.Props["row_tone"], node.Props["density"] = "fld_state", domain.PageDateToneRole, "compact"
	root := domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{node}}
	tree, err := Lower(root, tableRecords(rec("a", "Open", "bad"), rec("b", "Done", ""), rec("c", "Open", "good")))
	if err != nil {
		t.Fatal(err)
	}
	table := tree.Children[0]
	if table.Props["groups"] != "Open\t2\nDone\t1" || table.Props["rowtones"] != "bad,good," || table.Props["density"] != "compact" {
		t.Errorf("props = %v", table.Props)
	}
	var order []string
	for _, c := range table.Children {
		order = append(order, c.Props["text"])
	}
	if strings.Join(order, ",") != "a,c,b" {
		t.Errorf("cells in the order %v, want the Open group's rows then Done's", order)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a grouped Table is not a valid tree: %v", issues)
	}
	for _, written := range []string{"groups", "rowtones"} {
		bad := tableNode(domain.PageColumn{Header: "Ticket", From: "title"})
		bad.Props[written] = "x"
		if _, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{bad}}, tableRecords(rec("a", "Open", ""))); err == nil {
			t.Errorf("a page wrote %s", written)
		}
	}
	missing := tableNode(domain.PageColumn{Header: "Ticket", From: "title"})
	missing.Props["group_by"] = "fld_nowhere"
	if _, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{missing}}, tableRecords(rec("a", "Open", ""))); err == nil || !strings.Contains(err.Error(), "not projected") {
		t.Errorf("a Field the Dataset does not project: %v", err)
	}
}

// The row-number column counts the rows as they are laid out after grouping, and an empty group value has a name a person can read.
func TestLowerTableNumbersRowsAfterGroupingAndNamesTheEmptyGroup(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentTable), string(domain.ComponentStatusBadge)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
	rec := func(title, group string) map[string]string {
		return map[string]string{"title": title, domain.PageFieldRole("fld_g"): group, domain.PageRecordRole: "/r/" + title}
	}
	node := tableNode(domain.PageColumn{Header: "#", From: domain.PageRowPositionColumn}, domain.PageColumn{Header: "Ticket", From: "title"})
	node.Props["group_by"] = "fld_g"
	root := domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{node}}
	tree, err := Lower(root, tableRecords(rec("a", "x"), rec("b", "y"), rec("c", "x"), rec("d", "")))
	if err != nil {
		t.Fatal(err)
	}
	table := tree.Children[0]
	var order []string
	for i := 0; i < len(table.Children); i += 2 {
		order = append(order, table.Children[i].Props["text"]+table.Children[i+1].Props["text"])
	}
	if strings.Join(order, ",") != "1a,2c,3b,4d" {
		t.Errorf("rows read %v, want 1a,2c,3b,4d", order)
	}
	if table.Props["groups"] != "x\t2\ny\t1\n(none)\t1" {
		t.Errorf("groups = %q", table.Props["groups"])
	}
}
