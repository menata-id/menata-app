package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func progressTemplate() domain.PageNode {
	return domain.PageNode{Kind: "component", Type: string(domain.ComponentProgress), From: map[string]string{
		"label": domain.PageRowLabelRole, "value": domain.PageMeasureRole("msr_open"), "max": domain.PageMeasureRole("msr_all"),
	}}
}

func inStack(n domain.PageNode) domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{n}}
}

func dimensionList(template domain.PageNode) domain.PageNode {
	return domain.PageNode{
		Kind: "component", Type: string(domain.ComponentCollection), Props: map[string]string{"gap": "tight", "divided": "true"},
		Binding:  &domain.PageBinding{Dataset: "ds_a", Rows: domain.PageRowsDimension},
		Children: []domain.PageNode{template},
	}
}

// A Collection bound to dimension rows clones its one child per row, in the resolver's order, each reading the
// row's display and any of its Measures -- and keeps its own properties.
func TestLower_dimensionCollectionIsOneItemPerRowReadingItsMeasures(t *testing.T) {
	root, err := Lower(inStack(dimensionList(progressTemplate())), fixedRows(
		Row{Label: "Silvia", Measures: map[string]string{"msr_open": "2", "msr_all": "3"}},
		Row{Label: "Rina", Measures: map[string]string{"msr_open": "0", "msr_all": "1"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	got := root.Children[0]
	if got.Type != string(domain.ComponentCollection) || got.Props["divided"] != "true" || len(got.Children) != 2 {
		t.Fatalf("tree = %+v", got)
	}
	first, second := got.Children[0].Props, got.Children[1].Props
	if first["label"] != "Silvia" || first["value"] != "2" || first["max"] != "3" || second["label"] != "Rina" || second["value"] != "0" {
		t.Errorf("rows = %v, %v", first, second)
	}
}

func TestLower_dimensionCollectionRefusesWhatOnlyARecordCouldAnswer(t *testing.T) {
	rows := fixedRows(Row{Label: "x", Measures: map[string]string{"msr_open": "1", "msr_all": "1"}})
	link := domain.PageNode{Kind: "static", Type: "link", From: map[string]string{"text": domain.PageRowLabelRole, "href": domain.PageRecordRole}}
	write := domain.PageNode{Kind: "component", Type: "Button", Props: map[string]string{"label": "x", "confirm": "y"}, Binding: &domain.PageBinding{Write: domain.PageWriteDelete}}
	when := progressTemplate()
	when.When = &domain.PageCondition{Role: domain.PageRowLabelRole, Is: "x"}
	count := domain.PageNode{Kind: "static", Type: "caption", Count: &domain.PageCount{Of: "rel_a", One: "{n}", Other: "{n}"}}
	measureOnList := dimensionList(progressTemplate())
	measureOnList.Binding.Measure = "msr_open"
	typo := progressTemplate()
	typo.From["max"] = domain.PageMeasureRole("msr_nope")
	for name, tc := range map[string]struct {
		root domain.PageNode
		want string
	}{
		"a record link":         {dimensionList(link), "record's address"},
		"a write":               {dimensionList(write), "binding inside a dimension row"},
		"a condition":           {dimensionList(when), "when:"},
		"a count":               {dimensionList(count), "count:"},
		"a measure on the list": {measureOnList, "takes no measure"},
		"an unknown measure":    {dimensionList(typo), "msr_nope"},
		"two children": {func() domain.PageNode {
			n := dimensionList(progressTemplate())
			n.Children = append(n.Children, progressTemplate())
			return n
		}(), "exactly one child"},
	} {
		if _, err := Lower(inStack(tc.root), rows); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want one mentioning %q", name, err, tc.want)
		}
	}
}
