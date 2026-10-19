package metadata

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
)

const cellTablePage = `
  - id: nav_table
    label: Table
    route: /pages/nav_table
    page:
      layout: stack
      children:
        - component: Table
          label: Docs
          columns:
            - { header: Title, from: title, href: record }
            - header: Status
              empty: "—"
              cell:
                component: StatusBadge
                tone: neutral
                from: { label: status }
            - header: Owner
              hide_header: true
              cell:
                component: StatusBadge
                tone: neutral
                from: { label: "person:initials" }
          binding: { dataset: ds_rows, rows: records }
`

// TestAColumnsCellIsDecodedAsTheTemplateOfItsColumn: `cell:` is one page node, kept as a child of the Table marked with the
// column's place, so every walk over a record template reads it; `empty:` and `hide_header:` are the column's own.
func TestAColumnsCellIsDecodedAsTheTemplateOfItsColumn(t *testing.T) {
	var doc pageNodeDoc
	src := "component: Table\nlabel: T\ncolumns:\n  - { header: A, from: title }\n  - header: B\n    empty: none\n    hide_header: true\n    cell: { component: Tag, each: tags, from: { label: title, color: color } }\nbinding: { dataset: ds_rows, rows: records }\n"
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	n := doc.toDomain()
	if len(n.Columns) != 2 || n.Columns[0].Cell || !n.Columns[1].Cell || n.Columns[1].Empty != "none" || !n.Columns[1].HideHeader {
		t.Errorf("columns = %+v", n.Columns)
	}
	if len(n.Children) != 1 || n.Children[0].Cell != 2 || n.Children[0].Type != "Tag" || n.Children[0].Each != domain.PageEachTags {
		t.Errorf("children = %+v, want the Tag template marked with column 2", n.Children)
	}
	if err := pageFixture(t, cellTablePage); err != nil {
		t.Errorf("a Table with cell templates was refused: %v", err)
	}
}

func TestAColumnsCellMisuseIsALoadError(t *testing.T) {
	for name, src := range map[string]string{
		"a scalar cell":         "component: Table\ncolumns:\n  - { header: A, cell: Tag }\n",
		"a list for a cell":     "component: Table\ncolumns:\n  - header: A\n    cell: [x]\n",
		"hide_header, a word":   "component: Table\ncolumns:\n  - { header: A, from: title, hide_header: maybe }\n",
		"a cell with a bad key": "component: Table\ncolumns:\n  - header: A\n    cell: { component: Tag, each: [] }\n",
	} {
		var doc pageNodeDoc
		if err := yaml.Unmarshal([]byte(src), &doc); err == nil {
			t.Errorf("%s: the declaration was accepted", name)
		}
	}
	for name, tc := range map[string]struct{ from, to, want string }{
		"a cell reading a role nobody projects": {"from: { label: status }", "from: { label: subtitle }", "subtitle"},
		"a cell beside from":                    {"header: Status\n              empty: \"—\"", "header: Status\n              from: status\n              empty: \"—\"", "takes no from"},
		"empty: with no cell":                   {"{ header: Title, from: title, href: record }", "{ header: Title, from: title, href: record, empty: x }", "beside cell"},
	} {
		err := pageFixture(t, strings.Replace(cellTablePage, tc.from, tc.to, 1))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// TestAToggleNeedsACompletionToWrite: the Toggle has no becomes:, so what it asks of the Machine is the completion it writes,
// and a status Field that can hold both of its values.
func TestAToggleNeedsACompletionToWrite(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}}},
		CardFields: []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}}}
	toggle := domain.PageNode{Kind: "component", Type: "Toggle", From: map[string]string{"describes": "status"}, Binding: &domain.PageBinding{Write: domain.PageWriteTransition}}
	run := func() string { return strings.Join(transitionBindingIssues(toggle, m, "w"), "\n") }
	if got := run(); !strings.Contains(got, "completion") {
		t.Errorf("a Toggle over a Machine with no completion: %q", got)
	}
	m.Completion = &domain.Completion{Field: "fld_status", Done: "done"}
	if got := run(); got != "" {
		t.Errorf("a Toggle over a Machine that completes: %q", got)
	}
	m.Completion.Done = "shipped"
	if got := run(); !strings.Contains(got, "shipped") {
		t.Errorf("a completion value the status Field cannot hold: %q", got)
	}
}

// TestACellTemplateIsHeldToTheSameRulesAsAnyRecordTemplate: a cell is not a back door around what a record template may hold -- an
// `each: tags` over a Machine with no `card_tags:`, a Button writing a Field by a route, and a Form are all refused where they sit.
func TestACellTemplateIsHeldToTheSameRulesAsAnyRecordTemplate(t *testing.T) {
	cell := func(body string) string {
		return strings.Replace(cellTablePage, "              cell:\n                component: StatusBadge\n                tone: neutral\n                from: { label: status }", "              cell:\n"+body, 1)
	}
	for name, tc := range map[string]struct{ body, want string }{
		"tags with no card_tags":     {"                component: Tag\n                each: tags\n                from: { label: title, color: color }", "card_tags"},
		"a typed action":             {"                component: Button\n                label: Go\n                variant: secondary\n                action: /x", "action"},
		"a Form in a cell":           {"                component: Form\n                submit: Go\n                binding: { write: create }", "dataset"},
		"a bare Toggle (no write)":   {"                component: Toggle\n                from: { describes: title }", "Toggle"},
		"a delete bound to a Toggle": {"                component: Toggle\n                from: { describes: title }\n                binding: { write: delete }", "Toggle"},
	} {
		err := pageFixture(t, cell(tc.body))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}
