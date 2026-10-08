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
	return Resolver{Records: func(domain.PageBinding) ([]map[string]string, error) { return recs, nil }}
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
				Records: func(domain.PageBinding) ([]map[string]string, error) { return []map[string]string{rec}, nil },
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
