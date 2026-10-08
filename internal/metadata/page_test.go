package metadata

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// pageFixture loads a one-Machine Workspace whose Application declares `nav` as its navigation, so each test
// states only the part of `page:` it is about. The Machine holds a status Dimension, an aggregate Dataset over
// it and a records Dataset, which is every shape a Binding can be wrong about.
func pageFixture(t *testing.T, nav string) error {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "doc.yaml", `
id: mch_doc
name: Doc
fields:
  - id: fld_status
    name: Status
    type: status
    options: [draft, approved]
  - id: fld_title
    name: Title
    type: text
  - id: fld_parent
    name: Parent
    type: relation
    machine: mch_doc
card_fields:
  - { field: fld_title, role: title }
  - { field: fld_status, role: status }
  - { field: fld_parent, role: person }
datasets:
  - id: ds_by_status
    dimension: fld_status
    measures:
      - id: msr_total
        aggregate: count
  - id: ds_flat
    measures:
      - id: msr_total
        aggregate: count
  - id: ds_rows
    select: records
    limit: 10
  - id: ds_by_param
    select: records
    limit: 10
    where:
      field: fld_title
      op: equals
      value: $parameters.title
`)
	writeFile(t, dir, "app.yaml", `
workspace: default
machines:
  - doc.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_docs
name: Docs
machines:
  - mch_doc
navigation:
`+nav)
	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	return err
}

const validPage = `
  - id: nav_by_status
    label: By status
    route: /pages/nav_by_status
    page:
      layout: grid
      columns: "4"
      mobile: "1"
      gap: default
      children:
        - component: Metric
          binding: { dataset: ds_by_status, measure: msr_total, rows: dimension }
`

func TestPage_validDeclarationLoads(t *testing.T) {
	if err := pageFixture(t, validPage); err != nil {
		t.Fatalf("a valid page: was refused: %v", err)
	}
}

const validListPage = `
  - id: nav_list
    label: List
    route: /pages/nav_list
    page:
      layout: stack
      children:
        - component: Collection
          gap: tight
          binding: { dataset: ds_rows, rows: records }
          children:
            - layout: row
              children:
                - static: paragraph
                  from: { text: title }
                - component: StatusBadge
                  tone: info
                  from: { label: status }
`

func TestPage_recordsBindingLoads(t *testing.T) {
	if err := pageFixture(t, validListPage); err != nil {
		t.Fatalf("a valid records page was refused: %v", err)
	}
}

func TestPage_loweredTreeIsWhatTheAuthorDeclared(t *testing.T) {
	var doc pageNodeDoc
	if err := yaml.Unmarshal([]byte(`
layout: stack
gap: tight
children:
  - static: paragraph
    text: Hello
  - component: Metric
    tone: warn
    binding: { dataset: ds_x, measure: msr_y, rows: dimension }
`), &doc); err != nil {
		t.Fatal(err)
	}
	n := doc.toDomain()
	if n.Kind != "layout" || n.Type != "stack" || n.Props["gap"] != "tight" {
		t.Errorf("root = %+v", n)
	}
	if len(n.Children) != 2 || n.Children[0].Props["text"] != "Hello" {
		t.Fatalf("children = %+v", n.Children)
	}
	b := n.Children[1].Binding
	if b == nil || b.Dataset != "ds_x" || b.Measure != "msr_y" || b.Rows != "dimension" || n.Children[1].Props["tone"] != "warn" {
		t.Errorf("bound child = %+v", n.Children[1])
	}
}

// Each case is one fault the loader must refuse, named by the rejection it exercises. The positive test above
// is what stops this table from passing because the validator refuses everything.
// TestPage_aTagLoadsAndItsPaletteIsHeldAtLoad: a page may declare `component: Tag`, and the closed palette is
// checked where every other Component's input is -- at load, through the registered validator -- so
// `chartreuse` is a refusal and not a chip silently drawn slate.
func TestPage_aTagLoadsAndItsPaletteIsHeldAtLoad(t *testing.T) {
	tag := func(props string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - component: Tag\n" + props
	}
	for name, props := range map[string]string{
		"label alone":       "          label: Urgent\n",
		"label and palette": "          label: Urgent\n          color: amber\n",
	} {
		if err := pageFixture(t, tag(props)); err != nil {
			t.Errorf("%s: a valid Tag was refused: %v", name, err)
		}
	}
	for name, c := range map[string]struct{ props, want string }{
		"colour outside the palette": {"          label: Urgent\n          color: chartreuse\n", "chartreuse"},
		"no label":                   {"          color: amber\n", "label"},
		"a class":                    {"          label: Urgent\n          class: mr-1\n", "class"},
	} {
		err := pageFixture(t, tag(c.props))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v; want a refusal mentioning %q", name, err, c.want)
		}
	}
}

