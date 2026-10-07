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
      columns: "3"
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

func bound(ds, ms string) string {
	return "      layout: stack\n      children:\n        - component: Metric\n          binding: { dataset: " + ds + ", measure: " + ms + ", rows: dimension }\n"
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
