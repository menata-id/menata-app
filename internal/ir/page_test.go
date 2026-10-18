package ir

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func metricBound(props map[string]string) domain.PageNode {
	return domain.PageNode{
		Kind: "component", Type: string(domain.ComponentMetric), Props: props,
		Binding: &domain.PageBinding{Dataset: "ds_a", Measure: "msr_b", Rows: domain.PageRowsDimension},
	}
}

func fixedRows(rows ...Row) Resolver {
	return Resolver{Rows: func(domain.PageBinding) ([]Row, error) { return rows, nil }}
}

func fixedRecords(recs ...map[string]string) Resolver {
	return Resolver{Records: func(domain.PageBinding) (RecordSet, error) { return RecordSet{Records: recs}, nil }}
}

// listBound is the shape a records binding is written in: a Collection whose one child is the item template.
func listBound(template domain.PageNode) domain.PageNode {
	return domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection), Props: map[string]string{"gap": "tight"},
		Binding:  &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{template},
	}
}

func rowTemplate() domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "row", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}},
		{Kind: "component", Type: "StatusBadge", Props: map[string]string{"tone": "info"}, From: map[string]string{"label": "status"}},
	}}
}

func TestLower_boundNodeBecomesOneNodePerRowInResolverOrder(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "default"},
		Children: []domain.PageNode{metricBound(map[string]string{"tone": "warn"})}}
	got, err := Lower(root, fixedRows(Row{Label: "b", Value: "2"}, Row{Label: "a", Value: "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != NodeLayout || got.Type != "grid" || len(got.Children) != 2 {
		t.Fatalf("tree = %+v", got)
	}
	if got.Children[0].Props["label"] != "b" || got.Children[1].Props["label"] != "a" {
		t.Errorf("rows were reordered: %+v", got.Children)
	}
	want := map[string]string{"label": "b", "value": "2", "tone": "warn"}
	if !reflect.DeepEqual(got.Children[0].Props, want) {
		t.Errorf("props = %v, want %v (other properties copied to every row)", got.Children[0].Props, want)
	}
	got.Children[0].Props["tone"] = "bad"
	if got.Children[1].Props["tone"] != "warn" {
		t.Error("rows share one Props map; changing one changed the other")
	}
}

func TestLower_zeroRowsIsAnEmptyLayoutNotAnError(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "grid", Children: []domain.PageNode{metricBound(nil)}}
	got, err := Lower(root, fixedRows())
	if err != nil || len(got.Children) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestLower_refusesADeclarationThatIsNotAWellFormedBinding(t *testing.T) {
	wrap := func(n domain.PageNode) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}
	}
	cases := map[string]domain.PageNode{
		"not bindable": {Kind: "static", Type: "heading", Binding: &domain.PageBinding{Dataset: "d", Measure: "m", Rows: "dimension"}},
		"no dataset":   {Kind: "component", Type: "Metric", Binding: &domain.PageBinding{Measure: "m", Rows: "dimension"}},
		"unknown rows": {Kind: "component", Type: "Metric", Binding: &domain.PageBinding{Dataset: "d", Measure: "m", Rows: "scalar"}},
		"typed label":  metricBound(map[string]string{"label": "x"}),
		"typed value":  metricBound(map[string]string{"value": "9"}),
		"holds a child": func() domain.PageNode {
			n := metricBound(nil)
			n.Children = []domain.PageNode{{Kind: "static", Type: "paragraph"}}
			return n
		}(),
	}
	for name, n := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Lower(wrap(n), fixedRows(Row{Label: "a", Value: "1"})); err == nil {
				t.Fatal("Lower accepted it")
			}
		})
	}
}

func TestLower_rootMustBeOneUnboundNode(t *testing.T) {
	if _, err := Lower(metricBound(nil), fixedRows(Row{Label: "a", Value: "1"})); err == nil || !strings.Contains(err.Error(), "root") {
		t.Errorf("err = %v; want a root refusal", err)
	}
}

func TestLower_resolverErrorsPropagate(t *testing.T) {
	boom := errors.New("boom")
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{metricBound(nil)}}
	_, err := Lower(root, Resolver{Rows: func(domain.PageBinding) ([]Row, error) { return nil, boom }})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

func TestLower_recordsBindingClonesTheTemplateOncePerRecordInResolverOrder(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(rowTemplate())}}
	got, err := Lower(root, fixedRecords(
		map[string]string{"title": "B", "status": "draft"},
		map[string]string{"title": "A", "status": "approved"},
	))
	if err != nil {
		t.Fatal(err)
	}
	list := got.Children[0]
	if list.Type != "Collection" || list.Props["gap"] != "tight" || len(list.Children) != 2 {
		t.Fatalf("list = %+v", list)
	}
	first, second := list.Children[0], list.Children[1]
	if first.Children[0].Props["text"] != "B" || second.Children[0].Props["text"] != "A" {
		t.Errorf("records were reordered: %+v", list.Children)
	}
	badge := first.Children[1].Props
	if badge["label"] != "draft" || badge["tone"] != "info" {
		t.Errorf("badge props = %v; want label from the record and the declared tone kept", badge)
	}
	first.Children[1].Props["tone"] = "bad"
	if second.Children[1].Props["tone"] != "info" {
		t.Error("items share one Props map; changing one changed the other")
	}
}

func TestLower_zeroRecordsIsAnEmptyCollectionNotAnError(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(rowTemplate())}}
	got, err := Lower(root, fixedRecords())
	if err != nil || len(got.Children[0].Children) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestLower_refusesARecordsDeclarationThatIsNotWellFormed(t *testing.T) {
	wrap := func(n domain.PageNode) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}
	}
	rec := map[string]string{"title": "t", "status": "s"}
	with := func(f func(*domain.PageNode)) domain.PageNode {
		n := listBound(rowTemplate())
		f(&n)
		return wrap(n)
	}
	cases := map[string]domain.PageNode{
		"no template": with(func(n *domain.PageNode) { n.Children = nil }),
		"two templates": with(func(n *domain.PageNode) {
			n.Children = append(n.Children, rowTemplate())
		}),
		"a measure":                      with(func(n *domain.PageNode) { n.Binding.Measure = "msr_b" }),
		"dimension mode on a collection": with(func(n *domain.PageNode) { n.Binding.Rows = domain.PageRowsDimension; n.Binding.Measure = "m" }),
		"records mode on a metric": wrap(domain.PageNode{Kind: "component", Type: "Metric",
			Binding: &domain.PageBinding{Dataset: "d", Rows: domain.PageRowsRecords}}),
		"from outside a template":             wrap(domain.PageNode{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}}),
		"from on the bound collection itself": with(func(n *domain.PageNode) { n.From = map[string]string{"gap": "title"} }),
		"property written and taken": with(func(n *domain.PageNode) {
			n.Children[0].Children[0].Props = map[string]string{"text": "typed"}
		}),
		"role the record lacks": with(func(n *domain.PageNode) {
			n.Children[0].Children[0].From = map[string]string{"text": "nonesuch"}
		}),
		"a binding inside the template": with(func(n *domain.PageNode) {
			n.Children[0].Children = append(n.Children[0].Children, metricBound(nil))
		}),
	}
	for name, n := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Lower(n, Resolver{
				Rows:    func(domain.PageBinding) ([]Row, error) { return []Row{{Label: "a", Value: "1"}}, nil },
				Records: func(domain.PageBinding) (RecordSet, error) { return RecordSet{Records: []map[string]string{rec}}, nil },
			}); err == nil {
				t.Fatal("Lower accepted it")
			}
		})
	}
}

func TestLower_fromOutsideARecordsTemplateIsRefusedEvenWithNoResolver(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}}}}
	if _, err := Lower(root, Resolver{}); err == nil || !strings.Contains(err.Error(), "from:") {
		t.Errorf("err = %v; want a from: refusal", err)
	}
}

func TestLower_aRecordsBindingNeedsARecordResolver(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(rowTemplate())}}
	if _, err := Lower(root, fixedRows()); err == nil {
		t.Error("Lower accepted a records binding with no record resolver")
	}
}

func linkTo(navID string, props map[string]string) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "link", To: navID, Props: props}}}
}

func navFixture(route string) Resolver {
	return Resolver{Route: NavigationRoutes([]domain.NavigationItem{{ID: "nav_x", Label: "The Label", Route: route}})}
}

func TestLower_linkResolvesItsRouteAndDefaultsItsTextToTheNavigationLabel(t *testing.T) {
	tree, err := Lower(linkTo("nav_x", nil), navFixture("/somewhere"))
	if err != nil {
		t.Fatal(err)
	}
	got := tree.Children[0].Props
	if got["href"] != "/somewhere" || got["text"] != "The Label" {
		t.Errorf("props = %v; want href=/somewhere text=The Label", got)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a resolved link must validate: %v", issues)
	}
}

// The same page over two navigations links to two routes: the destination is data, not part of the tree.
func TestLower_theSamePageLinksWhereverTheNavigationPointsIt(t *testing.T) {
	a, _ := Lower(linkTo("nav_x", nil), navFixture("/one"))
	b, _ := Lower(linkTo("nav_x", nil), navFixture("/two"))
	if a.Children[0].Props["href"] != "/one" || b.Children[0].Props["href"] != "/two" {
		t.Errorf("hrefs = %q, %q; want /one, /two", a.Children[0].Props["href"], b.Children[0].Props["href"])
	}
}

func TestLower_aWrittenTextOverridesTheLabelButAWrittenHrefIsRefused(t *testing.T) {
	tree, err := Lower(linkTo("nav_x", map[string]string{"text": "Go there"}), navFixture("/r"))
	if err != nil || tree.Children[0].Props["text"] != "Go there" {
		t.Errorf("err=%v props=%v; want the written text kept", err, tree.Children[0].Props)
	}
	if _, err := Lower(linkTo("nav_x", map[string]string{"href": "/typed"}), navFixture("/r")); err == nil || !strings.Contains(err.Error(), "href") {
		t.Errorf("err = %v; want a typed href refused", err)
	}
}

func TestLower_refusesALinkThatIsNotWellFormed(t *testing.T) {
	for name, c := range map[string]struct {
		root domain.PageNode
		res  Resolver
		want string
	}{
		"unknown target":   {linkTo("nav_missing", nil), navFixture("/r"), "nav_missing"},
		"no target":        {linkTo("", nil), navFixture("/r"), "to:"},
		"no resolver":      {linkTo("nav_x", nil), Resolver{}, "resolver"},
		"to on a non-link": {domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "static", Type: "paragraph", To: "nav_x"}}}, navFixture("/r"), "only on static: link"},
	} {
		if _, err := Lower(c.root, c.res); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

func recordLinkTemplate(from map[string]string, props map[string]string, to string) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		listBound(domain.PageNode{Kind: "static", Type: "link", From: from, Props: props, To: to})}}
}