func TestPage_faultsAreRefusedAtLoad(t *testing.T) {
	item := func(route, page string) string {
		return "  - id: nav_p\n    label: P\n    route: " + route + "\n    page:\n" + page
	}
	cases := []struct {
		name string
		nav  string
		want string
	}{
		{"wrong route", item("/elsewhere", "      layout: stack\n"), "a page is rendered at"},
		{"root is not a layout", item("/pages/nav_p", "      static: heading\n      text: x\n"), "root must be a layout"},
		{"two discriminators", item("/pages/nav_p", "      layout: stack\n      static: heading\n"), "exactly one of"},
		{"no discriminator", item("/pages/nav_p", "      gap: tight\n"), "exactly one of"},
		{"unknown layout type", item("/pages/nav_p", "      layout: carousel\n"), "carousel"},
		{"undeclared property", item("/pages/nav_p", "      layout: stack\n      colour: red\n"), "colour"},
		{"unknown component", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Gauge\n"), "Gauge"},
		{"binding on a layout", item("/pages/nav_p", "      layout: stack\n      binding: { dataset: ds_by_status, measure: msr_total, rows: dimension }\n"), "bindable"},
		{"binding on a non-bindable component", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Avatar\n          binding: { dataset: ds_by_status, measure: msr_total, rows: dimension }\n"), "bindable"},
		{"binding names no dataset", item("/pages/nav_p", bound("ds_missing", "msr_total")), "ds_missing"},
		{"binding names no measure", item("/pages/nav_p", bound("ds_by_status", "msr_nope")), "msr_nope"},
		{"binding on a dataset without a dimension", item("/pages/nav_p", bound("ds_flat", "msr_total")), "no dimension"},
		{"binding on a records dataset", item("/pages/nav_p", bound("ds_rows", "msr_total")), "selects records"},
		{"binding with an expression", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Metric\n          binding: { dataset: ds_by_status, measure: [a, b], rows: dimension }\n"), "no grammar"},
		{"binding with an unknown key", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Metric\n          binding: { dataset: ds_by_status, measure: msr_total, rows: dimension, filter: x }\n"), "not a key a binding declares"},
		{"bound node also types its figure", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Metric\n          value: \"42\"\n          binding: { dataset: ds_by_status, measure: msr_total, rows: dimension }\n"), "comes from the binding"},
		{"bound node with children", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Metric\n          binding: { dataset: ds_by_status, measure: msr_total, rows: dimension }\n          children:\n            - static: paragraph\n              text: x\n"), "holds no children"},
		{"static node holding a child", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: heading\n          text: x\n          children:\n            - static: paragraph\n              text: y\n"), "slot/type mismatch"},
		{"leaf component holding a child", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Avatar\n          initials: AB\n          label: Ab\n          children:\n            - static: paragraph\n              text: y\n"), "declares no slots"},
		{"records binding on an aggregate dataset", item("/pages/nav_p", list("ds_by_status", "title")), "is an aggregate"},
		{"records binding filtering on a request parameter", item("/pages/nav_p", list("ds_by_param", "title")), "no request parameters"},
		{"records binding names no dataset", item("/pages/nav_p", list("ds_missing", "title")), "ds_missing"},
		{"from names a role no machine projects", item("/pages/nav_p", list("ds_rows", "nonesuch")), "nonesuch"},
		{"from names a role this machine does not declare", item("/pages/nav_p", list("ds_rows", "money")), "declares no card_fields role"},
		{"from names a reference-backed role", item("/pages/nav_p", list("ds_rows", "person")), "reference to another machine"},
		{"from outside any records template", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: paragraph\n          from: { text: title }\n"), "from: is valid only"},
		{"from as a list", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: paragraph\n          from: [title]\n"), "from: is a mapping"},
		{"from with an expression value", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: paragraph\n          from: { text: [title] }\n"), "names one Projection role"},
		{"link to an item the Application does not declare", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: link\n          to: nav_missing\n"), "nav_missing"},
		{"link with a typed href", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: link\n          to: nav_p\n          href: /typed\n"), "never a typed href"},
		{"link with no destination", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: link\n          text: x\n"), "to: <navigation item id>"},
		{"to on a node that is not a link", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: paragraph\n          to: nav_p\n"), "only on static: link"},
		{"to as a route", item("/pages/nav_p", "      layout: stack\n      children:\n        - static: link\n          to: [nav_p]\n"), "never a route"},
		{"record link takes href from a Projection role", item("/pages/nav_p", recordLink("{ text: title, href: title }")), "only"},
		{"record link shows the route as text", item("/pages/nav_p", recordLink("{ text: record, href: record }")), "valid only as a link's href"},
		{"record link with nothing to say", item("/pages/nav_p", recordLink("{ href: record }")), "text"},
		{"record link to a role the machine does not declare", item("/pages/nav_p", recordLink("{ text: money, href: record }")), "declares no card_fields role"},
		{"depth beyond the maximum", item("/pages/nav_p", deep(12)), "depth"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := pageFixture(t, c.nav)
			if err == nil {
				t.Fatalf("loaded; want a refusal mentioning %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v; want it to mention %q", err, c.want)
			}
		})
	}
}

func TestPage_aLinkResolvesAgainstTheApplicationsOwnNavigation(t *testing.T) {
	nav := "  - id: nav_other\n    label: Other\n    route: /other\n" +
		"  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - static: link\n          to: nav_other\n"
	if err := pageFixture(t, nav); err != nil {
		t.Fatalf("a link to a declared sibling must load: %v", err)
	}
}

func TestPage_aRecordLinkLoadsAndNamesNoRoleForTheRoute(t *testing.T) {
	if err := pageFixture(t, "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n"+recordLink("{ text: title, href: record }")); err != nil {
		t.Fatalf("a link whose href is the reserved record word must load: %v", err)
	}
}

// recordLink is a records-bound Collection over ds_rows whose one item is a link taking `from`.
func recordLink(from string) string {
	return "      layout: stack\n      children:\n" + "        - component: Collection\n          gap: tight\n          binding: { dataset: ds_rows, rows: records }\n          children:\n            - static: link\n              from: " + from + "\n"
}

func bound(ds, ms string) string {
	return "      layout: stack\n      children:\n        - component: Metric\n          binding: { dataset: " + ds + ", measure: " + ms + ", rows: dimension }\n"
}

// list is a records-bound Collection over dataset, whose one item paragraph takes its text from role.
func list(dataset, role string) string {
	return "      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: " + dataset +
		", rows: records }\n          children:\n            - static: paragraph\n              from: { text: " + role + " }\n"
}

// deep nests stacks n levels below the root, past ir.MaxDepth.
func deep(n int) string {
	var b strings.Builder
	b.WriteString("      layout: stack\n")
	indent := "      "
	for i := 0; i < n; i++ {
		b.WriteString(indent + "children:\n" + indent + "  - layout: stack\n")
		indent += "    "
	}
	return b.String()
}

func TestPage_aListOfLinkLoadsAndItsDatasetIsHeldAtLoad(t *testing.T) {
	page := func(dataset string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - static: link\n          list_of: " + dataset + "\n          text: All of them\n"
	}
	if err := pageFixture(t, page("ds_rows")); err != nil {
		t.Fatalf("a link to a declared Dataset's Machine must load: %v", err)
	}
	err := pageFixture(t, page("ds_missing"))
	if err == nil || !strings.Contains(err.Error(), "ds_missing") {
		t.Fatalf("err = %v; want a Dataset this Workspace does not declare refused at load, naming it", err)
	}
	if err := pageFixture(t, strings.Replace(page("ds_rows"), "list_of: ds_rows", "list_of: ds_rows\n          to: nav_p", 1)); err == nil {
		t.Error("a link naming both to: and list_of: loaded; it has two destinations")
	}
}
