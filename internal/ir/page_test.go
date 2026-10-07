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

func fixedRows(rows ...Row) RowResolver {
	return func(domain.PageBinding) ([]Row, error) { return rows, nil }
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
	_, err := Lower(root, func(domain.PageBinding) ([]Row, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}
