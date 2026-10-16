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
		"children":                {func(n *domain.PageNode) { n.Children = []domain.PageNode{{Kind: "static", Type: "paragraph"}} }, "holds no children"},
		"from on the table":       {func(n *domain.PageNode) { n.From = map[string]string{"label": "title"} }, "do not belong on a Table"},
		"written headers":         {func(n *domain.PageNode) { n.Props["headers"] = "X" }, "never written"},
		"written aligns":          {func(n *domain.PageNode) { n.Props["aligns"] = "end" }, "never written"},
		"no columns":              {func(n *domain.PageNode) { n.Columns = nil }, "declares columns"},
		"too many columns":        {func(n *domain.PageNode) { n.Columns = many }, "at most"},
		"a blank header":          {func(n *domain.PageNode) { n.Columns[0].Header = " " }, "header"},
		"a header with a newline": {func(n *domain.PageNode) { n.Columns[0].Header = "A\nB" }, "header"},
		"no role":                 {func(n *domain.PageNode) { n.Columns[0].From = "" }, "from:"},
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

func TestLowerTableWithNoRecordsHasHeadersAndNoCells(t *testing.T) {
	tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{tableNode(domain.PageColumn{Header: "Ticket", From: "title"})}}, tableRecords())
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Children[0]; got.Props["headers"] != "Ticket" || len(got.Children) != 0 {
		t.Errorf("an empty table = %+v", got)
	}
}
