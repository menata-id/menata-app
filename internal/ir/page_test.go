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
	if first.Props["required"] != "true" || f.Children[0].Props["label"] != "Name" {
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