// A link inside a records template takes its text from a Projection role and its address from the reserved
// `record` word, each item getting its own record's route.
func TestLower_aRecordLinkTakesItsRouteFromTheRecordNotFromTheYAML(t *testing.T) {
	tree, err := Lower(recordLinkTemplate(map[string]string{"text": "title", "href": domain.PageRecordRole}, nil, ""), fixedRecords(
		map[string]string{"title": "A", domain.PageRecordRole: "/machines/m/records/1"},
		map[string]string{"title": "B", domain.PageRecordRole: "/machines/m/records/2"},
	))
	if err != nil {
		t.Fatal(err)
	}
	items := tree.Children[0].Children
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	for i, want := range []string{"/machines/m/records/1", "/machines/m/records/2"} {
		if got := items[i].Props["href"]; got != want {
			t.Errorf("item %d href = %q, want %q", i, got, want)
		}
	}
}

func TestLower_refusesARecordLinkThatCouldLeakOrFabricateAnAddress(t *testing.T) {
	rec := map[string]string{"title": "A", "status": "s", domain.PageRecordRole: "/machines/m/records/1"}
	for name, c := range map[string]struct {
		root domain.PageNode
		want string
	}{
		"href from a projection role": {recordLinkTemplate(map[string]string{"text": "title", "href": "title"}, nil, ""), "only"},
		"record as text":              {recordLinkTemplate(map[string]string{"text": domain.PageRecordRole}, nil, "nav_x"), "valid only as a link's href"},
		"record on a paragraph":       {domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(domain.PageNode{Kind: "static", Type: "paragraph", From: map[string]string{"text": domain.PageRecordRole}})}}, "valid only as a link's href"},
		"to and record together":      {recordLinkTemplate(map[string]string{"text": "title", "href": domain.PageRecordRole}, nil, "nav_x"), "one destination"},
		"typed href beside record":    {recordLinkTemplate(map[string]string{"text": "title", "href": domain.PageRecordRole}, map[string]string{"href": "/typed"}, ""), "may not also be written"},
		"no text to show":             {recordLinkTemplate(map[string]string{"href": domain.PageRecordRole}, nil, ""), "text"},
		"record outside a template":   {domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "static", Type: "link", From: map[string]string{"text": "title", "href": domain.PageRecordRole}}}}, "item template"},
	} {
		res := fixedRecords(rec)
		res.Route = NavigationRoutes([]domain.NavigationItem{{ID: "nav_x", Label: "L", Route: "/r"}})
		if _, err := Lower(c.root, res); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

func TestLower_emptyWordsAreCarriedOntoTheCollectionWhetherOrNotItIsEmpty(t *testing.T) {
	bound := listBound(rowTemplate())
	bound.Props = map[string]string{"gap": "tight", "empty": "No documents yet."}
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{bound}}
	for name, rec := range map[string]Resolver{"none": fixedRecords(), "some": fixedRecords(map[string]string{"title": "t", "status": "s"})} {
		got, err := Lower(root, rec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Children[0].Props["empty"] != "No documents yet." {
			t.Errorf("%s: empty did not reach the lowered Collection: %+v", name, got.Children[0].Props)
		}
	}
}

func TestLower_emptyIsRefusedWhereABindingCannotProduceNothing(t *testing.T) {
	unbound := domain.PageNode{Kind: "component", Type: "Collection", Props: map[string]string{"gap": "tight", "empty": "none"},
		Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{unbound}}
	if _, err := Lower(root, fixedRecords()); err == nil {
		t.Fatal("Lower accepted empty: on a Collection that holds its own children")
	}
}

func listOfLink(dataset string, props map[string]string) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "link", ListOf: dataset, Props: props}}}
}

func sourceFixture(sources map[string]string) Resolver {
	return Resolver{Source: func(id string) (string, bool) { m, ok := sources[id]; return m, ok }}
}

// The href is the runtime's generic page for the Machine the Dataset reads; the author wrote a Dataset id
// and some words, and the route is built where every other Machine route is.
func TestLower_aListOfLinkTakesItsRouteFromTheDatasetsMachine(t *testing.T) {
	tree, err := Lower(listOfLink("ds_x", map[string]string{"text": "All of them"}), sourceFixture(map[string]string{"ds_x": "mch_y"}))
	if err != nil {
		t.Fatal(err)
	}
	got := tree.Children[0].Props
	if got["href"] != domain.MachineListRoute("mch_y") || got["text"] != "All of them" {
		t.Errorf("props = %v; want href=%s text=All of them", got, domain.MachineListRoute("mch_y"))
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a resolved link must validate: %v", issues)
	}
	other, _ := Lower(listOfLink("ds_x", map[string]string{"text": "t"}), sourceFixture(map[string]string{"ds_x": "mch_z"}))
	if other.Children[0].Props["href"] == got["href"] {
		t.Error("the same page over a different source Machine linked to the same route -- the route is not coming from the resolver")
	}
}

func TestLower_refusesAListOfLinkThatIsNotWellFormed(t *testing.T) {
	res := sourceFixture(map[string]string{"ds_x": "mch_y"})
	both := listOfLink("ds_x", map[string]string{"text": "t"})
	both.Children[0].To = "nav_x"
	fromHref := recordLinkTemplate(map[string]string{"href": domain.PageRecordRole}, map[string]string{"text": "t"}, "")
	fromHref.Children[0].Children[0].ListOf = "ds_x"
	inTemplate := Resolver{Source: res.Source, Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{{domain.PageRecordRole: "/r"}}}, nil
	}}
	for name, c := range map[string]struct {
		root domain.PageNode
		res  Resolver
		want string
	}{
		"unknown dataset": {listOfLink("ds_missing", map[string]string{"text": "t"}), res, "ds_missing"},
		"no text":         {listOfLink("ds_x", nil), res, "text"},
		"typed href":      {listOfLink("ds_x", map[string]string{"text": "t", "href": "/machines/mch_y"}), res, "href"},
		"no resolver":     {listOfLink("ds_x", map[string]string{"text": "t"}), Resolver{}, "resolver"},
		"beside to":       {both, res, "one destination"},
		"beside from":     {fromHref, inTemplate, "one destination"},
		"on a non-link":   {domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "static", Type: "paragraph", ListOf: "ds_x"}}}, res, "only on static: link"},
	} {
		if _, err := Lower(c.root, c.res); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

func countNode(c domain.PageCount) domain.PageNode {
	return domain.PageNode{Kind: "static", Type: "caption", Count: &c}
}

func countText(t *testing.T, c domain.PageCount, counts ...string) []string {
	t.Helper()
	recs := make([]map[string]string, 0, len(counts))
	for _, n := range counts {
		recs = append(recs, map[string]string{domain.PageCountRole("rel_x"): n})
	}
	tree, err := Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(countNode(c))}}, fixedRecords(recs...))
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	var out []string
	for _, k := range tree.Children[0].Children {
		out = append(out, k.Props["text"])
	}
	return out
}

// TestLowerCountChoosesTheWordingAndSubstitutesTheNumber: the author writes words, the Relation writes the
// number; one serves exactly 1, other serves 0 and every larger number.
func TestLowerCountChoosesTheWordingAndSubstitutesTheNumber(t *testing.T) {
	c := domain.PageCount{Of: "rel_x", One: "{n} card", Other: "{n} cards"}
	got := strings.Join(countText(t, c, "0", "1", "2", "12"), "|")
	if want := "0 cards|1 card|2 cards|12 cards"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := countText(t, domain.PageCount{Of: "rel_x", Other: "{n} items"}, "1"); got[0] != "1 items" {
		t.Fatalf("with no one:, other serves 1 too; got %q", got[0])
	}
}

func TestLowerCountIsRefusedWhereItCannotMeanAThing(t *testing.T) {
	good := domain.PageCount{Of: "rel_x", Other: "{n} cards"}
	rec := map[string]string{domain.PageCountRole("rel_x"): "3"}
	cases := map[string]struct {
		node domain.PageNode
		want string
	}{
		"outside a template": {countNode(good), "item template"},
		"a relation the dataset does not declare": {
			countNode(domain.PageCount{Of: "rel_missing", Other: "{n}"}), "does not declare",
		},
		"no number in the words": {countNode(domain.PageCount{Of: "rel_x", Other: "many cards"}), "{n}"},
		"no other:":              {countNode(domain.PageCount{Of: "rel_x", One: "{n} card"}), "other:"},
		"text also written": {domain.PageNode{Kind: "static", Type: "caption", Count: &good,
			Props: map[string]string{"text": "typed"}}, "may not also"},
		"text also taken from a role": {domain.PageNode{Kind: "static", Type: "caption", Count: &good,
			From: map[string]string{"text": "title"}}, "may not also"},
		"on a component": {domain.PageNode{Kind: "component", Type: "Tag", Count: &good}, "static"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var err error
			if name == "outside a template" {
				_, err = Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{tc.node}}, Resolver{})
			} else {
				full := map[string]string{"title": "x"}
				for k, v := range rec {
					full[k] = v
				}
				_, err = Lower(domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(tc.node)}}, fixedRecords(full))
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

func completeList(complete string) domain.PageNode {
	bound := listBound(rowTemplate())
	bound.Props = map[string]string{"gap": "tight"}
	if complete != "" {
		bound.Props["complete"] = complete
	}
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{bound}}
}

func truncatedRecords(truncated bool, limit int) Resolver {
	rec := map[string]string{"title": "t", "status": "s"}
	return Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{rec}, Truncated: truncated, Limit: limit}, nil
	}}
}

// The author claims completeness; lowering turns the claim into the bound only when the bound bit, and never
// copies the claim itself onto the node. A list that was not cut short is the same node as one that claimed
// nothing.
func TestLowerCompleteNamesTheBoundOnlyWhenItBit(t *testing.T) {
	for name, c := range map[string]struct {
		complete  string
		truncated bool
		want      string
	}{
		"claimed and cut":       {"true", true, "200"},
		"claimed, not cut":      {"true", false, ""},
		"not claimed, cut":      {"", true, ""},
		"explicitly not, cut":   {"false", true, ""},
		"claimed, cut, no size": {"true", true, ""},
	} {
		limit := 200
		if name == "claimed, cut, no size" {
			limit = 0
		}
		got, err := Lower(completeList(c.complete), truncatedRecords(c.truncated, limit))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		props := got.Children[0].Props
		if props["truncated"] != c.want {
			t.Errorf("%s: truncated = %q, want %q", name, props["truncated"], c.want)
		}
		if _, leaked := props["complete"]; leaked {
			t.Errorf("%s: the claim reached the lowered node: %+v", name, props)
		}
	}
}

