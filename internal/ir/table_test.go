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
		"a dimension binding":     {func(n *domain.PageNode) { n.Binding.Rows = domain.PageRowsDimension }, "takes a binding with rows: records"},
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
