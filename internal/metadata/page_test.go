package metadata

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/ir"
	"menata.app/internal/registry"
)

// pageFixture loads a one-Machine Workspace whose Application declares `nav` as its navigation, so each test
// states only the part of `page:` it is about. The Machine holds a status Dimension, an aggregate Dataset over
// it and a records Dataset, which is every shape a Binding can be wrong about.
func pageFixture(t *testing.T, nav string) error {
	t.Helper()
	return pageFixtureWith(t, nav, "relation\n    machine: mch_doc")
}

// pageFixtureWith is pageFixture with the Field in the `person` role declared as `parentType` (a `type:` value,
// possibly followed by more lines), so a test can say which kind of reference that role is.
func pageFixtureWith(t *testing.T, nav, parentType string) error {
	t.Helper()
	dir := t.TempDir()
	machineFiles, appMachines := "  - doc.yaml\n", "  - mch_doc\n"
	if parentType == "person" {
		writeFile(t, dir, "user.yaml", "id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n")
		machineFiles, appMachines = "  - user.yaml\n"+machineFiles, "  - mch_user\n"+appMachines
	}
	// The Machine declares a `date` card role only for a page that groups by it: another test needs a Machine
	// that declares none, to prove the derived date roles refuse one.
	dateRole := ""
	if strings.Contains(nav, "group:") && !strings.Contains(nav, "label: Undated") {
		dateRole = "  - { field: fld_due, role: date }\n"
	}
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
  - id: fld_owner
    name: Owner
    type: `+parentType+`
  - id: fld_color
    name: Color
    type: status
    options: [blue, amber]
  - id: fld_due
    name: Due
    type: date
  - id: fld_start
    name: Start
    type: date
card_fields:
`+dateRole+`  - { field: fld_title, role: title }
  - { field: fld_status, role: status }
  - { field: fld_color, role: color }
  - { field: fld_owner, role: person }
  - { field: fld_parent, role: container }
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
  - id: ds_with_children
    select: records
    limit: 10
    relations:
      - id: rel_children
        machine: mch_doc
        via: fld_parent
  - id: ds_week
    select: records
    limit: 10
    where:
      all:
        - { field: fld_due, op: gte, value: $week }
        - { field: fld_due, op: lte, value: $week+6 }
  - id: ds_week_twice
    select: records
    limit: 10
    where:
      all:
        - { field: fld_due, op: gte, value: $week }
        - { field: fld_due, op: gte, value: $week+1 }
        - { field: fld_due, op: lte, value: $week+6 }
  - id: ds_week_beside_another
    select: records
    limit: 10
    where:
      all:
        - { field: fld_due, op: gte, value: $week }
        - { field: fld_due, op: lte, value: $week+6 }
        - { field: fld_start, op: gte, value: $week }
        - { field: fld_start, op: lte, value: $week+6 }
  - id: ds_week_open
    select: records
    limit: 10
    where:
      field: fld_due
      op: gte
      value: $week
  - id: ds_week_any
    select: records
    limit: 10
    where:
      any:
        - { field: fld_due, op: gte, value: $week }
        - { field: fld_due, op: lte, value: $week+6 }
  - id: ds_by_param
    select: records
    limit: 10
    where:
      field: fld_title
      op: equals
      value: $parameters.title
`)
	writeFile(t, dir, "app.yaml", "\nworkspace: default\nmachines:\n"+machineFiles+"applications:\n  - app-main.yaml\n")
	writeFile(t, dir, "app-main.yaml", "\nid: app_docs\nname: Docs\nmachines:\n"+appMachines+"navigation:\n"+nav)
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

// A page's request is its query string, so a Dataset filtering on `$parameters.<name>` is bindable (007 §9.2).
// It was a load error while a page had no way to supply one; the refusal now would silently disable a
// declared capability.
func TestPage_aRecordsBindingMayFilterOnARequestParameter(t *testing.T) {
	if err := pageFixture(t, strings.Replace(validListPage, "ds_rows", "ds_by_param", 1)); err != nil {
		t.Fatalf("a records page over a parameter-filtered dataset was refused: %v", err)
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

// TestPage_anEmptyStateLoadsAndABlankOneIsRefused: a page may declare `component: EmptyState` with its one input, and a
// blank message, a missing one or any property the contract does not declare is a load error. The Component has no
// installed consumer: a page cannot show it only when a list is empty, which is why `Collection` has `empty:`.
func TestPage_anEmptyStateLoadsAndABlankOneIsRefused(t *testing.T) {
	empty := func(props string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - component: EmptyState\n" + props
	}
	if err := pageFixture(t, empty("          text: Reports build overnight.\n")); err != nil {
		t.Errorf("a valid EmptyState was refused: %v", err)
	}
	for name, c := range map[string]struct{ props, want string }{
		"no text":      {"", "text"},
		"a blank text": {"          text: \"   \"\n", "blank"},
		"a title":      {"          text: Nothing yet.\n          title: Empty\n", "title"},
		"a class":      {"          text: Nothing yet.\n          class: mt-2\n", "class"},
	} {
		err := pageFixture(t, empty(c.props))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v; want a refusal mentioning %q", name, err, c.want)
		}
	}
}

// TestPage_aCalloutLoadsAndItsToneIsHeldAtLoad: a page may declare `component: Callout`, and the tones a message takes
// are checked at load through the registered validator, so `neutral` (a label tone, not a message) is refused.
func TestPage_aCalloutLoadsAndItsToneIsHeldAtLoad(t *testing.T) {
	callout := func(props string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - component: Callout\n" + props
	}
	if err := pageFixture(t, callout("          text: Nothing to review today.\n          tone: info\n")); err != nil {
		t.Errorf("a valid Callout was refused: %v", err)
	}
	for name, c := range map[string]struct{ props, want string }{
		"a label tone":  {"          text: Saved.\n          tone: neutral\n", "neutral"},
		"no tone":       {"          text: Saved.\n", "tone"},
		"a chosen role": {"          text: Saved.\n          tone: good\n          role: alert\n", "role"},
	} {
		err := pageFixture(t, callout(c.props))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v; want a refusal mentioning %q", name, err, c.want)
		}
	}
}

// TestPage_aPersonRoleIsTheOneReferenceAPageResolves: a records template may take its text from a role whose Field
// is a person, because the Workspace's members are answered by the Loader's memoized PersonNames; a relation to
// any other Machine stays refused (it would read that whole Machine, 007 §20).
func TestPage_aPersonRoleIsTheOneReferenceAPageResolves(t *testing.T) {
	page := "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n" + list("ds_rows", "person")
	if err := pageFixtureWith(t, page, "person"); err != nil {
		t.Errorf("a person role was refused: %v", err)
	}
	err := pageFixtureWith(t, page, "relation\n    machine: mch_doc")
	if err == nil || !strings.Contains(err.Error(), "only a person Field and a container Field are resolved") {
		t.Errorf("a relation role: error = %v; want it refused", err)
	}
}

// TestPage_aContainerRoleLoadsWhereARelationRoleIsRefused: the other reference a page resolves. A `container` Field
// is a relation, and its title is read for only the records the page lists (composition.Loader.RelatedLabels), so a
// page may take its text from it; the same relation declared under any other role is still the whole-Machine read.
func TestPage_aContainerRoleLoadsWhereARelationRoleIsRefused(t *testing.T) {
	page := "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n" + list("ds_rows", "container")
	if err := pageFixtureWith(t, page, "person"); err != nil {
		t.Errorf("a container role was refused: %v", err)
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
		{"derived date role over a machine with no date role", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: ds_rows, rows: records }\n          children:\n            - component: StatusBadge\n              from: { label: \"date:label\", tone: \"date:tone\" }\n"), "declares no card_fields role \"date\" to derive them from"},
		{"derived group role over a machine whose view groups by nothing", item("/pages/nav_p", "      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: ds_rows, rows: records }\n          children:\n            - component: StatusBadge\n              tone: neutral\n              from: { label: \"group:label\" }\n"), "groups by no Field to derive it from"},
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

// TestPage_aRecordsTemplateCanFillATagFromTheRecord: a Tag's label and colour may each come from a Projection
// role, which is the whole of binding a record's label to the registered Component. The load-time check
// stands in a placeholder for the record, and the palette check must not refuse that placeholder: whether a
// real record's value is in the palette is the data's question, answered at render by the neutral fallback.
func TestPage_aRecordsTemplateCanFillATagFromTheRecord(t *testing.T) {
	page := "      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: ds_rows, rows: records }\n          children:\n            - component: Tag\n              from: { label: title, color: color }\n"
	if err := pageFixture(t, "  - id: nav_t\n    label: T\n    route: /pages/nav_t\n    page:\n"+page); err != nil {
		t.Fatalf("a Tag filled from a record was refused: %v", err)
	}
}

// TestPage_aTagColourMayNotComeFromARoleThatIsNotAPalette: only the `color` role is a palette entry, so
// wiring a title (or any other role) into a Tag's colour is refused at load rather than drawn slate for ever.
func TestPage_aTagColourMayNotComeFromARoleThatIsNotAPalette(t *testing.T) {
	page := "      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: ds_rows, rows: records }\n          children:\n            - component: Tag\n              from: { label: title, color: title }\n"
	err := pageFixture(t, "  - id: nav_t\n    label: T\n    route: /pages/nav_t\n    page:\n"+page)
	if err == nil || !strings.Contains(err.Error(), "palette") {
		t.Fatalf("error = %v; want a refusal naming the palette", err)
	}
}

func countPage(of, bound string) string {
	return "  - id: nav_c\n    label: C\n    route: /pages/nav_c\n    page:\n      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: " + bound + ", rows: records }\n          children:\n            - static: caption\n              count: { of: " + of + ", one: \"{n} child\", other: \"{n} children\" }\n"
}

// TestPage_aCountNamesARelationTheBoundDatasetDeclares: `count: {of: rel_...}` loads exactly when the
// Dataset the enclosing Collection is bound to declares that Relation (007 §7.5). A Relation of a *different*
// Dataset is not enough -- the count travels with the bound Dataset's selection.
func TestPage_aCountNamesARelationTheBoundDatasetDeclares(t *testing.T) {
	if err := pageFixture(t, countPage("rel_children", "ds_with_children")); err != nil {
		t.Fatalf("a count over a declared Relation was refused: %v", err)
	}
	for name, nav := range map[string]string{
		"a Relation no Dataset declares":            countPage("rel_nope", "ds_with_children"),
		"a bound Dataset that declares no Relation": countPage("rel_children", "ds_rows"),
	} {
		err := pageFixture(t, nav)
		if err == nil || !strings.Contains(err.Error(), "does not declare") {
			t.Errorf("%s: error = %v; want a refusal saying the Dataset does not declare it", name, err)
		}
	}
}

func TestPage_aCountBlockWithAnUnknownKeyIsRefused(t *testing.T) {
	nav := strings.Replace(countPage("rel_children", "ds_with_children"), "other:", "many:", 1)
	if err := pageFixture(t, nav); err == nil {
		t.Fatal("count: with a key it does not declare loaded")
	}
}

func completePage(value string) string {
	return "  - id: nav_k\n    label: K\n    route: /pages/nav_k\n    page:\n      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          complete: " + value + "\n          binding: { dataset: ds_rows, rows: records }\n          children:\n            - static: link\n              from: { text: title, href: record }\n"
}

// TestPage_aListMayClaimItIsCompleteAndNothingElseMayBeSaidAboutItsBound: `complete: true|false` loads on a
// records-bound Collection (the load-time placeholder is never cut short, so the claim is only checked for
// meaning); anything else, or the runtime-produced `truncated:` typed by hand, is a load error.
func TestPage_aListMayClaimItIsCompleteAndNothingElseMayBeSaidAboutItsBound(t *testing.T) {
	for _, v := range []string{"true", "false"} {
		if err := pageFixture(t, completePage(v)); err != nil {
			t.Errorf("complete: %s was refused: %v", v, err)
		}
	}
	if err := pageFixture(t, completePage("yes")); err == nil {
		t.Error("complete: yes loaded")
	}
	typed := strings.Replace(completePage("true"), "complete: true", "truncated: 5", 1)
	if err := pageFixture(t, typed); err == nil {
		t.Error("a hand-written truncated: loaded; it is the runtime's to produce")
	}
}

// TestPage_aListCutShortByItsBoundStillValidates: the loader only ever lowers a page against a placeholder that
// is never cut short, so the `truncated` prop lowering produces at request time is otherwise never seen by
// `ir.Validate` or the Collection's registered validator. This lowers against a cut selection and runs both.
func TestPage_aListCutShortByItsBoundStillValidates(t *testing.T) {
	ensureIRVocabulary()
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
		Kind: "component", Type: "Collection", Props: map[string]string{"gap": "tight", "complete": "true"},
		Binding:  &domain.PageBinding{Dataset: "ds_rows", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}}},
	}}}
	res := PlaceholderResolver(nil, nil)
	inner := res.Records
	res.Records = func(b domain.PageBinding) (ir.RecordSet, error) {
		set, err := inner(b)
		set.Truncated, set.Limit = true, 200
		return set, err
	}
	tree, err := ir.Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Children[0].Props["truncated"]; got != "200" {
		t.Fatalf("truncated = %q, want 200", got)
	}
	if issues := ir.Validate(tree); len(issues) != 0 {
		t.Errorf("a cut list's tree does not validate: %v", issues)
	}
	if issues := registry.ValidateComponentUse(domain.ComponentCollection, tree.Children[0].Props); len(issues) != 0 {
		t.Errorf("a cut list's Collection is refused by its own validator: %v", issues)
	}
}

const linkingMetricPage = `
  - id: nav_overview
    label: Overview
    route: /pages/nav_overview
    page:
      layout: stack
      children:
        - component: Metric
          binding: { dataset: ds_by_status, measure: msr_total, rows: dimension }
          to: nav_list
          param: title
`

// A Metric that sends ?title= must land on a page whose Dataset reads $parameters.title; otherwise the link
// draws, works, and filters nothing -- which no test of either page alone would see.
func TestPage_aLinkingMetricMustSendAParameterTheDestinationReads(t *testing.T) {
	good := linkingMetricPage + strings.Replace(validListPage, "ds_rows", "ds_by_param", 1)
	if err := pageFixture(t, good); err != nil {
		t.Fatalf("a linking Metric over a page that reads the parameter was refused: %v", err)
	}
	for name, nav := range map[string]string{
		"destination reads no parameter": linkingMetricPage + validListPage,
		"destination reads another":      strings.Replace(linkingMetricPage, "param: title", "param: other", 1) + strings.Replace(validListPage, "ds_rows", "ds_by_param", 1),
		"destination has no page":        linkingMetricPage + "\n  - id: nav_list\n    label: List\n    route: /somewhere\n",
	} {
		err := pageFixture(t, nav)
		if err == nil || !strings.Contains(err.Error(), "filtering on $parameters.") {
			t.Errorf("%s: err = %v, want the parameter mismatch reported", name, err)
		}
	}
}

func TestWriteBindingIssues(t *testing.T) {
	ds := domain.Dataset{ID: "ds_x", Source: "mch_x", Select: domain.SelectRecords}
	node := domain.PageNode{Kind: "component", Type: "Form", Binding: &domain.PageBinding{Dataset: "ds_x", Write: domain.PageWriteCreate}}
	datasets := map[string]domain.Dataset{"ds_x": ds}
	check := func(m *domain.Machine) string {
		machines := map[string]*domain.Machine{}
		if m != nil {
			machines["mch_x"] = m
		}
		return strings.Join(bindingIssues(node, datasets, machines, nil, "w"), "\n")
	}
	text := domain.Field{ID: "f1", Name: "Name", Type: domain.FieldTypeText, Required: true}
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{text}}); got != "" {
		t.Errorf("a Machine a form can ask for was refused: %s", got)
	}
	if got := check(nil); !strings.Contains(got, "does not install") {
		t.Errorf("a Dataset whose Machine is absent: %q", got)
	}
	stamped := domain.Field{ID: "f2", Name: "By", Type: domain.FieldTypeText, Stamp: domain.FieldStampCurrentUser}
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{stamped}}); !strings.Contains(got, "no Field a form can ask for") {
		t.Errorf("a Machine with only stamped Fields: %q", got)
	}
	// A relation is a select of the related Machine's records (an instalment is created for a contract), so a
	// required one is asked for; a group Field is still one no page form can ask for.
	ref := domain.Field{ID: "f3", Name: "Owner", Type: domain.FieldTypeRelation, RelatedMachine: "mch_y", Required: true}
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{text, ref}}); got != "" {
		t.Errorf("a required relation was refused: %q", got)
	}
	group := domain.Field{ID: "f4", Name: "Team", Type: domain.FieldTypeGroup, Required: true}
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{text, group}}); !strings.Contains(got, `requires field "f4"`) {
		t.Errorf("a required group no form can ask for: %q", got)
	}
	group.Required = false
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{text, group}}); got != "" {
		t.Errorf("an optional group is simply not asked for, got %q", got)
	}
}

// `write: update` acts on the record of the Collection it sits in. Outside one it has no record; with a dataset
// it would be a second Collection; over a Machine with nothing an edit form can ask for it could never draw.
func TestUpdateBindingIssues(t *testing.T) {
	text := domain.Field{ID: "f1", Name: "Name", Type: domain.FieldTypeText}
	editable := &domain.Machine{ID: "mch_x", Fields: []domain.Field{text}}
	boolOnly := &domain.Machine{ID: "mch_x", Fields: []domain.Field{{ID: "f2", Name: "Done", Type: domain.FieldTypeBoolean}}}
	node := func(dataset string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: "Form", Binding: &domain.PageBinding{Dataset: dataset, Write: domain.PageWriteUpdate}}
	}
	issues := func(n domain.PageNode, item *domain.Machine) string {
		return strings.Join(bindingIssues(n, nil, nil, item, "w"), "\n")
	}
	if got := issues(node(""), editable); got != "" {
		t.Errorf("an update Form inside a Collection of an editable Machine was refused: %s", got)
	}
	if got := issues(node(""), nil); !strings.Contains(got, "acts on a record") {
		t.Errorf("an update Form outside a records Collection: %q", got)
	}
	if got := issues(node("ds_other"), editable); !strings.Contains(got, "takes no dataset") {
		t.Errorf("an update Form naming a dataset: %q", got)
	}
	if got := issues(node(""), boolOnly); !strings.Contains(got, "no Field an edit form can ask for") {
		t.Errorf("an update Form over a Machine with only booleans: %q", got)
	}
}

func TestDeleteBindingIssues(t *testing.T) {
	deletable := &domain.Machine{ID: "mch_x"}
	logOnly := &domain.Machine{ID: "mch_log", AppendOnly: true}
	node := func(dataset string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: "Button", Binding: &domain.PageBinding{Dataset: dataset, Write: domain.PageWriteDelete}}
	}
	issues := func(n domain.PageNode, item *domain.Machine) string {
		return strings.Join(bindingIssues(n, nil, nil, item, "w"), "\n")
	}
	if got := issues(node(""), deletable); got != "" {
		t.Errorf("a delete Button inside a Collection of a deletable Machine was refused: %s", got)
	}
	if got := issues(node(""), nil); !strings.Contains(got, "acts on a record") {
		t.Errorf("a delete Button outside a records Collection: %q", got)
	}
	if got := issues(node("ds_other"), deletable); !strings.Contains(got, "takes no dataset") {
		t.Errorf("a delete Button naming a dataset: %q", got)
	}
	if got := issues(node(""), logOnly); !strings.Contains(got, "append-only") {
		t.Errorf("a delete Button over an append-only Machine: %q", got)
	}
}

func TestMoveBindingIssues(t *testing.T) {
	movable := &domain.Machine{ID: "mch_x"}
	logOnly := &domain.Machine{ID: "mch_log", AppendOnly: true}
	node := func(dataset string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: "Button", Binding: &domain.PageBinding{Dataset: dataset, Write: domain.PageWriteMove}}
	}
	issues := func(n domain.PageNode, item *domain.Machine) string {
		return strings.Join(bindingIssues(n, nil, nil, item, "w"), "\n")
	}
	if got := issues(node(""), movable); got != "" {
		t.Errorf("a move Button inside a Collection of a movable Machine was refused: %s", got)
	}
	if got := issues(node(""), nil); !strings.Contains(got, "acts on a record") {
		t.Errorf("a move Button outside a records Collection: %q", got)
	}
	if got := issues(node("ds_other"), movable); !strings.Contains(got, "takes no dataset") {
		t.Errorf("a move Button naming a dataset: %q", got)
	}
	if got := issues(node(""), logOnly); !strings.Contains(got, "append-only") {
		t.Errorf("a move Button over an append-only Machine: %q", got)
	}
}

// A move changes the Machine's own order, so the list it sits in must be that order: a Dataset with a `where:` or a
// `sort:` shows a neighbour the move would not step over.
func TestRecordsBindingRefusesAMoveOverAFilteredOrSortedDataset(t *testing.T) {
	coll := domain.PageNode{Kind: "component", Type: "Collection",
		Binding:  &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "component", Type: "Button", Binding: &domain.PageBinding{Write: domain.PageWriteMove}}}}
	m := &domain.Machine{ID: "mch_x"}
	plain := domain.Dataset{ID: "ds_a", Select: domain.SelectRecords}
	sorted := plain
	sorted.Sort = []domain.SortKey{{Field: "created_at"}}
	run := func(ds domain.Dataset) string {
		return strings.Join(bindingIssues(coll, map[string]domain.Dataset{"ds_a": ds}, map[string]*domain.Machine{"mch_x": m}, nil, "w"), "\n")
	}
	ds := func(d domain.Dataset) domain.Dataset { d.Source = "mch_x"; return d }
	if got := run(ds(plain)); strings.Contains(got, "Machine's own order") {
		t.Errorf("a plain Dataset was refused: %s", got)
	}
	if got := run(ds(sorted)); !strings.Contains(got, "Machine's own order") {
		t.Errorf("a sorted Dataset under a move was accepted: %q", got)
	}
}

func TestTransitionBindingIssues(t *testing.T) {
	task := &domain.Machine{
		ID: "mch_x",
		Fields: []domain.Field{
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}, Default: "todo"},
		},
		CardFields: []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}},
		Completion: &domain.Completion{Field: "fld_status", Done: "done"},
	}
	node := func(dataset, becomes string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: "Button",
			Props:   map[string]string{"becomes": becomes},
			Binding: &domain.PageBinding{Dataset: dataset, Write: domain.PageWriteTransition}}
	}
	issues := func(n domain.PageNode, item *domain.Machine) string {
		return strings.Join(bindingIssues(n, nil, nil, item, "w"), "\n")
	}
	for _, becomes := range []string{domain.PageTransitionDone, domain.PageTransitionReopen, "todo"} {
		if got := issues(node("", becomes), task); got != "" {
			t.Errorf("becomes %q was refused: %s", becomes, got)
		}
	}
	if got := issues(node("", "done"), nil); !strings.Contains(got, "acts on a record") {
		t.Errorf("outside a records Collection: %q", got)
	}
	if got := issues(node("ds_other", "done"), task); !strings.Contains(got, "takes no dataset") {
		t.Errorf("naming a dataset: %q", got)
	}
	if got := issues(node("", "blocked"), task); !strings.Contains(got, "not an option") {
		t.Errorf("a target that is no option: %q", got)
	}
	logOnly := *task
	logOnly.AppendOnly = true
	if got := issues(node("", "done"), &logOnly); !strings.Contains(got, "append-only") {
		t.Errorf("an append-only Machine: %q", got)
	}
	noCompletion := *task
	noCompletion.Completion = nil
	for _, becomes := range []string{domain.PageTransitionDone, domain.PageTransitionReopen} {
		if got := issues(node("", becomes), &noCompletion); !strings.Contains(got, "completion") {
			t.Errorf("%s over a Machine with no completion: %q", becomes, got)
		}
	}
	noStatus := *task
	noStatus.CardFields = nil
	if got := issues(node("", "done"), &noStatus); !strings.Contains(got, "card_fields") {
		t.Errorf("a Machine with no status role: %q", got)
	}
}

func TestConditionIssues(t *testing.T) {
	task := &domain.Machine{
		ID: "mch_x",
		Fields: []domain.Field{
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}, Default: "todo"},
			{ID: "fld_title", Type: domain.FieldTypeText},
		},
		CardFields: []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}, {Field: "fld_title", Role: domain.CardFieldRoleTitle}},
		Completion: &domain.Completion{Field: "fld_status", Done: "done"},
	}
	issues := func(c domain.PageCondition, item *domain.Machine) string {
		return strings.Join(conditionIssues(c, item, "w"), "\n")
	}
	for _, c := range []domain.PageCondition{
		{Role: "status", Is: domain.PageTransitionDone},
		{Role: "status", IsNot: domain.PageTransitionReopen},
		{Role: "status", Is: "todo"},
		{Role: "title", Is: "anything: a Field with no options takes any literal"},
	} {
		if got := issues(c, task); got != "" {
			t.Errorf("%+v was refused: %s", c, got)
		}
	}
	for name, tc := range map[string]struct {
		c    domain.PageCondition
		item *domain.Machine
		want string
	}{
		"outside a records Collection": {domain.PageCondition{Role: "status", Is: "todo"}, nil, "item template"},
		"role the Machine lacks":       {domain.PageCondition{Role: "date", Is: "x"}, task, "no card_fields"},
		"misspelt option":              {domain.PageCondition{Role: "status", Is: "finished"}, task, "not an option"},
		"sentinel over another Field":  {domain.PageCondition{Role: "title", Is: domain.PageTransitionDone}, task, "reads fld_title"},
	} {
		if got := issues(tc.c, tc.item); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want it to mention %q", name, got, tc.want)
		}
	}
	noCompletion := *task
	noCompletion.Completion = nil
	if got := issues(domain.PageCondition{Role: "status", Is: domain.PageTransitionDone}, &noCompletion); !strings.Contains(got, "completion") {
		t.Errorf("a sentinel over a Machine with no completion: %q", got)
	}
}

// TestBindingIssuesWalksIntoWhen: the walk, and not only the helper, asks every `when:` -- a misspelt status under
// a records Collection must be a load error.
func TestBindingIssuesWalksIntoWhen(t *testing.T) {
	task := &domain.Machine{
		ID:         "mch_x",
		Fields:     []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}}},
		CardFields: []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}},
	}
	ds := map[string]domain.Dataset{"ds_x": {ID: "ds_x", Source: "mch_x", Select: domain.SelectRecords}}
	page := func(is string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: "Collection",
			Binding: &domain.PageBinding{Dataset: "ds_x", Rows: domain.PageRowsRecords},
			Children: []domain.PageNode{{Kind: "static", Type: "text", Props: map[string]string{"text": "t"},
				When: &domain.PageCondition{Role: "status", Is: is}}}}
	}
	machines := map[string]*domain.Machine{"mch_x": task}
	if got := bindingIssues(page("done"), ds, machines, nil, "w"); len(got) != 0 {
		t.Errorf("a real option was refused: %v", got)
	}
	if got := strings.Join(bindingIssues(page("finished"), ds, machines, nil, "w"), "\n"); !strings.Contains(got, "not an option") {
		t.Errorf("a misspelt option reached no check: %q", got)
	}
}

func eachPage(inTemplate bool, each string) string {
	tag := "            - component: Tag\n              each: " + each + "\n              from: { label: title, color: color }\n"
	if !inTemplate {
		return "  - id: nav_e\n    label: E\n    route: /pages/nav_e\n    page:\n      layout: stack\n      children:\n        - component: Tag\n          each: " + each + "\n          from: { label: title, color: color }\n"
	}
	return "  - id: nav_e\n    label: E\n    route: /pages/nav_e\n    page:\n      layout: stack\n      children:\n        - component: Collection\n          gap: tight\n          binding: { dataset: ds_rows, rows: records }\n          children:\n" + tag
}

// TestPage_eachTagsNeedsAMachineThatDeclaresCardTags: `each: tags` is valid only inside a records template over a
// Machine with `card_tags:`; the fixture's Machine has none, so an otherwise well-formed page is refused naming
// why, and so is one outside a template, one with a list that is not tags, and an empty value.
func TestPage_eachTagsNeedsAMachineThatDeclaresCardTags(t *testing.T) {
	for name, c := range map[string]struct{ nav, want string }{
		"a Machine with no card_tags": {eachPage(true, "tags"), "declares no card_tags"},
		"outside a template":          {eachPage(false, "tags"), "item template"},
		"a list that is not tags":     {eachPage(true, "comments"), "not a list"},
	} {
		err := pageFixture(t, c.nav)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v; want it to mention %q", name, err, c.want)
		}
	}
	if err := pageFixture(t, strings.Replace(eachPage(true, "tags"), "each: tags", "each: ", 1)); err == nil {
		t.Error("an empty each: loaded")
	}
}

// A Collection bound to a Dimension's rows draws one item per value, its template reading the value and the
// Dataset's own Measures. The Measure ids are checked at load: one the Dataset does not declare fails as a
// mistyped role would, and the bound Collection itself names no measure and needs a Dimension to expand.
func TestPage_aDimensionCollectionReadsTheDatasetsOwnMeasures(t *testing.T) {
	page := func(dataset, measure, max string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n" +
			"        - component: Collection\n          gap: tight\n          binding: { dataset: " + dataset + measure + ", rows: dimension }\n" +
			"          children:\n            - component: Progress\n              from: { label: \"row:label\", value: \"measure:msr_total\", max: \"" + max + "\" }\n"
	}
	if err := pageFixture(t, page("ds_by_status", "", "measure:msr_total")); err != nil {
		t.Fatalf("a dimension Collection of Progress bars over declared Measures was refused: %v", err)
	}
	for name, tc := range map[string]struct{ nav, want string }{
		"an undeclared measure":       {page("ds_by_status", "", "measure:msr_nope"), "msr_nope"},
		"a dataset with no dimension": {page("ds_flat", "", "measure:msr_total"), "no dimension"},
		"a measure on the list":       {page("ds_by_status", ", measure: msr_total", "measure:msr_total"), "takes no measure"},
		"a non-number into Progress":  {page("ds_by_status", "", "row:label"), "not a number"},
	} {
		if err := pageFixture(t, tc.nav); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want one mentioning %q", name, err, tc.want)
		}
	}
}

// The load-time total answers any Measure id, so hint_figure's Measure is checked against the Dataset itself.
func TestPage_aHintFigureNamesAMeasureOfTheBoundDataset(t *testing.T) {
	page := func(of string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n" +
			"        - component: Metric\n          label: All\n          hint_figure: { of: " + of + ", other: \"{n} in all\" }\n" +
			"          binding: { dataset: ds_by_status, measure: msr_total, rows: total }\n"
	}
	if err := pageFixture(t, page("msr_total")); err != nil {
		t.Fatalf("a hint_figure over a declared Measure was refused: %v", err)
	}
	if err := pageFixture(t, page("msr_nope")); err == nil || !strings.Contains(err.Error(), "msr_nope") {
		t.Errorf("err = %v, want the undeclared Measure named", err)
	}
}

func groupedPage(dataset, group string) string {
	return `
  - id: nav_week
    label: Week
    route: /pages/nav_week
    page:
      layout: grid
      columns: "7"
      mobile: "1"
      children:
        - component: Collection
          gap: tight
          empty: none
          binding: { dataset: ` + dataset + `, rows: records, group: ` + group + ` }
          children:
            - static: subheading
              from: { text: "bucket:label" }
            - static: paragraph
              from: { text: title }
`
}

// `group: date` asks two things of what it groups: the Machine names a Field to bucket by, and the Dataset closes
// a window on it with one gte and one lte -- the days drawn are read from those bounds, so a window that cannot
// be read has to stop the load and not draw an empty calendar.
func TestPage_aGroupedListNeedsAWindowItCanRead(t *testing.T) {
	if err := pageFixture(t, groupedPage("ds_week", "date")); err != nil {
		t.Fatalf("a grouped list over a closed week was refused: %v", err)
	}
	// Bounds on another Field are that Field's, not a second window on the one grouped by.
	if err := pageFixture(t, groupedPage("ds_week_beside_another", "date")); err != nil {
		t.Fatalf("a window beside another Field's bounds was refused: %v", err)
	}
	for name, c := range map[string]struct{ dataset, group, want string }{
		"no upper bound":    {"ds_week_open", "date", "exactly one gte and one lte"},
		"two lower bounds":  {"ds_week_twice", "date", "exactly one gte and one lte"},
		"bounds in an any:": {"ds_week_any", "date", "all-only"},
		"no window at all":  {"ds_rows", "date", "all-only"},
		"another dimension": {"ds_week", "status", "not a dimension"},
	} {
		err := pageFixture(t, groupedPage(c.dataset, c.group))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error mentioning %q, got %v", name, c.want, err)
		}
	}
}

func TestPage_aGroupNeedsTheMachineToNameADateRole(t *testing.T) {
	nav := strings.Replace(groupedPage("ds_week", "date"), "label: Week", "label: Undated", 1)
	err := pageFixture(t, nav)
	if err == nil || !strings.Contains(err.Error(), "declares no card_fields role") {
		t.Errorf("want a refusal naming the missing date role, got %v", err)
	}
}

// The bucket's name is a role of the heading template only: a record's template reading it is a lowering error,
// and it is never a card_fields role a Machine could be asked for.
func TestPage_aBucketNameIsNotAProjectionRole(t *testing.T) {
	bad := strings.Replace(groupedPage("ds_week", "date"), "from: { text: title }", `from: { text: "bucket:label" }`, 1)
	if err := pageFixture(t, bad); err == nil {
		t.Error("a record's template read the bucket's name")
	}
	ungrouped := strings.Replace(validListPage, "from: { text: title }", `from: { text: "bucket:label" }`, 1)
	if err := pageFixture(t, ungrouped); err == nil {
		t.Error("an ungrouped list read the bucket's name")
	}
}

func weekToolbar(link, caption, when string) string {
	return `
  - id: nav_week
    label: Week
    route: /pages/nav_week
    page:
      layout: stack
      children:
        - static: link
          to: nav_week
          text: Next
` + link + `
        - static: caption
` + caption + `
        - static: paragraph
          text: back
` + when + `
        - component: Collection
          gap: tight
          empty: none
          binding: { dataset: ds_week, rows: records, group: date }
          children:
            - static: subheading
              from: { text: "bucket:label" }
            - static: paragraph
              from: { text: title }
`
}

const (
	okStep    = "          step: { parameter: week, by: 1 }"
	okWindow  = "          window: ds_week"
	okWhenKey = "          when: { parameter: week, is_not: \"0\" }"
)

// `step:`, `window:` and `when: {parameter: ...}` are the week toolbar's three keys. Each is checked at load for what
// ir.Lower cannot see: that the Dataset exists and closes a readable window, and that a stepped link lands on a page
// whose Datasets read $week -- a link that moves a parameter nothing reads draws and selects the same days.
func TestPage_aWeekToolbarLoadsAndEachKeyIsCheckedAtLoad(t *testing.T) {
	if err := pageFixture(t, weekToolbar(okStep, okWindow, okWhenKey)); err != nil {
		t.Fatalf("a well-formed week toolbar was refused: %v", err)
	}
	for name, c := range map[string]struct{ step, window, when, want string }{
		"a window on a dataset nobody declares": {okStep, "          window: ds_nope", okWhenKey, "ds_nope"},
		"a window with no closed range":         {okStep, "          window: ds_week_open", okWhenKey, "exactly one gte and one lte"},
		"a step to a page that reads no $week":  {okStep, okWindow, okWhenKey, "reading $week"},
		"a step by zero":                        {"          step: { parameter: week, by: 0 }", okWindow, okWhenKey, "not zero"},
		"a step by a word":                      {"          step: { parameter: week, by: soon }", okWindow, okWhenKey, "whole number"},
		"a step with an unknown key":            {"          step: { parameter: week, by: 1, wrap: true }", okWindow, okWhenKey, "wrap"},
		"a step on a parameter nothing names":   {"          step: { parameter: page, by: 1 }", okWindow, okWhenKey, "week"},
		"a window that is a list":               {okStep, "          window: [ds_week]", okWhenKey, "window"},
		"a when with a role and a parameter":    {okStep, okWindow, "          when: { parameter: week, role: status, is_not: \"0\" }", "not both"},
		"a when comparing a week to a word":     {okStep, okWindow, "          when: { parameter: week, is_not: soon }", "whole number"},
	} {
		nav := weekToolbar(c.step, c.window, c.when)
		if strings.HasPrefix(name, "a step to a page") {
			nav = "  - id: nav_other\n    label: Other\n    route: /other\n" + strings.Replace(nav, "to: nav_week", "to: nav_other", 1)
		}
		err := pageFixture(t, nav)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error mentioning %q, got %v", name, c.want, err)
		}
	}
}

// A node a request-scoped `when:` would hide is still validated at load, because the load-time lowering holds every
// condition: a "This week" link that names a missing destination must stop the load on the week it happens to be
// hidden in, not be found the first time someone steps away from this week.
func TestPage_aNodeHiddenByAParameterConditionIsStillChecked(t *testing.T) {
	nav := strings.Replace(weekToolbar(okStep, okWindow, okWhenKey), "- static: paragraph\n          text: back", "- static: link\n          to: nav_missing", 1)
	if err := pageFixture(t, nav); err == nil || !strings.Contains(err.Error(), "nav_missing") {
		t.Errorf("a guarded link to a missing destination loaded: %v", err)
	}
}

// TestPage_aDisclosureLoadsWithABodyAndABlankSummaryIsRefused: a page may declare `component: Disclosure` with a summary
// and children in its `body` slot; a blank or missing summary and any undeclared property are load errors.
func TestPage_aDisclosureLoadsWithABodyAndABlankSummaryIsRefused(t *testing.T) {
	page := func(props string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - component: Disclosure\n" + props
	}
	body := "          children:\n            - static: message\n              text: Inside.\n"
	if err := pageFixture(t, page("          summary: More\n"+body)); err != nil {
		t.Errorf("a valid Disclosure was refused: %v", err)
	}
	for name, c := range map[string]struct{ props, want string }{
		"no summary":      {body, "summary"},
		"a blank summary": {"          summary: \"  \"\n" + body, "blank"},
		"a class":         {"          summary: More\n          class: mt-2\n" + body, "class"},
	} {
		err := pageFixture(t, page(c.props))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error naming %q", name, err, c.want)
		}
	}
}

// TestPage_aSegmentedControlLoadsAndABadOneIsRefused: a page may declare `component: SegmentedControl` with its four
// inputs; a lone option, a selected value outside the options, a bad parameter name and any undeclared property are
// load errors.
func TestPage_aSegmentedControlLoadsAndABadOneIsRefused(t *testing.T) {
	page := func(label, query, options, selected, extra string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - component: SegmentedControl\n" +
			"          label: " + label + "\n          query: " + query + "\n          options: |-\n" + options + "          selected: " + selected + "\n" + extra
	}
	two := "            week=Week\n            month=Month\n"
	if err := pageFixture(t, page("Range", "range", two, "week", "")); err != nil {
		t.Errorf("a valid SegmentedControl was refused: %v", err)
	}
	for name, c := range map[string]struct{ doc, want string }{
		"one option":        {page("Range", "range", "            week=Week\n", "week", ""), "fewer than two"},
		"selected not in":   {page("Range", "range", two, "year", ""), "selected"},
		"a bad query":       {page("Range", "Range", two, "week", ""), "query"},
		"a blank label":     {page("\"  \"", "range", two, "week", ""), "label"},
		"an undeclared key": {page("Range", "range", two, "week", "          class: mt-2\n"), "class"},
	} {
		err := pageFixture(t, c.doc)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error naming %q", name, err, c.want)
		}
	}
}

// TestPage_aPopoverLoadsWithABodyAndABadOneIsRefused: a page may declare `component: Popover` with its inputs and children
// in its `body` slot; a missing input, a value outside a closed set, a glyph with no title and any undeclared property are
// load errors.
func TestPage_aPopoverLoadsWithABodyAndABadOneIsRefused(t *testing.T) {
	page := func(props string) string {
		return "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n      layout: stack\n      children:\n        - component: Popover\n" + props
	}
	body := "          children:\n            - static: message\n              text: Inside.\n"
	full := "          label: More\n          trigger: secondary\n          align: start\n          density: compact\n"
	if err := pageFixture(t, page(full+body)); err != nil {
		t.Errorf("a valid Popover was refused: %v", err)
	}
	for name, c := range map[string]struct{ props, want string }{
		"no label":      {"          trigger: secondary\n          align: start\n          density: compact\n" + body, "label"},
		"a bad trigger": {"          label: More\n          trigger: primary\n          align: start\n          density: compact\n" + body, "trigger"},
		"a bad align":   {"          label: More\n          trigger: secondary\n          align: left\n          density: compact\n" + body, "align"},
		"a glyph":       {"          label: \"…\"\n          trigger: secondary\n          align: start\n          density: compact\n" + body, "glyph"},
		"a class":       {full + "          class: mt-2\n" + body, "class"},
	} {
		err := pageFixture(t, page(c.props))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error naming %q", name, err, c.want)
		}
	}
}

const validTablePage = `
  - id: nav_table
    label: Table
    route: /pages/nav_table
    page:
      layout: stack
      children:
        - component: Table
          label: Docs
          empty: Nothing yet.
          columns:
            - { header: Title, from: title, href: record }
            - { header: Status, from: status }
            - { header: Colour, from: color, align: end }
          binding: { dataset: ds_rows, rows: records }
`

func TestPage_aTableLoadsAndItsColumnsReachTheTree(t *testing.T) {
	if err := pageFixture(t, validTablePage); err != nil {
		t.Fatalf("a valid Table page was refused: %v", err)
	}
	var doc pageNodeDoc
	if err := yaml.Unmarshal([]byte("component: Table\nlabel: T\ncolumns:\n  - { header: A, from: title, align: end, href: record }\nbinding: { dataset: ds_rows, rows: records }\n"), &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.toDomain().Columns
	if len(got) != 1 || got[0] != (domain.PageColumn{Header: "A", From: "title", Align: "end", Href: "record"}) {
		t.Errorf("columns = %+v", got)
	}
}

// A grid's `columns: 4` is a scalar and a Table's `columns:` is a list: the same key, told apart by shape.
func TestPage_columnsIsANumberOnAGridAndAListOnATable(t *testing.T) {
	var grid pageNodeDoc
	if err := yaml.Unmarshal([]byte("layout: grid\ncolumns: 4\nchildren:\n  - static: paragraph\n    text: x\n"), &grid); err != nil {
		t.Fatal(err)
	}
	if g := grid.toDomain(); g.Props["columns"] != "4" || len(g.Columns) != 0 {
		t.Errorf("a grid's columns = props %v, columns %v", g.Props, g.Columns)
	}
	for name, src := range map[string]string{
		"a mapping":       "component: Table\ncolumns: { header: A }\n",
		"an empty list":   "component: Table\ncolumns: []\n",
		"a scalar column": "component: Table\ncolumns:\n  - A\n",
		"an unknown key":  "component: Table\ncolumns:\n  - { header: A, from: title, width: 3 }\n",
		"a nested value":  "component: Table\ncolumns:\n  - { header: A, from: [title] }\n",
	} {
		var doc pageNodeDoc
		if err := yaml.Unmarshal([]byte(src), &doc); err == nil {
			t.Errorf("%s: the declaration was accepted", name)
		}
	}
}

func TestPage_eachTableMisuseIsALoadError(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"a role the machine does not project": {"from: title, href: record }\n            - { header: Start, from: owner2", "from: title, href: record }\n            - { header: Start, from: date", "no card_fields role"},
		"a dataset that is an aggregate":      {"dataset: ds_rows", "dataset: ds_flat", "aggregate"},
		"a column with no header":             {"{ header: Status, from: status }", "{ header: \"\", from: status }", "header"},
		"a dimension binding":                 {"rows: records }", "rows: dimension }", "a dimension binding needs an aggregate dataset"},
	} {
		nav := strings.Replace(validTablePage, tc.from, tc.to, 1)
		if name == "a role the machine does not project" {
			nav = strings.Replace(validTablePage, "{ header: Title, from: title, href: record }", "{ header: Title, from: title, href: record }\n            - { header: Start, from: date }", 1)
		}
		err := pageFixture(t, nav)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
	if err := pageFixture(t, strings.Replace(validTablePage, "          binding: { dataset: ds_rows, rows: records }\n", "", 1)); err == nil {
		t.Error("a Table with no binding was accepted")
	}
}

// workspacePageFixture loads a Workspace whose manifest declares `wsNav` as its own navigation, over one Machine with
// a records Dataset and an aggregate one, and one Application whose menu has an item a Workspace page may link to.
func workspacePageFixture(t *testing.T, wsNav string) error {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "doc.yaml", `
id: mch_doc
name: Doc
fields:
  - id: fld_title
    name: Title
    type: text
  - id: fld_status
    name: Status
    type: status
    options: [draft, approved]
card_fields:
  - { field: fld_title, role: title }
  - { field: fld_status, role: status }
datasets:
  - id: ds_rows
    select: records
    limit: 10
  - id: ds_figures
    measures:
      - id: msr_total
        aggregate: count
`)
	writeFile(t, dir, "app.yaml", "\nworkspace: default\nmachines:\n  - doc.yaml\nnavigation:\n"+wsNav+"applications:\n  - app-main.yaml\n")
	writeFile(t, dir, "app-main.yaml", "\nid: app_docs\nname: Docs\nmachines:\n  - mch_doc\nnavigation:\n  - { id: nav_docs, label: Docs, route: /machines/mch_doc }\n")
	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	return err
}

const validWorkspacePage = `  - id: nav_ws_overview
    label: Overview
    route: /pages/nav_ws_overview
    page:
      layout: stack
      children:
        - component: Metric
          label: Docs
          binding: { dataset: ds_figures, measure: msr_total, rows: total }
        - static: link
          to: nav_docs
        - component: Table
          label: Docs
          columns:
            - { header: Title, from: title }
          binding: { dataset: ds_rows, rows: records }
`

func TestWorkspacePage_aDeclaredWorkspacePageLoadsAndItsLinksReachAnApplication(t *testing.T) {
	if err := workspacePageFixture(t, validWorkspacePage); err != nil {
		t.Fatalf("a valid Workspace page was refused: %v", err)
	}
}

func TestWorkspacePage_isHeldToTheSameRulesAsAnApplicationsPage(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"a dataset nobody declares":           {"dataset: ds_figures", "dataset: ds_nobody", "no machine in this workspace declares"},
		"a link to an item nobody declares":   {"to: nav_docs", "to: nav_nobody", "nav_nobody"},
		"a route that is not /pages/<id>":     {"route: /pages/nav_ws_overview", "route: /overview", "rendered at"},
		"a root that is not a layout":         {"      layout: stack\n      children:\n", "      component: Metric\n      children:\n", "root must be a layout"},
		"a role the machine does not project": {"{ header: Title, from: title }", "{ header: Title, from: owner }", "declares no card_fields role"},
		"a Table over an aggregate dataset":   {"dataset: ds_rows", "dataset: ds_figures", "aggregate"},
	} {
		err := workspacePageFixture(t, strings.Replace(validWorkspacePage, tc.from, tc.to, 1))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

func TestWorkspacePage_openToIsReadAndHeldToItsRules(t *testing.T) {
	for _, who := range []string{"admin", "member"} {
		ws := strings.Replace(validWorkspacePage, "    page:\n", "    open_to: "+who+"\n    page:\n", 1)
		if err := workspacePageFixture(t, ws); err != nil {
			t.Errorf("open_to: %s was refused: %v", who, err)
		}
	}
	for name, tc := range map[string]struct{ from, to string }{
		"a word that is no role":  {"    page:\n", "    open_to: everyone\n    page:\n"},
		"a negation":              {"    page:\n", "    open_to: not_admin\n    page:\n"},
		"on an item with no page": {"    page:\n", "    open_to: member\n    xpage:\n"},
	} {
		ws := strings.Replace(validWorkspacePage, tc.from, tc.to, 1)
		if name == "on an item with no page" { // an item that declares no page at all
			ws = "  - id: nav_ws_plain\n    label: Plain\n    route: /somewhere\n    open_to: member\n"
		}
		if err := workspacePageFixture(t, ws); err == nil || !strings.Contains(err.Error(), "open_to") {
			t.Errorf("%s: %v, want an open_to error", name, err)
		}
	}
}

func TestPage_anApplicationsOwnPageTakesNoOpenTo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.yaml", "\nid: mch_doc\nname: Doc\nfields:\n  - { id: fld_title, name: Title, type: text }\ncard_fields:\n  - { field: fld_title, role: title }\n")
	writeFile(t, dir, "app.yaml", "\nworkspace: default\nmachines:\n  - doc.yaml\napplications:\n  - app-main.yaml\n")
	writeFile(t, dir, "app-main.yaml", "\nid: app_docs\nname: Docs\nmachines:\n  - mch_doc\nnavigation:\n  - { id: nav_docs, label: Docs, route: /machines/mch_doc, open_to: member }\n")
	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err == nil || !strings.Contains(err.Error(), "open_to") {
		t.Errorf("an Application's navigation item with open_to: %v, want an open_to error", err)
	}
}

func TestPage_aViewerConditionIsReadAndHeldToItsRules(t *testing.T) {
	var doc pageNodeDoc
	if err := yaml.Unmarshal([]byte("static: paragraph\ntext: x\nwhen: { viewer: workspace_role, is: admin }\n"), &doc); err != nil {
		t.Fatal(err)
	}
	if w := doc.toDomain().When; w == nil || w.Viewer != "workspace_role" || w.Is != "admin" {
		t.Errorf("the condition = %+v", w)
	}
	for name, src := range map[string]string{
		"a viewer and a role":      "static: paragraph\nwhen: { viewer: workspace_role, role: status, is: admin }\n",
		"a viewer and a parameter": "static: paragraph\nwhen: { viewer: workspace_role, parameter: week, is: admin }\n",
		"no operand":               "static: paragraph\nwhen: { viewer: workspace_role }\n",
		"both operands":            "static: paragraph\nwhen: { viewer: workspace_role, is: admin, is_not: member }\n",
	} {
		var d pageNodeDoc
		if err := yaml.Unmarshal([]byte(src), &d); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Through the loader: a negation and a role that is not a Workspace role are load errors, on an Application's page and
	// on a Workspace's.
	app := strings.Replace(validListPage, "static: paragraph\n                  from: { text: title }", "static: paragraph\n                  from: { text: title }\n                  when: { viewer: workspace_role, is_not: member }", 1)
	if err := pageFixture(t, app); err == nil || !strings.Contains(err.Error(), "never is_not") {
		t.Errorf("a negated viewer condition on an Application page: %v", err)
	}
	ws := strings.Replace(validWorkspacePage, "        - static: link\n          to: nav_docs", "        - static: link\n          to: nav_docs\n          when: { viewer: workspace_role, is: owner }", 1)
	if err := workspacePageFixture(t, ws); err == nil || !strings.Contains(err.Error(), "not a Workspace role") {
		t.Errorf("an unknown Workspace role on a Workspace page: %v", err)
	}
}

func TestPage_aValidViewerConditionLoadsOnAnApplicationsPageAndOnAWorkspacesOwn(t *testing.T) {
	app := strings.Replace(validListPage, "static: paragraph\n                  from: { text: title }", "static: paragraph\n                  from: { text: title }\n                  when: { viewer: workspace_role, is: admin }", 1)
	if err := pageFixture(t, app); err != nil {
		t.Errorf("a viewer condition on an Application page was refused: %v", err)
	}
	ws := strings.Replace(validWorkspacePage, "        - static: link\n          to: nav_docs", "        - static: link\n          to: nav_docs\n          when: { viewer: workspace_role, is: member }", 1)
	if err := workspacePageFixture(t, ws); err != nil {
		t.Errorf("a viewer condition on a Workspace page was refused: %v", err)
	}
}

// An Application's page may link to the Workspace's own pages and to its own menu, and to nothing in another Application.
func TestPage_anApplicationPageMayLinkToTheWorkspacesPagesButNotToAnotherApplication(t *testing.T) {
	load := func(to string) error {
		dir := t.TempDir()
		writeFile(t, dir, "doc.yaml", "id: mch_doc\nname: Doc\nfields:\n  - id: fld_title\n    name: Title\n    type: text\n")
		writeFile(t, dir, "other.yaml", "id: mch_other\nname: Other\nfields:\n  - id: fld_title\n    name: Title\n    type: text\n")
		writeFile(t, dir, "app.yaml", "\nworkspace: default\nmachines:\n  - doc.yaml\n  - other.yaml\nnavigation:\n  - id: nav_ws_overview\n    label: Overview\n    route: /pages/nav_ws_overview\n    page:\n      layout: stack\n      children:\n        - static: paragraph\n          text: hi\napplications:\n  - app-main.yaml\n  - app-other.yaml\n")
		writeFile(t, dir, "app-main.yaml", "\nid: app_docs\nname: Docs\nmachines:\n  - mch_doc\nnavigation:\n  - id: nav_docs\n    label: Docs\n    route: /pages/nav_docs\n    page:\n      layout: stack\n      children:\n        - static: link\n          to: "+to+"\n")
		writeFile(t, dir, "app-other.yaml", "\nid: app_other\nname: Other\nmachines:\n  - mch_other\nnavigation:\n  - { id: nav_other, label: Other, route: /machines/mch_other }\n")
		_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
		return err
	}
	for _, ok := range []string{"nav_docs", "nav_ws_overview"} {
		if err := load(ok); err != nil {
			t.Errorf("a link to %s was refused: %v", ok, err)
		}
	}
	if err := load("nav_other"); err == nil || !strings.Contains(err.Error(), "nav_other") {
		t.Errorf("a link into another Application: %v, want a refusal naming it", err)
	}
}

// TestTableColumnsNameFieldsAndDerivedRoles: a Table column may name any Field of the Dataset's Machine (a relation
// too, resolved by the ids listed), and a derived role is a column like any other -- but a Field the Machine does not
// have, a group Field, and a derived role the Machine has nothing to derive it from are refused at load, each by name.
func TestTableColumnsNameFieldsAndDerivedRoles(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Fields: []domain.Field{
		{ID: "fld_title", Type: domain.FieldTypeText},
		{ID: "fld_party", Type: domain.FieldTypeRelation, RelatedMachine: "mch_party"},
		{ID: "fld_team", Type: domain.FieldTypeGroup},
	}, CardFields: []domain.CardField{{Field: "fld_title", Role: domain.CardFieldRoleTitle}}}
	ds := domain.Dataset{ID: "ds_a", Source: "mch_x", Select: domain.SelectRecords}
	run := func(cols ...domain.PageColumn) string {
		table := domain.PageNode{Kind: "component", Type: "Table", Columns: cols,
			Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords}}
		return strings.Join(bindingIssues(table, map[string]domain.Dataset{"ds_a": ds}, map[string]*domain.Machine{"mch_x": m}, nil, "w"), "\n")
	}
	if got := run(domain.PageColumn{Header: "T", Field: "fld_title"}, domain.PageColumn{Header: "P", Field: "fld_party"},
		domain.PageColumn{Header: "C", From: domain.PageCreatedLabelRole}, domain.PageColumn{Header: "Title", From: "title"}); got != "" {
		t.Errorf("fields, a relation and a derived role were refused: %s", got)
	}
	for name, tc := range map[string]struct {
		col  domain.PageColumn
		want string
	}{
		"an unknown Field":         {domain.PageColumn{Header: "X", Field: "fld_nope"}, "has no such Field"},
		"a group Field":            {domain.PageColumn{Header: "X", Field: "fld_team"}, "group Field"},
		"a status tone, no status": {domain.PageColumn{Header: "X", From: domain.PageStatusToneRole}, "no card_fields role \"status\""},
		"a date, no date role":     {domain.PageColumn{Header: "X", From: domain.PageDateLabelRole}, "no card_fields role \"date\""},
		"an undeclared role":       {domain.PageColumn{Header: "X", From: "subtitle"}, "declares no card_fields role \"subtitle\""},
	} {
		if got := run(tc.col); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want it to say %q", name, got, tc.want)
		}
	}
}

// TestOptionalAndContainsAreHeldToTheirMeaning: `optional` belongs to a request parameter on a records Dataset, and
// `contains` to a text Field with words to look for -- each misuse refused by name.
func TestOptionalAndContainsAreHeldToTheirMeaning(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}, {ID: "fld_value", Type: domain.FieldTypeMoney}}}
	run := func(ds domain.Dataset) string {
		ds.ID, ds.Source = "ds_x", "mch_x"
		fields := map[string]domain.Field{}
		for _, f := range m.Fields {
			fields[f.ID] = f
		}
		return strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "\n")
	}
	records := func(c expression.Comparison) domain.Dataset {
		return domain.Dataset{Select: domain.SelectRecords, Limit: 10, Where: &expression.Predicate{All: []expression.Comparison{c}}}
	}
	if got := run(records(expression.Comparison{Field: "fld_name", Op: expression.OpContains, Value: "$parameters.q", Optional: true})); got != "" {
		t.Errorf("an optional search was refused: %s", got)
	}
	for name, tc := range map[string]struct {
		ds   domain.Dataset
		want string
	}{
		"optional on a literal":           {records(expression.Comparison{Field: "fld_name", Op: expression.OpEquals, Value: "x", Optional: true}), "must be a $parameters."},
		"contains on money":               {records(expression.Comparison{Field: "fld_value", Op: expression.OpContains, Value: "1"}), "looks for words in text"},
		"contains with no words":          {records(expression.Comparison{Field: "fld_name", Op: expression.OpContains}), "no words to look for"},
		"optional on the runtime's scope": {records(expression.Comparison{Field: "fld_name", Op: expression.OpEquals, Value: "$parameters.application", Optional: true}), "never left out"},
		"optional mixed in any:": {domain.Dataset{Select: domain.SelectRecords, Limit: 10, Where: &expression.Predicate{Any: []expression.Comparison{
			{Field: "fld_name", Op: expression.OpEquals, Value: "$current_user"},
			{Field: "fld_name", Op: expression.OpContains, Value: "$parameters.q", Optional: true}}}}, "mixes optional and required"},
		"an id from a literal":  {records(expression.Comparison{Field: "id", Op: expression.OpEquals, Value: "rec_1"}), "compares with a request parameter"},
		"an id mistyped":        {records(expression.Comparison{Field: "id", Op: expression.OpEquals, Value: "$paramters.id"}), "compares with a request parameter"},
		"optional in a measure": {domain.Dataset{Dimension: "fld_name", Measures: []domain.Measure{{ID: "msr_n", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{{Field: "fld_name", Op: expression.OpEquals, Value: "x", Optional: true}}}}}}, "an aggregate has no request parameters"},
	} {
		if got := run(tc.ds); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want it to say %q", name, got, tc.want)
		}
	}
}

// TestAnEdgeThatAsksIsAFormNotAButton: a Button sends a move and nothing else, so an edge whose target makes a Field
// required (a proof when Paid) is refused on a Button and taken by a Form.
func TestAnEdgeThatAsksIsAFormNotAButton(t *testing.T) {
	m := &domain.Machine{ID: "mch_plan", Fields: []domain.Field{
		{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"Open", "Paid"}},
		{ID: "fld_proof", Type: domain.FieldTypeFile, RequiredWhen: &domain.FieldCondition{Field: "fld_status", Equals: "Paid"}},
	}, Transitions: []domain.Transition{{ID: "trn_pay", Name: "Pay", Field: "fld_status", From: "Open", To: "Paid", Action: domain.ActionEdit}}}
	button := domain.PageNode{Kind: "component", Type: "Button", Binding: &domain.PageBinding{Write: domain.PageWriteTransition, Transition: "trn_pay"}}
	if got := strings.Join(transitionBindingIssues(button, m, "w"), "\n"); !strings.Contains(got, "is a Form") {
		t.Errorf("a Button on an edge that asks for a proof: %q", got)
	}
	form := button
	form.Type = "Form"
	if got := transitionBindingIssues(form, m, "w"); len(got) != 0 {
		t.Errorf("a Form on the same edge was refused: %v", got)
	}
}

// TestAConditionalComparisonNeverTurnsOffAScope (the review of step 4): if_param belongs to a literal about the record in
// all:; on the viewer it would list everyone's records when the view is not chosen, and in any: it would drop the group.
func TestAConditionalComparisonNeverTurnsOffAScope(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Fields: []domain.Field{{ID: "fld_owner", Type: domain.FieldTypePerson}, {ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"Open", "Late"}}}}
	fields := map[string]domain.Field{"fld_owner": m.Fields[0], "fld_status": m.Fields[1]}
	run := func(ds domain.Dataset) string {
		ds.ID, ds.Source, ds.Select, ds.Limit = "ds_x", "mch_x", domain.SelectRecords, 10
		return strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "\n")
	}
	scoped := domain.Dataset{ViewerScoped: true, Where: &expression.Predicate{All: []expression.Comparison{{Field: "fld_owner", Op: expression.OpEquals, Value: "$current_user", IfParam: "view=mine"}}}}
	if got := run(scoped); !strings.Contains(got, "never the viewer") || !strings.Contains(got, "no top-level") {
		t.Errorf("a conditional viewer scope: %q", got)
	}
	inAny := domain.Dataset{Where: &expression.Predicate{Any: []expression.Comparison{{Field: "fld_status", Op: expression.OpEquals, Value: "Late", IfParam: "view=late"}}}}
	if got := run(inAny); !strings.Contains(got, "sit in all:") {
		t.Errorf("a conditional comparison in any:: %q", got)
	}
	fine := domain.Dataset{Where: &expression.Predicate{All: []expression.Comparison{{Field: "fld_status", Op: expression.OpEquals, Value: "Late", IfParam: "view=late"}}}}
	if got := run(fine); got != "" {
		t.Errorf("a named view was refused: %q", got)
	}
}