func TestLowerCompleteIsRefusedWhereItCannotMeanAThing(t *testing.T) {
	unbound := func(prop string) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
			Kind: "component", Type: "Collection", Props: map[string]string{"gap": "tight", prop: "true"},
			Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}}}
	}
	authored := completeList("true")
	authored.Children[0].Props["truncated"] = "5"
	notBool := completeList("yes")
	for name, root := range map[string]domain.PageNode{
		"complete on a Collection holding its own children":  unbound("complete"),
		"truncated on a Collection holding its own children": unbound("truncated"),
		"a hand-written truncated on a bound list":           authored,
		"complete that is not true or false":                 notBool,
	} {
		if _, err := Lower(root, truncatedRecords(true, 3)); err == nil {
			t.Errorf("%s: Lower accepted it", name)
		}
	}
}

func linkingMetric(to, param string, props map[string]string) domain.PageNode {
	n := metricBound(props)
	n.To, n.Param = to, param
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}
}

func rowsAndRoutes(route string, rows ...Row) Resolver {
	r := navFixture(route)
	r.Rows = func(domain.PageBinding) ([]Row, error) { return rows, nil }
	return r
}

// The destination is a navigation item and the value is the row's own Dimension value, escaped as data: a
// status holding `&` or a space must arrive at the other page as itself.
func TestLower_aLinkingMetricCarriesEachRowsValueToItsDestination(t *testing.T) {
	tree, err := Lower(linkingMetric("nav_x", "status", nil), rowsAndRoutes("/pages/nav_x", Row{Label: "in review & more", Value: "3"}, Row{Label: "draft", Value: "1"}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/pages/nav_x?status=in+review+%26+more", "/pages/nav_x?status=draft"}
	for i, w := range want {
		if got := tree.Children[i].Props["href"]; got != w {
			t.Errorf("row %d href = %q, want %q", i, got, w)
		}
	}
	plain, err := Lower(linkingMetric("", "", nil), rowsAndRoutes("/pages/nav_x", Row{Label: "draft", Value: "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, has := plain.Children[0].Props["href"]; has {
		t.Error("a Metric that declares no destination acquired an href")
	}
}

// A reference Dimension stores an id and shows a title: the link must carry what the destination can compare
// against (the id), while the Metric shows the title.
func TestLower_aLinkCarriesTheRowsKeyWhenItsLabelIsNotTheStoredValue(t *testing.T) {
	tree, err := Lower(linkingMetric("nav_x", "project", nil), rowsAndRoutes("/pages/nav_x", Row{Label: "Website relaunch", Key: "rec_abc", Value: "3"}))
	if err != nil {
		t.Fatal(err)
	}
	c := tree.Children[0]
	if c.Props["label"] != "Website relaunch" || c.Props["href"] != "/pages/nav_x?project=rec_abc" {
		t.Errorf("label=%q href=%q, want the title shown and the id linked", c.Props["label"], c.Props["href"])
	}
}

func TestLower_refusesALinkingMetricThatIsNotWellFormed(t *testing.T) {
	res := rowsAndRoutes("/pages/nav_x", Row{Label: "draft", Value: "1"})
	collection := listBound(rowTemplate())
	collection.To, collection.Param = "nav_x", "status"
	link := linkTo("nav_x", nil)
	link.Children[0].Param = "status"
	for name, tc := range map[string]struct {
		root domain.PageNode
		want string
	}{
		"to without param":    {linkingMetric("nav_x", "", nil), "both to:"},
		"param without to":    {linkingMetric("", "status", nil), "both to:"},
		"unknown destination": {linkingMetric("nav_nope", "status", nil), "names no navigation item"},
		"typed href":          {linkingMetric("nav_x", "status", map[string]string{"href": "/x"}), "never a typed href"},
		"on a records list":   {domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{collection}}, "belong on a Metric"},
		"param on a link":     {link, "param: carries a row's value"},
	} {
		_, err := Lower(tc.root, res)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, tc.want)
		}
	}
}

func formBound(props map[string]string) domain.PageNode {
	return domain.PageNode{
		Kind: "component", Type: string(domain.ComponentForm), Props: props,
		Binding: &domain.PageBinding{Dataset: "ds_a", Write: domain.PageWriteCreate},
	}
}

func formResolver(spec FormSpec) Resolver {
	return Resolver{Form: func(domain.PageBinding) (FormSpec, error) { return spec, nil }}
}

func formPage(n domain.PageNode) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}
}

func TestLower_formBecomesAFieldPerInputWithEverythingTheMachineDecided(t *testing.T) {
	spec := FormSpec{Route: "/machines/mch_x/records", Permitted: true, Inputs: []domain.FormInput{
		{FieldID: "fld_name", Label: "Name", Kind: domain.InputText, Required: true},
		{FieldID: "fld_color", Label: "Colour", Kind: domain.InputSelect, Default: "blue", Options: []string{"blue", "red"}},
	}}
	got, err := Lower(formPage(formBound(map[string]string{"submit": "Add label"})), formResolver(spec))
	if err != nil {
		t.Fatal(err)
	}
	f := got.Children[0]
	if f.Type != "Form" || f.Props["action"] != spec.Route || f.Props["submit"] != "Add label" || len(f.Children) != 2 {
		t.Fatalf("form = %+v", f)
	}
	in := f.Children[1].Children[0]
	if in.Props["id"] != f.Children[1].Props["for"] {
		t.Errorf("the control's id must be what its label names: %v vs %v", in.Props, f.Children[1].Props)
	}
	if in.Type != "Input" || in.Props["name"] != "fld_color" || in.Props["kind"] != "select" || in.Props["value"] != "blue" ||
		in.Props["options"] != "blue"+domain.InputOptionsSep+"red" {
		t.Errorf("select input = %+v", in)
	}
	first := f.Children[0].Children[0]
	if first.Props["required"] != "true" || f.Children[0].Props["label"] != "Name *" {
		t.Errorf("first = %+v / %+v", f.Children[0], first)
	}
	if f.Children[0].Props["for"] == "" || f.Children[0].Props["for"] == f.Children[1].Props["for"] {
		t.Errorf("label targets must be set and distinct: %v %v", f.Children[0].Props, f.Children[1].Props)
	}
}

func TestLower_aViewerWhoMayNotCreateGetsNoFormAtAll(t *testing.T) {
	spec := FormSpec{Route: "/machines/mch_x/records", Permitted: false, Inputs: []domain.FormInput{{FieldID: "f", Label: "F", Kind: domain.InputText}}}
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}},
		formBound(map[string]string{"submit": "Add"}),
	}}
	got, err := Lower(root, formResolver(spec))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Children) != 1 {
		t.Errorf("children = %+v, want only the paragraph", got.Children)
	}
}

func TestLower_refusesAFormThatIsNotWellFormed(t *testing.T) {
	spec := FormSpec{Route: "/machines/mch_x/records", Permitted: true, Inputs: []domain.FormInput{{FieldID: "f", Label: "F", Kind: domain.InputText}}}
	ok := formBound(map[string]string{"submit": "Add"})
	cases := map[string]func(n *domain.PageNode){
		"typed action":      func(n *domain.PageNode) { n.Props["action"] = "/machines/mch_y/records" },
		"children":          func(n *domain.PageNode) { n.Children = []domain.PageNode{{Kind: "component", Type: "Input"}} },
		"rows beside write": func(n *domain.PageNode) { n.Binding.Rows = domain.PageRowsRecords },
		"measure":           func(n *domain.PageNode) { n.Binding.Measure = "m" },
		"unknown mode":      func(n *domain.PageNode) { n.Binding.Write = "delete" },
		"no dataset":        func(n *domain.PageNode) { n.Binding.Dataset = "" },
		"from":              func(n *domain.PageNode) { n.From = map[string]string{"label": "x"} },
		"on a Metric":       func(n *domain.PageNode) { n.Type = string(domain.ComponentMetric) },
	}
	for name, mutate := range cases {
		n := ok
		n.Props = map[string]string{"submit": "Add"}
		b := *ok.Binding
		n.Binding = &b
		mutate(&n)
		if _, err := Lower(formPage(n), formResolver(spec)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := Lower(formPage(ok), Resolver{}); err == nil {
		t.Error("a Form with no resolver must be refused")
	}
}

func TestLower_anInputIsNeverWrittenByAnAuthor(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "component", Type: string(domain.ComponentFormInput), Props: map[string]string{"name": "fld_x", "kind": "text"}},
	}}
	if _, err := Lower(root, Resolver{}); err == nil || !strings.Contains(err.Error(), "never written") {
		t.Errorf("err = %v", err)
	}
}

// updateInTemplate is a records Collection whose one item is an update Form; the resolver answers two records,
// the second of which this viewer may not edit.
func updateInTemplate(edits []FormSpec) (domain.PageNode, Resolver) {
	coll := domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection),
		Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{
			Kind: "component", Type: string(domain.ComponentForm), Props: map[string]string{"submit": "Rename"},
			Binding: &domain.PageBinding{Write: domain.PageWriteUpdate},
		}},
	}
	res := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{{"title": "a"}, {"title": "b"}}, Edits: edits}, nil
	}}
	return formPage(coll), res
}

func TestLower_updateFormPatchesTheItemsOwnRecordAndStartsAsItsValues(t *testing.T) {
	in := func(v string) []domain.FormInput {
		return []domain.FormInput{{FieldID: "fld_name", Label: "Name", Kind: domain.InputText, Required: true, Default: v}}
	}
	root, res := updateInTemplate([]FormSpec{
		{Route: "/machines/mch_x/records/r1", Method: domain.FormMethodPatch, Permitted: true, Inputs: in("alpha")},
		{Route: "/machines/mch_x/records/r2", Method: domain.FormMethodPatch, Permitted: false, Inputs: in("beta")},
	})
	got, err := Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Children[0].Children
	if len(items) != 1 {
		t.Fatalf("items = %d, want only the record this viewer may edit", len(items))
	}
	f := items[0]
	if f.Props["action"] != "/machines/mch_x/records/r1" || f.Props["method"] != domain.FormMethodPatch || f.Props["submit"] != "Rename" {
		t.Errorf("form props = %v", f.Props)
	}
	if v := f.Children[0].Children[0].Props["value"]; v != "alpha" {
		t.Errorf("control starts as %q, want the record's own value", v)
	}
}

