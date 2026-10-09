package ir

import (
	"errors"
	"reflect"
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
	got, err := Lower(root, fixedRows(Row{"b", "2"}, Row{"a", "1"}))
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
			if _, err := Lower(wrap(n), fixedRows(Row{"a", "1"})); err == nil {
				t.Fatal("Lower accepted it")
			}
		})
	}
}

func TestLower_rootMustBeOneUnboundNode(t *testing.T) {
	if _, err := Lower(metricBound(nil), fixedRows(Row{"a", "1"})); err == nil || !strings.Contains(err.Error(), "root") {
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
				Rows:    func(domain.PageBinding) ([]Row, error) { return []Row{{"a", "1"}}, nil },
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
