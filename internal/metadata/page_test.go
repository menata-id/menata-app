package metadata

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
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
card_fields:
  - { field: fld_title, role: title }
  - { field: fld_status, role: status }
  - { field: fld_color, role: color }
  - { field: fld_owner, role: person }
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

// TestPage_aPersonRoleIsTheOneReferenceAPageResolves: a records template may take its text from a role whose Field
// is a person, because the Workspace's members are answered by the Loader's memoized PersonNames; a relation to
// any other Machine stays refused (it would read that whole Machine, 007 §20).
func TestPage_aPersonRoleIsTheOneReferenceAPageResolves(t *testing.T) {
	page := "  - id: nav_p\n    label: P\n    route: /pages/nav_p\n    page:\n" + list("ds_rows", "person")
	if err := pageFixtureWith(t, page, "person"); err != nil {
		t.Errorf("a person role was refused: %v", err)
	}
	err := pageFixtureWith(t, page, "relation\n    machine: mch_doc")
	if err == nil || !strings.Contains(err.Error(), "only a person Field is resolved") {
		t.Errorf("a relation role: error = %v; want it refused", err)
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
	ref := domain.Field{ID: "f3", Name: "Owner", Type: domain.FieldTypeRelation, RelatedMachine: "mch_y", Required: true}
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{text, ref}}); !strings.Contains(got, `requires field "f3"`) {
		t.Errorf("a required reference no form can ask for: %q", got)
	}
	ref.Required = false
	if got := check(&domain.Machine{ID: "mch_x", Fields: []domain.Field{text, ref}}); got != "" {
		t.Errorf("an optional reference is simply not asked for, got %q", got)
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