func TestLower_refusesAnUpdateFormThatCouldNotBeBuiltHonestly(t *testing.T) {
	edit := []FormSpec{{Route: "/machines/mch_x/records/r", Method: domain.FormMethodPatch, Permitted: true, Inputs: []domain.FormInput{{FieldID: "f", Label: "F", Kind: domain.InputText}}}}
	cases := map[string]struct {
		mutate func(n *domain.PageNode)
		edits  []FormSpec
		want   string
	}{
		"a dataset of its own": {func(n *domain.PageNode) { n.Children[0].Binding.Dataset = "ds_b" }, append(edit, edit...), "no dataset"},
		"typed method":         {func(n *domain.PageNode) { n.Children[0].Props["method"] = "patch" }, append(edit, edit...), "method"},
		"typed action":         {func(n *domain.PageNode) { n.Children[0].Props["action"] = "/x" }, append(edit, edit...), "action"},
		"no resolver answer":   {func(n *domain.PageNode) {}, nil, "no update form"},
	}
	for name, tc := range cases {
		root, res := updateInTemplate(tc.edits)
		tc.mutate(&root.Children[0])
		if _, err := Lower(root, res); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, tc.want)
		}
	}
	outside := formBound(map[string]string{"submit": "Rename"})
	outside.Binding = &domain.PageBinding{Write: domain.PageWriteUpdate}
	if _, err := Lower(formPage(outside), formResolver(edit[0])); err == nil || !strings.Contains(err.Error(), "item template") {
		t.Errorf("an update Form outside a records template: err = %v", err)
	}
	asCreate := formBound(map[string]string{"submit": "Add"})
	asCreate.Binding.Write = domain.PageWriteUpdate
	if _, err := Lower(formPage(asCreate), formResolver(edit[0])); err == nil {
		t.Error("an update Form naming a dataset outside a template must be refused")
	}
}

// deleteInTemplate is a records Collection whose one item is a delete Button; the resolver answers two records,
// the second of which this viewer may not delete.
func deleteInTemplate(deletes []RecordAction) (domain.PageNode, Resolver) {
	coll := domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection),
		Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{
			Kind: "component", Type: string(domain.ComponentButton),
			Props:   map[string]string{"label": "Delete", "variant": "secondary", "confirm": "Delete this record?"},
			Binding: &domain.PageBinding{Write: domain.PageWriteDelete},
		}},
	}
	res := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{{"title": "a"}, {"title": "b"}}, Deletes: deletes}, nil
	}}
	return formPage(coll), res
}

func TestLower_deleteButtonSendsDeleteToTheItemsOwnRecordAndOnlyWhenPermitted(t *testing.T) {
	root, res := deleteInTemplate([]RecordAction{
		{Route: "/machines/mch_x/records/r1", Permitted: true},
		{Route: "/machines/mch_x/records/r2", Permitted: false},
	})
	got, err := Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Children[0].Children
	if len(items) != 1 {
		t.Fatalf("items = %d, want only the record this viewer may delete", len(items))
	}
	p := items[0].Props
	if p["action"] != "/machines/mch_x/records/r1" || p["method"] != domain.ButtonMethodDelete || p["confirm"] != "Delete this record?" || p["label"] != "Delete" {
		t.Errorf("button props = %v", p)
	}
}

func TestLower_refusesADeleteButtonThatCouldNotBeBuiltHonestly(t *testing.T) {
	del := []RecordAction{{Route: "/machines/mch_x/records/r", Permitted: true}, {Route: "/machines/mch_x/records/r", Permitted: true}}
	cases := map[string]struct {
		mutate  func(n *domain.PageNode)
		deletes []RecordAction
		want    string
	}{
		"a dataset of its own": {func(n *domain.PageNode) { n.Children[0].Binding.Dataset = "ds_b" }, del, "no dataset"},
		"no confirm":           {func(n *domain.PageNode) { delete(n.Children[0].Props, "confirm") }, del, "confirm"},
		"typed method":         {func(n *domain.PageNode) { n.Children[0].Props["method"] = "delete" }, del, "method"},
		"typed action":         {func(n *domain.PageNode) { n.Children[0].Props["action"] = "/x" }, del, "action"},
		"typed name":           {func(n *domain.PageNode) { n.Children[0].Props["name"] = "x" }, del, "name"},
		"no resolver answer":   {func(n *domain.PageNode) {}, nil, "no delete"},
	}
	for name, tc := range cases {
		root, res := deleteInTemplate(tc.deletes)
		tc.mutate(&root.Children[0])
		if _, err := Lower(root, res); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, tc.want)
		}
	}
	outside := domain.PageNode{Kind: "component", Type: string(domain.ComponentButton),
		Props:   map[string]string{"label": "Delete", "variant": "secondary", "confirm": "Sure?"},
		Binding: &domain.PageBinding{Write: domain.PageWriteDelete}}
	if _, err := Lower(formPage(outside), Resolver{}); err == nil || !strings.Contains(err.Error(), "item template") {
		t.Errorf("a delete Button outside a records template: err = %v", err)
	}
}

func moveInTemplate(direction string, moves []RecordMove) (domain.PageNode, Resolver) {
	coll := domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection),
		Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{
			Kind: "component", Type: string(domain.ComponentButton),
			Props:   map[string]string{"label": "Move " + direction, "variant": "secondary", "direction": direction},
			Binding: &domain.PageBinding{Write: domain.PageWriteMove},
		}},
	}
	res := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{{"title": "a"}, {"title": "b"}, {"title": "c"}}, Moves: moves}, nil
	}}
	return formPage(coll), res
}

func TestLower_moveButtonPostsToTheItemsMoveRouteAndIsDrawnOnlyWhereItCanGo(t *testing.T) {
	moves := []RecordMove{
		{Route: "/machines/mch_x/records/r1", Up: false, Down: true},
		{Route: "/machines/mch_x/records/r2", Up: true, Down: true},
		{Route: "/machines/mch_x/records/r3", Up: true, Down: false},
	}
	for dir, wantRoutes := range map[string][]string{
		"up":   {"/machines/mch_x/records/r2/move?direction=up", "/machines/mch_x/records/r3/move?direction=up"},
		"down": {"/machines/mch_x/records/r1/move?direction=down", "/machines/mch_x/records/r2/move?direction=down"},
	} {
		root, res := moveInTemplate(dir, moves)
		got, err := Lower(root, res)
		if err != nil {
			t.Fatal(err)
		}
		var routes []string
		for _, item := range got.Children[0].Children {
			p := item.Props
			if p["method"] != domain.ButtonMethodPost || p["direction"] != "" || p["confirm"] != "" {
				t.Errorf("%s: button props = %v", dir, p)
			}
			routes = append(routes, p["action"])
		}
		if strings.Join(routes, " ") != strings.Join(wantRoutes, " ") {
			t.Errorf("%s: routes = %v, want %v (no Up on the first record, no Down on the last)", dir, routes, wantRoutes)
		}
	}
}

func TestLower_refusesAMoveButtonThatCouldNotBeBuiltHonestly(t *testing.T) {
	mv := []RecordMove{{Route: "/machines/mch_x/records/r", Up: true, Down: true}, {Route: "/machines/mch_x/records/r", Up: true, Down: true}, {Route: "/machines/mch_x/records/r", Up: true, Down: true}}
	cases := map[string]struct {
		mutate func(n *domain.PageNode)
		moves  []RecordMove
		want   string
	}{
		"a dataset of its own": {func(n *domain.PageNode) { n.Children[0].Binding.Dataset = "ds_b" }, mv, "no dataset"},
		"no direction":         {func(n *domain.PageNode) { delete(n.Children[0].Props, "direction") }, mv, "direction"},
		"sideways":             {func(n *domain.PageNode) { n.Children[0].Props["direction"] = "left" }, mv, "direction"},
		"a confirm":            {func(n *domain.PageNode) { n.Children[0].Props["confirm"] = "Sure?" }, mv, "asks nothing"},
		"typed method":         {func(n *domain.PageNode) { n.Children[0].Props["method"] = "post" }, mv, "method"},
		"typed action":         {func(n *domain.PageNode) { n.Children[0].Props["action"] = "/x" }, mv, "action"},
		"no resolver answer":   {func(n *domain.PageNode) {}, nil, "no move"},
	}
	for name, tc := range cases {
		root, res := moveInTemplate("up", tc.moves)
		tc.mutate(&root.Children[0])
		if _, err := Lower(root, res); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, tc.want)
		}
	}
	outside := domain.PageNode{Kind: "component", Type: string(domain.ComponentButton),
		Props:   map[string]string{"label": "Move up", "variant": "secondary", "direction": "up"},
		Binding: &domain.PageBinding{Write: domain.PageWriteMove}}
	if _, err := Lower(formPage(outside), Resolver{}); err == nil || !strings.Contains(err.Error(), "item template") {
		t.Errorf("a move Button outside a records template: err = %v", err)
	}
}

func metricTotal(props map[string]string) domain.PageNode {
	return domain.PageNode{
		Kind: "component", Type: string(domain.ComponentMetric), Props: props,
		Binding: &domain.PageBinding{Dataset: "ds_a", Measure: "msr_b", Rows: domain.PageRowsTotal},
	}
}

func fixedTotal(v string) Resolver {
	return Resolver{Total: func(domain.PageBinding) (string, error) { return v, nil }}
}

func lowerOne(t *testing.T, n domain.PageNode, r Resolver) (UINode, error) {
	t.Helper()
	return Lower(domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "default"}, Children: []domain.PageNode{n}}, r)
}

