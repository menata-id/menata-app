package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// cellTable is a Table whose columns' cell templates ride as marked children, exactly as metadata hands them over.
func cellTable(cols []domain.PageColumn, cells map[int]domain.PageNode, actions ...domain.PageNode) domain.PageNode {
	n := tableNode(cols...)
	for i := range cols {
		if c, ok := cells[i+1]; ok {
			c.Cell = i + 1
			n.Children = append(n.Children, c)
		}
	}
	n.Children = append(n.Children, actions...)
	return n
}

func registerCellComponents() {
	RegisterComponentTypes([]string{string(domain.ComponentTable), string(domain.ComponentTag), string(domain.ComponentAvatar), string(domain.ComponentStatusBadge), string(domain.ComponentToggle), string(domain.ComponentButton)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentTable)})
}

func cellResolver(recs []map[string]string, tags [][]map[string]string, tr func(i int) RecordTransition) Resolver {
	return Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		set := RecordSet{Records: recs, Tags: tags, Done: "done", Reopen: "todo"}
		for i := range recs {
			set.Transitions = append(set.Transitions, tr(i))
		}
		return set, nil
	}}
}

func allowAll(complete bool) RecordTransition {
	return RecordTransition{Route: "/machines/mch_t/records/x", Field: "fld_status", Done: "done", Reopen: "todo", Complete: complete,
		Allows: func(string) (bool, bool) { return true, true }}
}

func lowerCellTable(t *testing.T, n domain.PageNode, r Resolver) (UINode, error) {
	t.Helper()
	registerCellComponents()
	tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}, r)
	if err != nil {
		return UINode{}, err
	}
	return tree.Children[0], nil
}

func demoCells() (cols []domain.PageColumn, cells map[int]domain.PageNode) {
	cols = []domain.PageColumn{
		{Header: "Complete", HideHeader: true, Cell: true},
		{Header: "Task", From: "title"},
		{Header: "Label", Cell: true, Empty: "—"},
		{Header: "Assignee", Cell: true, Empty: "none"},
	}
	cells = map[int]domain.PageNode{
		1: {Kind: "component", Type: "Toggle", From: map[string]string{"describes": "title"}, Binding: &domain.PageBinding{Write: domain.PageWriteTransition}},
		3: {Kind: "component", Type: "Tag", Each: domain.PageEachTags, From: map[string]string{"label": "title", "color": "color"}},
		4: {Kind: "component", Type: "Avatar", Props: map[string]string{"size": "compact", "presence": "present"}, From: map[string]string{"initials": domain.PagePersonInitialsRole, "label": "person"}},
	}
	return
}

// TestACellColumnDrawsItsTemplateOncePerRecord: a column's `cell:` is lowered as a Collection's item is -- a Tag per label (a
// row when there are several), the column's `empty:` words when the template draws nothing, an Avatar only for a record that names
// someone -- and the Table says which header is for assistive technology only.
func TestACellColumnDrawsItsTemplateOncePerRecord(t *testing.T) {
	cols, cells := demoCells()
	recs := []map[string]string{
		{"title": "a", "person": "Budi", domain.PagePersonInitialsRole: "B"},
		{"title": "b", "person": "", domain.PagePersonInitialsRole: ""},
	}
	tags := [][]map[string]string{{{"title": "Legal", "color": "rose"}, {"title": "Set", "color": "emerald"}}, nil}
	table, err := lowerCellTable(t, cellTable(cols, cells), cellResolver(recs, tags, func(i int) RecordTransition { return allowAll(i == 1) }))
	if err != nil {
		t.Fatal(err)
	}
	if table.Props["headers"] != "Complete\nTask\nLabel\nAssignee" || table.Props["hidden"] != "0" {
		t.Errorf("headers = %q, hidden = %q", table.Props["headers"], table.Props["hidden"])
	}
	if len(table.Children) != 8 {
		t.Fatalf("%d cells, want 4 columns x 2 records", len(table.Children))
	}
	toggleA, toggleB := table.Children[0], table.Children[4]
	if toggleA.Type != "Toggle" || toggleA.Props["checked"] != "false" || toggleA.Props["value"] != "done" || toggleA.Props["name"] != "fld_status" || toggleA.Props["describes"] != "a" {
		t.Errorf("an open record's Toggle = %+v, want unchecked and writing the Done value", toggleA.Props)
	}
	if toggleB.Props["checked"] != "true" || toggleB.Props["value"] != "todo" {
		t.Errorf("a finished record's Toggle = %+v, want checked and writing the Reopen value", toggleB.Props)
	}
	if row := table.Children[2]; row.Kind != NodeLayout || row.Type != "row" || len(row.Children) != 2 || row.Children[0].Props["label"] != "Legal" {
		t.Errorf("two labels = %+v, want a row of two Tags", row)
	}
	if none := table.Children[6]; none.Type != "paragraph" || none.Props["text"] != "—" {
		t.Errorf("no labels = %+v, want the column's empty: words", none)
	}
	if av := table.Children[3]; av.Type != "Avatar" || av.Props["initials"] != "B" || av.Props["label"] != "Budi" {
		t.Errorf("an assigned record = %+v, want an Avatar of its initials", av)
	}
	if nobody := table.Children[7]; nobody.Type != "paragraph" || nobody.Props["text"] != "none" {
		t.Errorf("an unassigned record = %+v, want empty: and no circle of initials", nobody)
	}
}