// A total is one Metric: the author's words and hint, the resolver's figure.
func TestLower_totalIsOneMetricWithTheWrittenLabelAndTheResolversFigure(t *testing.T) {
	got, err := lowerOne(t, metricTotal(map[string]string{"label": "Open", "hint": "Not finished yet"}), fixedTotal("7"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"label": "Open", "hint": "Not finished yet", "value": "7"}
	if len(got.Children) != 1 || !reflect.DeepEqual(got.Children[0].Props, want) {
		t.Errorf("got %+v, want one Metric with %v", got.Children, want)
	}
}

// tone is a signal: drawn when there is something to signal, dropped on a zero.
func TestLower_totalToneIsDrawnOnlyWhenTheFigureIsNotZero(t *testing.T) {
	for figure, wantTone := range map[string]string{"3": "bad", "0": ""} {
		got, err := lowerOne(t, metricTotal(map[string]string{"label": "Overdue", "tone": "bad"}), fixedTotal(figure))
		if err != nil {
			t.Fatal(err)
		}
		if tone := got.Children[0].Props["tone"]; tone != wantTone {
			t.Errorf("figure %s: tone = %q, want %q", figure, tone, wantTone)
		}
	}
}

func TestLower_refusesATotalThatIsNotWellFormed(t *testing.T) {
	cases := map[string]func(*domain.PageNode){
		"no label to name the figure": func(n *domain.PageNode) { delete(n.Props, "label") },
		"a typed value":               func(n *domain.PageNode) { n.Props["value"] = "99" },
		"no measure":                  func(n *domain.PageNode) { n.Binding.Measure = "" },
		"a destination":               func(n *domain.PageNode) { n.To = "nav_x"; n.Param = "p" },
		"a child":                     func(n *domain.PageNode) { n.Children = []domain.PageNode{{Kind: "static", Type: "paragraph"}} },
		"a from":                      func(n *domain.PageNode) { n.From = map[string]string{"hint": "title"} },
	}
	for name, mutate := range cases {
		n := metricTotal(map[string]string{"label": "Open"})
		mutate(&n)
		if _, err := lowerOne(t, n, fixedTotal("1")); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := lowerOne(t, metricTotal(map[string]string{"label": "Open"}), Resolver{}); err == nil {
		t.Error("a total with no resolver was accepted")
	}
	coll := listBound(rowTemplate())
	coll.Binding.Rows = domain.PageRowsTotal
	if _, err := lowerOne(t, coll, fixedTotal("1")); err == nil || !strings.Contains(err.Error(), "rows: records") {
		t.Errorf("a Collection bound with rows: total was accepted: %v", err)
	}
}

func transitionInTemplate(becomes string, ts []RecordTransition) (domain.PageNode, Resolver) {
	coll := domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection),
		Binding: &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{
			Kind: "component", Type: string(domain.ComponentButton),
			Props:   map[string]string{"label": "Go", "variant": "secondary", "becomes": becomes},
			Binding: &domain.PageBinding{Write: domain.PageWriteTransition},
		}},
	}
	res := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: []map[string]string{{"title": "a"}, {"title": "b"}}, Transitions: ts}, nil
	}}
	return formPage(coll), res
}

func transitionTo(route string, current string) RecordTransition {
	return RecordTransition{Route: route, Field: "fld_status", Done: "done", Reopen: "todo",
		Allows: func(target string) (bool, bool) {
			switch target {
			case "todo", "in_progress", "done":
				return target != current, true
			}
			return false, false
		}}
}

func TestLower_transitionButtonPatchesTheStatusFieldAndIsDrawnOnlyWhereTheMoveIsAllowed(t *testing.T) {
	ts := []RecordTransition{transitionTo("/machines/mch_x/records/r1", "todo"), transitionTo("/machines/mch_x/records/r2", "done")}
	root, res := transitionInTemplate(domain.PageTransitionDone, ts)
	got, err := Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Children[0].Children
	if len(items) != 1 {
		t.Fatalf("items = %d, want one: a finished record is not offered Mark done", len(items))
	}
	p := items[0].Props
	if p["action"] != "/machines/mch_x/records/r1" || p["method"] != domain.ButtonMethodPatch || p["name"] != "fld_status" || p["value"] != "done" || p["becomes"] != "" || p["confirm"] != "" {
		t.Errorf("button props = %v", p)
	}
	root, res = transitionInTemplate(domain.PageTransitionReopen, ts)
	got, err = Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	if items = got.Children[0].Children; len(items) != 1 || items[0].Props["action"] != "/machines/mch_x/records/r2" || items[0].Props["value"] != "todo" {
		t.Errorf("$reopen items = %+v, want only the finished record, writing the Machine's reopen value", items)
	}
	root, res = transitionInTemplate("in_progress", ts)
	if got, err = Lower(root, res); err != nil || len(got.Children[0].Children) != 2 {
		t.Errorf("a literal option: err = %v, items = %d, want both records", err, len(got.Children[0].Children))
	}
}

// TestLower_transitionButtonTakesItsRecordsNameForAssistiveTechnology: a list repeats one label per row, so the
// Button may take `describes` from a role, and from nothing else.
func TestLower_transitionButtonTakesItsRecordsNameForAssistiveTechnology(t *testing.T) {
	ts := []RecordTransition{transitionTo("/r1", "todo"), transitionTo("/r2", "todo")}
	root, res := transitionInTemplate(domain.PageTransitionDone, ts)
	root.Children[0].Children[0].From = map[string]string{"describes": "title"}
	got, err := Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	items := got.Children[0].Children
	if len(items) != 2 || items[0].Props["describes"] != "a" || items[1].Props["describes"] != "b" {
		t.Errorf("describes = %v / %v, want each record's own title", items[0].Props, items[1].Props)
	}
	for name, from := range map[string]map[string]string{
		"another prop":    {"label": "title"},
		"an unknown role": {"describes": "no_such_role"},
		"also written":    {"describes": "title"},
	} {
		root, res = transitionInTemplate(domain.PageTransitionDone, ts)
		btn := &root.Children[0].Children[0]
		btn.From = from
		if name == "also written" {
			btn.Props["describes"] = "typed"
		}
		if _, err := Lower(root, res); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestLower_refusesATransitionButtonThatCouldNotBeBuiltHonestly(t *testing.T) {
	ts := []RecordTransition{transitionTo("/r1", "todo"), transitionTo("/r2", "todo")}
	cases := map[string]struct {
		becomes string
		mutate  func(n *domain.PageNode)
		ts      []RecordTransition
		want    string
	}{
		"a dataset of its own": {"done", func(n *domain.PageNode) { n.Children[0].Binding.Dataset = "ds_b" }, ts, "no dataset"},
		"no becomes":           {"", func(n *domain.PageNode) { delete(n.Children[0].Props, "becomes") }, ts, "becomes"},
		"not an option":        {"blocked", func(n *domain.PageNode) {}, ts, "not an option"},
		"a confirm":            {"done", func(n *domain.PageNode) { n.Children[0].Props["confirm"] = "Sure?" }, ts, "confirm"},
		"typed name":           {"done", func(n *domain.PageNode) { n.Children[0].Props["name"] = "fld_x" }, ts, "name"},
		"typed value":          {"done", func(n *domain.PageNode) { n.Children[0].Props["value"] = "done" }, ts, "value"},
		"typed method":         {"done", func(n *domain.PageNode) { n.Children[0].Props["method"] = "patch" }, ts, "method"},
		"typed action":         {"done", func(n *domain.PageNode) { n.Children[0].Props["action"] = "/x" }, ts, "action"},
		"no resolver answer":   {"done", func(n *domain.PageNode) {}, nil, "no transition"},
		"no completion":        {domain.PageTransitionDone, func(n *domain.PageNode) {}, []RecordTransition{{Route: "/r", Field: "f", Allows: ts[0].Allows}, {Route: "/r", Field: "f", Allows: ts[0].Allows}}, "completion"},
	}
	for name, tc := range cases {
		root, res := transitionInTemplate(tc.becomes, tc.ts)
		tc.mutate(&root.Children[0])
		if _, err := Lower(root, res); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, tc.want)
		}
	}
	outside := domain.PageNode{Kind: "component", Type: string(domain.ComponentButton),
		Props:   map[string]string{"label": "Go", "variant": "secondary", "becomes": "done"},
		Binding: &domain.PageBinding{Write: domain.PageWriteTransition}}
	if _, err := Lower(formPage(outside), Resolver{}); err == nil || !strings.Contains(err.Error(), "item template") {
		t.Errorf("a transition Button outside a records template: err = %v", err)
	}
}

// whenTemplate is a records Collection whose item template is one Text node under a `when:`.
func whenTemplate(c *domain.PageCondition, set RecordSet) (domain.PageNode, Resolver) {
	coll := domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection),
		Binding:  &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "static", Type: "text", Props: map[string]string{"text": "hi"}, When: c}},
	}
	return formPage(coll), Resolver{Records: func(domain.PageBinding) (RecordSet, error) { return set, nil }}
}

func statusRecords(statuses ...string) []map[string]string {
	out := make([]map[string]string, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, map[string]string{"status": s})
	}
	return out
}

func TestLower_whenDropsTheNodeForTheRecordsItDoesNotHoldFor(t *testing.T) {
	set := RecordSet{Records: statusRecords("todo", "done", "doing"), Done: "done", Reopen: "todo"}
	count := func(c domain.PageCondition) int {
		root, res := whenTemplate(&c, set)
		got, err := Lower(root, res)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		return len(got.Children[0].Children)
	}
	for _, tc := range []struct {
		c    domain.PageCondition
		want int
	}{
		{domain.PageCondition{Role: "status", Is: domain.PageTransitionDone}, 1},
		{domain.PageCondition{Role: "status", IsNot: domain.PageTransitionDone}, 2},
		{domain.PageCondition{Role: "status", Is: domain.PageTransitionReopen}, 1},
		{domain.PageCondition{Role: "status", IsNot: domain.PageTransitionReopen}, 2},
		{domain.PageCondition{Role: "status", Is: "doing"}, 1},
		{domain.PageCondition{Role: "status", IsNot: "doing"}, 2},
	} {
		if got := count(tc.c); got != tc.want {
			t.Errorf("%+v: %d nodes, want %d", tc.c, got, tc.want)
		}
	}
}

func TestLower_whenIsRefusedWhereItCouldNotBeHonest(t *testing.T) {
	set := RecordSet{Records: statusRecords("todo"), Done: "done", Reopen: "todo"}
	for name, tc := range map[string]struct {
		c    domain.PageCondition
		set  RecordSet
		want string
	}{
		"both operands":    {domain.PageCondition{Role: "status", Is: "a", IsNot: "b"}, set, "exactly one"},
		"neither operand":  {domain.PageCondition{Role: "status"}, set, "exactly one"},
		"unprojected role": {domain.PageCondition{Role: "priority", Is: "a"}, set, "does not project"},
		"no completion":    {domain.PageCondition{Role: "status", Is: domain.PageTransitionDone}, RecordSet{Records: statusRecords("todo")}, "completion"},
	} {
		root, res := whenTemplate(&tc.c, tc.set)
		if _, err := Lower(root, res); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
	outside := domain.PageNode{Kind: "static", Type: "text", Props: map[string]string{"text": "hi"}, When: &domain.PageCondition{Role: "status", Is: "a"}}
	if _, err := Lower(formPage(outside), Resolver{}); err == nil || !strings.Contains(err.Error(), "item template") {
		t.Errorf("a when: outside a records template: %v", err)
	}
}

func TestLower_whenHoldsForEveryNodeInAShapeOnlySet(t *testing.T) {
	set := RecordSet{Records: statusRecords("x"), Done: "x", Reopen: "x", ShapeOnly: true}
	root, res := whenTemplate(&domain.PageCondition{Role: "status", IsNot: domain.PageTransitionDone}, set)
	got, err := Lower(root, res)
	if err != nil || len(got.Children[0].Children) != 1 {
		t.Errorf("a shape-only set must keep the node so its subtree is validated: err = %v", err)
	}
}

func hideEmptyList(extra map[string]string) domain.PageNode {
	bound := listBound(rowTemplate())
	bound.Props = map[string]string{"gap": "tight", "hide_empty": "true"}
	for k, v := range extra {
		bound.Props[k] = v
	}
	return bound
}

func TestLower_hideEmptyLeavesOutACollectionThatListsNothingAndNeverReachesTheNode(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		hideEmptyList(map[string]string{"heading": "Overdue"}),
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "kept"}},
	}}
	none, err := Lower(root, fixedRecords())
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Children) != 1 || none.Children[0].Type != "paragraph" {
		t.Errorf("a hide_empty Collection over no records was drawn: %+v", none.Children)
	}
	some, err := Lower(root, fixedRecords(map[string]string{"title": "t", "status": "s"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(some.Children) != 2 || some.Children[0].Props["heading"] != "Overdue" {
		t.Fatalf("a hide_empty Collection over a record was not drawn with its heading: %+v", some.Children)
	}
	if _, leaked := some.Children[0].Props["hide_empty"]; leaked {
		t.Errorf("hide_empty reached the lowered Collection: %+v", some.Children[0].Props)
	}
}

func TestLower_hideEmptyFaults(t *testing.T) {
	unbound := domain.PageNode{Kind: "component", Type: "Collection", Props: map[string]string{"gap": "tight", "hide_empty": "true"},
		Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	for name, c := range map[string]struct {
		node domain.PageNode
		want string
	}{
		"beside empty":             {hideEmptyList(map[string]string{"empty": "none"}), "opposite ways"},
		"not a boolean":            {hideEmptyList(map[string]string{"hide_empty": "yes"}), "not true or false"},
		"on an unbound Collection": {unbound, "bound with rows"},
	} {
		root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{c.node}}
		if _, err := Lower(root, fixedRecords()); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

func TestLower_layoutEmptySaysItsWordsOnlyWhenEverythingWasHidden(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"gap": "default", "empty": "Nothing assigned."},
		Children: []domain.PageNode{hideEmptyList(nil), hideEmptyList(nil)}}
	quiet, err := Lower(root, fixedRecords())
	if err != nil {
		t.Fatal(err)
	}
	if len(quiet.Children) != 1 || quiet.Children[0].Kind != NodeComponent || quiet.Children[0].Type != string(domain.ComponentEmptyState) || quiet.Children[0].Props["text"] != "Nothing assigned." {
		t.Fatalf("a layout whose children were all hidden did not say its words: %+v", quiet.Children)
	}
	if err := Validate(quiet); err != nil {
		t.Errorf("the quiet page does not validate: %v", err)
	}
	busy, err := Lower(root, fixedRecords(map[string]string{"title": "t", "status": "s"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range busy.Children {
		if c.Type == string(domain.ComponentEmptyState) {
			t.Errorf("the quiet message was drawn beside real content: %+v", busy.Children)
		}
	}
	if _, leaked := busy.Props["empty"]; leaked {
		t.Errorf("empty reached the lowered layout: %+v", busy.Props)
	}
}

func TestLower_layoutEmptyNeedsChildrenAndWords(t *testing.T) {
	for name, props := range map[string]map[string]string{"no words": {"empty": ""}} {
		root := domain.PageNode{Kind: "layout", Type: "stack", Props: props, Children: []domain.PageNode{hideEmptyList(nil)}}
		if _, err := Lower(root, fixedRecords()); err == nil || !strings.Contains(err.Error(), "no words") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	childless := domain.PageNode{Kind: "layout", Type: "stack", Props: map[string]string{"empty": "x"}}
	if _, err := Lower(childless, fixedRecords()); err == nil || !strings.Contains(err.Error(), "needs children") {
		t.Errorf("a childless layout with empty: = %v", err)
	}
}

func TestLower_hideEmptyFalseKeepsTheCollectionOverNoRecords(t *testing.T) {
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{hideEmptyList(map[string]string{"hide_empty": "false"})}}
	got, err := Lower(root, fixedRecords())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Children) != 1 {
		t.Errorf("hide_empty: false hid the Collection: %+v", got.Children)
	}
}

func TestLower_aLayoutOfOnlyHideEmptyCollectionsNeedsItsOwnWords(t *testing.T) {
	flat := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{hideEmptyList(nil), hideEmptyList(nil)}}
	nested := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}},
		{Kind: "layout", Type: "stack", Children: []domain.PageNode{hideEmptyList(nil)}}}}
	for name, root := range map[string]domain.PageNode{"flat": flat, "nested": nested} {
		if _, err := Lower(root, fixedRecords()); err == nil || !strings.Contains(err.Error(), "write empty:") {
			t.Errorf("%s: err = %v; want it to ask for empty:", name, err)
		}
	}
	mixed := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{hideEmptyList(nil), {Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	if _, err := Lower(mixed, fixedRecords()); err != nil {
		t.Errorf("a layout that also holds fixed content was refused: %v", err)
	}
}

func groupedList() domain.PageNode {
	n := listBound(domain.PageNode{Kind: "static", Type: "subheading", From: map[string]string{"text": domain.PageBucketLabelRole}})
	n.Binding.Group = domain.PageGroupDate
	n.Props = map[string]string{"gap": "tight", "empty": "—"}
	n.Children = append(n.Children, rowTemplate())
	return domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "tight"}, Children: []domain.PageNode{n}}
}

func groupedRecords(buckets []Bucket, keys []string, recs ...map[string]string) Resolver {
	return Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{Records: recs, Buckets: buckets, BucketKeys: keys}, nil
	}}
}

// A grouped Collection becomes one section per bucket, as siblings in the parent, each holding the heading and
// the bucket's own list. The empty bucket is drawn with the author's `empty:` words, and the records stay in the
// order the resolver gave them inside the bucket they name.
func TestLowerGroupedDrawsEveryBucketOnceWithItsOwnRecords(t *testing.T) {
	days := []Bucket{{Key: "d1", Label: "Mon 05 Oct"}, {Key: "d2", Label: "Tue 06 Oct"}, {Key: "d3", Label: "Wed 07 Oct"}}
	got, err := Lower(groupedList(), groupedRecords(days, []string{"d3", "d1", "d3"},
		map[string]string{"title": "c", "status": "s"}, map[string]string{"title": "a", "status": "s"}, map[string]string{"title": "d", "status": "s"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Children) != 3 {
		t.Fatalf("a grid of three buckets lowered to %d children", len(got.Children))
	}
	for i, want := range []struct {
		label string
		items int
	}{{"Mon 05 Oct", 1}, {"Tue 06 Oct", 0}, {"Wed 07 Oct", 2}} {
		cell := got.Children[i]
		if cell.Kind != NodeLayout || cell.Type != string(domain.LayoutSection) || len(cell.Children) != 2 {
			t.Fatalf("bucket %d is not a section of heading + list: %+v", i, cell)
		}
		if cell.Children[0].Props["text"] != want.label {
			t.Errorf("bucket %d heading = %q, want %q", i, cell.Children[0].Props["text"], want.label)
		}
		list := cell.Children[1]
		if len(list.Children) != want.items {
			t.Errorf("bucket %d holds %d records, want %d", i, len(list.Children), want.items)
		}
		if list.Props["empty"] != "—" {
			t.Errorf("bucket %d lost the author's empty words: %+v", i, list.Props)
		}
	}
	if first := got.Children[2].Children[1].Children[0].Children[0].Props["text"]; first != "c" {
		t.Errorf("a bucket's records lost the resolver's order: first is %q, want c", first)
	}
}

func TestLowerGroupedRefusesWhatItCannotHonour(t *testing.T) {
	days := []Bucket{{Key: "d1", Label: "Mon"}}
	one := map[string]string{"title": "a", "status": "s"}
	with := func(edit func(*domain.PageNode)) domain.PageNode {
		root := groupedList()
		edit(&root.Children[0])
		return root
	}
	for name, c := range map[string]struct {
		root domain.PageNode
		res  Resolver
		want string
	}{
		"a record outside every bucket": {groupedList(), groupedRecords(days, []string{"elsewhere"}, one), "does not draw"},
		"keys and records disagree":     {groupedList(), groupedRecords(days, nil, one), "bucket keys"},
		"two buckets of one key":        {groupedList(), groupedRecords([]Bucket{{Key: "d1", Label: "a"}, {Key: "d1", Label: "b"}}, nil), "twice"},
		"complete":                      {with(func(n *domain.PageNode) { n.Props["complete"] = "true" }), groupedRecords(days, nil), "not built for a grouped"},
		"hide_empty":                    {with(func(n *domain.PageNode) { n.Props["hide_empty"] = "true" }), groupedRecords(days, nil), "not built for a grouped"},
		"one child":                     {with(func(n *domain.PageNode) { n.Children = n.Children[:1] }), groupedRecords(days, nil), "exactly two"},
		"an unknown dimension":          {with(func(n *domain.PageNode) { n.Binding.Group = "status" }), groupedRecords(days, nil), "not a dimension"},
		"a record link in the heading": {with(func(n *domain.PageNode) {
			n.Children[0] = domain.PageNode{Kind: "static", Type: "link", From: map[string]string{"text": domain.PageBucketLabelRole, "href": domain.PageRecordRole}}
		}), groupedRecords(days, nil), "has none"},
		"the bucket's name in a record's template": {with(func(n *domain.PageNode) {
			n.Children[1] = domain.PageNode{Kind: "static", Type: "paragraph", From: map[string]string{"text": domain.PageBucketLabelRole}}
		}), groupedRecords(days, []string{"d1"}, one), "bucket:label"},
	} {
		_, err := Lower(c.root, c.res)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error mentioning %q, got %v", name, c.want, err)
		}
	}
}

// A window that was cut short by its Dataset's limit is a week missing tasks, and the page says so once, above the
// first day, without the author claiming anything. A window that was not cut carries nothing.
func TestLowerGroupedNamesTheBoundOnlyWhenItBit(t *testing.T) {
	days := []Bucket{{Key: "d1", Label: "Mon"}, {Key: "d2", Label: "Tue"}}
	for _, truncated := range []bool{false, true} {
		res := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
			return RecordSet{Records: []map[string]string{{"title": "a", "status": "s"}}, Buckets: days, BucketKeys: []string{"d2"}, Truncated: truncated, Limit: 200}, nil
		}}
		got, err := Lower(groupedList(), res)
		if err != nil {
			t.Fatal(err)
		}
		first, second := got.Children[0].Children[1], got.Children[1].Children[1]
		if want := map[bool]string{false: "", true: "200"}[truncated]; first.Props["truncated"] != want || second.Props["truncated"] != "" {
			t.Errorf("truncated=%v: first list says %q, second says %q; want %q on the first only", truncated, first.Props["truncated"], second.Props["truncated"], want)
		}
	}
}