// TestAToggleIsNotDrawnWhereTheMoveIsNotAllowed: a viewer who cannot edit gets the column's empty words, not a dead control.
func TestAToggleIsNotDrawnWhereTheMoveIsNotAllowed(t *testing.T) {
	cols, cells := demoCells()
	cols[0].Empty = "locked"
	denied := func(int) RecordTransition {
		tr := allowAll(false)
		tr.Allows = func(string) (bool, bool) { return false, true }
		return tr
	}
	table, err := lowerCellTable(t, cellTable(cols, cells), cellResolver([]map[string]string{{"title": "a", "person": "", domain.PagePersonInitialsRole: ""}}, [][]map[string]string{nil}, denied))
	if err != nil {
		t.Fatal(err)
	}
	if c := table.Children[0]; c.Type != "paragraph" || c.Props["text"] != "locked" {
		t.Errorf("a refused move = %+v, want the empty words", c)
	}
}

func TestACellColumnRefusesEachMisuseForItsOwnReason(t *testing.T) {
	rec := []map[string]string{{"title": "a", "person": "", domain.PagePersonInitialsRole: ""}}
	res := cellResolver(rec, [][]map[string]string{nil}, func(int) RecordTransition { return allowAll(false) })
	for name, tc := range map[string]struct {
		edit func(cols []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn
		want string
	}{
		"a cell beside from:": {func(c []domain.PageColumn, _ map[int]domain.PageNode) []domain.PageColumn {
			c[0].From = "title"
			return c
		}, "takes no from:"},
		"empty: with no cell:": {func(c []domain.PageColumn, _ map[int]domain.PageNode) []domain.PageColumn {
			c[1].Empty = "x"
			return c
		}, "beside cell:"},
		"a cell column with no template": {func(c []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn {
			delete(cells, 3)
			return c
		}, "not carried"},
		"a Toggle with no name": {func(c []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn {
			n := cells[1]
			n.From = nil
			cells[1] = n
			return c
		}, "from: {describes"},
		"a Toggle with a typed value": {func(c []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn {
			n := cells[1]
			n.Props = map[string]string{"value": "done"}
			cells[1] = n
			return c
		}, "never typed"},
		"a Toggle reading another role": {func(c []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn {
			n := cells[1]
			n.From = map[string]string{"describes": "title", "label": "title"}
			cells[1] = n
			return c
		}, "takes only from"},
		"a Toggle bound to a dataset": {func(c []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn {
			n := cells[1]
			n.Binding = &domain.PageBinding{Write: domain.PageWriteTransition, Dataset: "ds_x"}
			cells[1] = n
			return c
		}, "binding is write"},
		"a Toggle written by delete": {func(c []domain.PageColumn, cells map[int]domain.PageNode) []domain.PageColumn {
			n := cells[1]
			n.Binding = &domain.PageBinding{Write: domain.PageWriteDelete}
			cells[1] = n
			return c
		}, "takes a binding with write"},
	} {
		cols, cells := demoCells()
		cols = tc.edit(cols, cells)
		_, err := lowerCellTable(t, cellTable(cols, cells), res)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
	// A Toggle outside a record template has no record to complete.
	registerCellComponents()
	loose := domain.PageNode{Kind: "component", Type: "Toggle", From: map[string]string{"describes": "title"}, Binding: &domain.PageBinding{Write: domain.PageWriteTransition}}
	if _, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{loose}}, Resolver{}); err == nil || !strings.Contains(err.Error(), "record template") {
		t.Errorf("a Toggle on its own: %v", err)
	}
	// A Machine with no completion: has nothing for a Toggle to write.
	cols, cells := demoCells()
	none := func(int) RecordTransition { tr := allowAll(false); tr.Done, tr.Reopen = "", ""; return tr }
	if _, err := lowerCellTable(t, cellTable(cols, cells), cellResolver(rec, [][]map[string]string{nil}, none)); err == nil || !strings.Contains(err.Error(), "completion:") {
		t.Errorf("a Toggle with no completion: %v", err)
	}
	// A Dimension's row is a group, not a record: no cell templates.
	dim := cellTable([]domain.PageColumn{{Header: "H", Cell: true}}, map[int]domain.PageNode{1: {Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}})
	dim.Binding.Rows = domain.PageRowsDimension
	dim.Binding.Measure = ""
	rows := Resolver{Rows: func(domain.PageBinding) ([]Row, error) { return []Row{{Label: "x"}}, nil }}
	if _, err := lowerCellTable(t, dim, rows); err == nil || !strings.Contains(err.Error(), "Dimension") {
		t.Errorf("a cell on a Dimension's row: %v", err)
	}
}