// No window to draw (a Dataset whose parameter was not sent lists nothing) is an empty list carrying the author's
// `empty:` words, not a page body with nothing in it.
func TestLowerGroupedWithNoWindowDrawsTheEmptyList(t *testing.T) {
	got, err := Lower(groupedList(), Resolver{Records: func(domain.PageBinding) (RecordSet, error) { return RecordSet{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Children) != 1 || got.Children[0].Type != string(domain.ComponentCollection) || got.Children[0].Props["empty"] != "\u2014" && got.Children[0].Props["empty"] != "—" {
		t.Fatalf("children = %+v, want one empty Collection carrying the empty: words", got.Children)
	}
}

// Each bucket's section is the tight gap, and a write or a tag lands on the record it belongs to however the
// records are spread across buckets (the resolver's index is the record's, not its place in the bucket).
func TestLowerGroupedKeepsEachRecordsWritesAndTagsAndTheSectionGap(t *testing.T) {
	root := groupedList()
	root.Children[0].Children[1] = domain.PageNode{Kind: "layout", Type: "row", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}},
		{Kind: "component", Type: string(domain.ComponentButton), Props: map[string]string{"label": "Go", "variant": "secondary", "becomes": "in_progress"}, Binding: &domain.PageBinding{Write: domain.PageWriteTransition}},
	}}
	ts := []RecordTransition{transitionTo("/r/0", "todo"), transitionTo("/r/1", "todo"), transitionTo("/r/2", "todo")}
	res := Resolver{Records: func(domain.PageBinding) (RecordSet, error) {
		return RecordSet{
			Records:     []map[string]string{{"title": "c"}, {"title": "a"}, {"title": "b"}},
			Transitions: ts, Buckets: []Bucket{{Key: "d1", Label: "Mon"}, {Key: "d2", Label: "Tue"}}, BucketKeys: []string{"d2", "d1", "d2"},
		}, nil
	}}
	got, err := Lower(root, res)
	if err != nil {
		t.Fatal(err)
	}
	route := func(item UINode) string { return item.Children[1].Props["action"] }
	if g := got.Children[0].Props["gap"]; g != string(domain.GapTight) {
		t.Errorf("section gap = %q, want %q", g, domain.GapTight)
	}
	mon, tue := got.Children[0].Children[1].Children, got.Children[1].Children[1].Children
	if len(mon) != 1 || route(mon[0]) != "/r/1" || len(tue) != 2 || route(tue[0]) != "/r/0" || route(tue[1]) != "/r/2" {
		t.Errorf("writes landed on the wrong records: Mon %v, Tue %v", mon, tue)
	}
}

// A bucket the resolver marks current is a section that says so, and only that one; the prop is produced by
// lowering and an author who writes it is refused (a page cannot know which day it is read on).
func TestLowerGroupedMarksOnlyTheCurrentBucket(t *testing.T) {
	days := []Bucket{{Key: "d1", Label: "Mon"}, {Key: "d2", Label: "Tue", Current: true}, {Key: "d3", Label: "Wed"}}
	got, err := Lower(groupedList(), groupedRecords(days, nil))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"", "true", ""} {
		if v := got.Children[i].Props[SectionCurrentProp]; v != want {
			t.Errorf("bucket %d current = %q, want %q", i, v, want)
		}
	}
}

func TestLowerRefusesAnAuthoredCurrentSection(t *testing.T) {
	page := domain.PageNode{Kind: "layout", Type: "section", Props: map[string]string{"current": "true"}, Children: []domain.PageNode{{Kind: "static", Type: "note", Props: map[string]string{"text": "x"}}}}
	if _, err := Lower(page, Resolver{}); err == nil || !strings.Contains(err.Error(), "never written") {
		t.Errorf("an authored current: want a refusal, got %v", err)
	}
}

func TestValidateRefusesACurrentSectionThatIsNotTrue(t *testing.T) {
	n := UINode{Kind: NodeLayout, Type: "section", Props: map[string]string{"current": "yes"}, Children: []UINode{{Kind: NodeStatic, Type: "note", Props: map[string]string{"text": "x"}}}}
	if err := Validate(n); err == nil {
		t.Error("current: yes was accepted")
	}
	n.Props["current"] = "true"
	if err := Validate(n); err != nil {
		t.Errorf("current: true was refused: %v", err)
	}
}

func stepLink(step *domain.PageStep) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "link", To: "nav_x", Step: step, Props: map[string]string{"text": "go"}}}}
}

func weekRequest(week int) Resolver {
	r := navFixture("/cal")
	r.Parameter = func(string) string { return strconv.Itoa(week) }
	r.Step = func(_ string, by int) string {
		if week+by == 0 {
			return ""
		}
		return strconv.Itoa(week + by)
	}
	return r
}

func TestLowerStepSendsTheParameterOneStepOnFromTheRequestsOwn(t *testing.T) {
	for name, c := range map[string]struct {
		week, by int
		want     string
	}{
		"forward":              {0, 1, "/cal?week=1"},
		"back":                 {0, -1, "/cal?week=-1"},
		"on from a later week": {3, 1, "/cal?week=4"},
		"back to this week is the plain route, not ?week=0": {1, -1, "/cal"},
	} {
		tree, err := Lower(stepLink(&domain.PageStep{Parameter: "week", By: c.by}), weekRequest(c.week))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := tree.Children[0].Props["href"]; got != c.want {
			t.Errorf("%s: href = %q, want %q", name, got, c.want)
		}
	}
}

func TestLowerRefusesAStepThatCouldNotBeHonest(t *testing.T) {
	notALink := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", Step: &domain.PageStep{Parameter: "week", By: 1}}}}
	noDestination := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "link", ListOf: "ds_x", Step: &domain.PageStep{Parameter: "week", By: 1}, Props: map[string]string{"text": "go"}}}}
	for name, c := range map[string]struct {
		root domain.PageNode
		want string
	}{
		"zero":                      {stepLink(&domain.PageStep{Parameter: "week"}), "other than zero"},
		"a parameter nothing names": {stepLink(&domain.PageStep{Parameter: "page", By: 1}), "week"},
		"on a node that is no link": {notALink, "static: link"},
		"on a link with no to:":     {noDestination, "to:"},
	} {
		if _, err := Lower(c.root, weekRequest(0)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

func parameterCondition(is, isNot string) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "shown"},
			When: &domain.PageCondition{Parameter: "week", Is: is, IsNot: isNot}}}}
}

func TestLowerParameterConditionFollowsTheRequestAndNeedsNoRecord(t *testing.T) {
	for name, c := range map[string]struct {
		week      int
		is, isNot string
		wantDrawn bool
	}{
		"is_not 0 away from this week": {2, "", "0", true},
		"is_not 0 on this week":        {0, "", "0", false},
		"is 0 on this week":            {0, "0", "", true},
		"is 0 away from it":            {-1, "0", "", false},
		"an operand spelt 00 is 0":     {0, "", "00", false},
	} {
		tree, err := Lower(parameterCondition(c.is, c.isNot), weekRequest(c.week))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if drawn := len(tree.Children) == 1; drawn != c.wantDrawn {
			t.Errorf("%s: drawn = %v, want %v", name, drawn, c.wantDrawn)
		}
	}
}

func TestLowerParameterConditionHoldsWhenOnlyTheShapeIsBeingChecked(t *testing.T) {
	tree, err := Lower(parameterCondition("", "0"), Resolver{})
	if err != nil || len(tree.Children) != 1 {
		t.Errorf("err = %v, children = %d; a shape-only lowering must draw the node so it is validated", err, len(tree.Children))
	}
}

func TestLowerRefusesAParameterConditionThatCouldNotBeHonest(t *testing.T) {
	both := parameterCondition("", "0")
	both.Children[0].When.Role = "status"
	other := parameterCondition("", "0")
	other.Children[0].When.Parameter = "page"
	for name, c := range map[string]struct {
		root domain.PageNode
		want string
	}{
		"a role beside a parameter":  {both, "not both"},
		"a parameter nothing names":  {other, "week"},
		"an operand that is no week": {parameterCondition("", "soon"), "whole number"},
		"both comparisons":           {parameterCondition("0", "0"), "exactly one"},
	} {
		if _, err := Lower(c.root, weekRequest(0)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

func windowNode(kind string, props map[string]string, count *domain.PageCount) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: kind, Type: "caption", Window: "ds_w", Props: props, Count: count}}}
}

func TestLowerWindowSaysTheDatasetsWindowInWordsAndWritesNoText(t *testing.T) {
	r := Resolver{Window: func(id string) (string, error) { return "words for " + id, nil }}
	tree, err := Lower(windowNode("static", nil, nil), r)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Children[0].Props["text"]; got != "words for ds_w" {
		t.Errorf("text = %q", got)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a window's static must validate: %v", issues)
	}
	failing := Resolver{Window: func(string) (string, error) { return "", fmt.Errorf("no range") }}
	for name, c := range map[string]struct {
		root domain.PageNode
		res  Resolver
		want string
	}{
		"text also written":  {windowNode("static", map[string]string{"text": "x"}, nil), r, "window:"},
		"a component":        {windowNode("component", nil, nil), r, "static node"},
		"no resolver":        {windowNode("static", nil, nil), Resolver{}, "resolver"},
		"a resolver failure": {windowNode("static", nil, nil), failing, "no range"},
	} {
		if _, err := Lower(c.root, c.res); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", name, err, c.want)
		}
	}
}

// TestLower_aLinkIntoAnotherApplicationIsDrawnOnlyWhereItResolves: `in:` names the other Application, and the link is not an
// error when this Workspace does not offer it to this viewer -- it is simply not drawn; the same link without in: still
// fails loudly for a destination that does not exist.
func TestLower_aLinkIntoAnotherApplicationIsDrawnOnlyWhereItResolves(t *testing.T) {
	foreign := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
		{Kind: "static", Type: "link", To: "nav_new_plan", In: "app_payments", Props: map[string]string{"text": "Create payment plan"}}}}
	offered := Resolver{Route: NavigationRoutes([]domain.NavigationItem{{ID: domain.ForeignLinkID("app_payments", "nav_new_plan"), Label: "New plan", Route: "/pages/nav_new_plan"}})}
	tree, err := Lower(foreign, offered)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Children[0].Props; got["href"] != "/pages/nav_new_plan" || got["text"] != "Create payment plan" {
		t.Errorf("an offered foreign link = %v, want href /pages/nav_new_plan", got)
	}
	tree, err = Lower(foreign, navFixture("/somewhere"))
	if err != nil {
		t.Fatalf("a link into an Application not offered here was an error: %v", err)
	}
	if len(tree.Children) != 0 {
		t.Errorf("a link into an Application not offered here was drawn: %+v", tree.Children)
	}
	own := linkTo("nav_gone", nil)
	if _, err := Lower(own, navFixture("/somewhere")); err == nil {
		t.Error("a link to a missing screen of the page's own Application was accepted")
	}
}

// TestLower_aSlashInToIsRefused: another Application's screens are named "<app>/<item>" inside the runtime; a page may not
// reach them by writing that in to:, which would skip the load check in: gets.
func TestLower_aSlashInToIsRefused(t *testing.T) {
	_, err := Lower(linkTo("app_x/nav_y", nil), navFixture("/somewhere"))
	if err == nil || !strings.Contains(err.Error(), "in: <application id>") {
		t.Errorf("a slashed to: = %v", err)
	}
}

// TestLower_aPrintButtonSendsNothing: `write: print` is a Button that opens the browser's print dialog -- valid anywhere on a
// page, with no Dataset, no record, no question and no route.
func TestLower_aPrintButtonSendsNothing(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentButton)})
	button := func(props map[string]string, binding domain.PageBinding) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
			{Kind: "component", Type: "Button", Props: props, Binding: &binding}}}
	}
	tree, err := Lower(button(map[string]string{"label": "Print", "variant": "secondary"}, domain.PageBinding{Write: domain.PageWritePrint}), Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	got := tree.Children[0].Props
	if got["method"] != domain.ButtonMethodPrint || got["action"] != "" || got["label"] != "Print" {
		t.Errorf("a print Button = %v, want method print with no action", got)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a lowered print Button is not valid: %v", issues)
	}
	for name, tc := range map[string]struct {
		props   map[string]string
		binding domain.PageBinding
		want    string
	}{
		"a dataset": {map[string]string{"label": "Print"}, domain.PageBinding{Write: domain.PageWritePrint, Dataset: "ds_x"}, "takes no dataset"},
		"a confirm": {map[string]string{"label": "Print", "confirm": "Sure?"}, domain.PageBinding{Write: domain.PageWritePrint}, "sends no request"},
		"an action": {map[string]string{"label": "Print", "action": "/machines/x"}, domain.PageBinding{Write: domain.PageWritePrint}, "never typed"},
	} {
		if _, err := Lower(button(tc.props, tc.binding), Resolver{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error saying %q", name, err, tc.want)
		}
	}
}

// TestLower_aPDFButtonAsksForThePageAndIsNotOfferedWhereNothingCanPrint: like Print it names no Dataset and sends no
// typed request, and a host that cannot print gets no button at all.
func TestLower_aPDFButtonAsksForThePageAndIsNotOfferedWhereNothingCanPrint(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentButton)})
	button := func(props map[string]string, binding domain.PageBinding) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{
			{Kind: "component", Type: "Button", Props: props, Binding: &binding}}}
	}
	pdf := domain.PageBinding{Write: domain.PageWritePDF}
	tree, err := Lower(button(map[string]string{"label": "Download PDF"}, pdf), Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Children[0].Props; got["method"] != domain.ButtonMethodPDF || got["action"] != "" {
		t.Errorf("a pdf Button = %v, want method pdf with no action", got)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Errorf("a lowered pdf Button is not valid: %v", issues)
	}
	for _, can := range []bool{true, false} {
		tree, err := Lower(button(map[string]string{"label": "Download PDF"}, pdf), Resolver{PDF: func() bool { return can }})
		if err != nil {
			t.Fatal(err)
		}
		if drawn := len(tree.Children) == 1; drawn != can {
			t.Errorf("a host that can print = %v, but the button drawn = %v", can, drawn)
		}
	}
	for name, tc := range map[string]struct {
		props   map[string]string
		binding domain.PageBinding
		want    string
	}{
		"a dataset": {map[string]string{"label": "PDF"}, domain.PageBinding{Write: domain.PageWritePDF, Dataset: "ds_x"}, "takes no dataset"},
		"a confirm": {map[string]string{"label": "PDF", "confirm": "Sure?"}, pdf, "takes no confirm"},
		"an action": {map[string]string{"label": "PDF", "action": "/machines/x"}, pdf, "never typed"},
	} {
		if _, err := Lower(button(tc.props, tc.binding), Resolver{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error saying %q", name, err, tc.want)
		}
	}
}

// TestLower_aFormThatReadsADocumentDrawsItsReaderItsNotesAndItsRows: with a reader the Form is preceded by a small form that
// posts a file to the page; a proposal's notes and flags reach the Fields, the document is named, the proposal rides as a hidden
// pair, and each proposed row is one grid of controls named rows.<kind>.<n>.<field> behind its own tick.
func TestLower_aFormThatReadsADocumentDrawsItsReaderItsNotesAndItsRows(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentForm), string(domain.ComponentField), string(domain.ComponentFormInput)})
	RegisterSlottedComponentTypes([]string{string(domain.ComponentForm), string(domain.ComponentField)})
	spec := FormSpec{
		Route: "/pages/nav_new/extract/save", Permitted: true, Carries: "proposal=ext_1", Document: "Document: ctr.pdf · 1.4 MB",
		Reader: &FormReader{Route: "/pages/nav_new/extract", Read: "Read document", Label: "Document"},
		Inputs: []domain.FormInput{
			{FieldID: "fld_party", Label: "Party", Kind: domain.InputText, Default: "Reza", Note: "p. 1 · title"},
			{FieldID: "fld_kind", Label: "Type", Kind: domain.InputSelect, Options: []string{"Cast"}, Default: "Cast", Flag: "needs review"},
		},
		Rows: []FormRows{{Label: "Instalments", Rows: [][]domain.FormInput{
			{{FieldID: "fld_gross", Label: "Gross", Kind: domain.InputNumber, Default: "54"}},
			{{FieldID: "fld_gross", Label: "Gross", Kind: domain.InputNumber, Default: "72"}},
		}}},
	}
	node := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "component", Type: "Form",
		Props: map[string]string{"submit": "Save"}, Binding: &domain.PageBinding{Dataset: "ds_x", Write: domain.PageWriteCreate}}}}
	tree, err := Lower(node, Resolver{Form: func(domain.PageBinding) (FormSpec, error) { return spec, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if issues := Validate(tree); len(issues) != 0 {
		t.Fatalf("the tree is not valid: %v", issues)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("%d nodes, want the reader and the form", len(tree.Children))
	}
	reader, form := tree.Children[0], tree.Children[1]
	if reader.Props["action"] != "/pages/nav_new/extract" || reader.Props["submit"] != "Read document" || reader.Props["multipart"] != "true" {
		t.Errorf("reader = %v", reader.Props)
	}
	if in := reader.Children[0].Children[0].Props; in["name"] != domain.ExtractFileParam || in["kind"] != string(domain.InputFile) {
		t.Errorf("the reader's control = %v", in)
	}
	if form.Props["carries"] != "proposal=ext_1" || form.Props["action"] != "/pages/nav_new/extract/save" {
		t.Errorf("form = %v", form.Props)
	}
	if form.Children[0].Kind != NodeStatic || form.Children[0].Props["text"] != "Document: ctr.pdf · 1.4 MB" {
		t.Errorf("the document is not named first: %+v", form.Children[0])
	}
	if f := form.Children[1].Props; f["note"] != "p. 1 · title" || f["flag"] != "" {
		t.Errorf("party field = %v", f)
	}
	if f := form.Children[2].Props; f["flag"] != "needs review" {
		t.Errorf("type field = %v", f)
	}
	grid := form.Children[3]
	if grid.Type != string(domain.LayoutGrid) || len(grid.Children) != 2 {
		t.Fatalf("first row = %+v", grid)
	}
	if n := grid.Children[0].Children[0].Props; n["name"] != "rows.0.0.include" || n["kind"] != string(domain.InputBoolean) {
		t.Errorf("the row's tick = %v", n)
	}
	if n := grid.Children[1].Children[0].Props; n["name"] != "rows.0.0.fld_gross" || n["value"] != "54" {
		t.Errorf("the row's control = %v", n)
	}
	if n := form.Children[4].Children[1].Children[0].Props["name"]; n != "rows.0.1.fld_gross" {
		t.Errorf("second row's control = %q", n)
	}
	seen := map[string]bool{}
	var ids func(UINode)
	ids = func(n UINode) {
		if id := n.Props["id"]; id != "" {
			if seen[id] {
				t.Errorf("the control id %q is used twice", id)
			}
			seen[id] = true
		}
		for _, c := range n.Children {
			ids(c)
		}
	}
	ids(tree)
	// A reader of nothing is not drawn.
	spec.Reader = nil
	tree, _ = Lower(node, Resolver{Form: func(domain.PageBinding) (FormSpec, error) { return spec, nil }})
	if len(tree.Children) != 1 {
		t.Errorf("without a reader the form stands alone, got %d nodes", len(tree.Children))
	}
}
