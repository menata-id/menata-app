package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestFilterChoicesCannotBeForgedByTheData: a Dimension row's label is data (a related record's title), and a tab or a
// line break in it must not start a chip of its own with an address the data chose.
func TestFilterChoicesCannotBeForgedByTheData(t *testing.T) {
	RegisterComponentTypes([]string{string(domain.ComponentFilterChips), string(domain.ComponentFilterSelect)})
	evil := "x\t9\tjavascript:alert(1)\t1\nPwn"
	r := Resolver{
		Rows:  func(domain.PageBinding) ([]Row, error) { return []Row{{Label: evil, Key: "rec_1", Value: "2"}}, nil },
		Total: func(domain.PageBinding) (string, error) { return "2", nil },
		Query: func(string) string { return "" },
	}
	b := &domain.PageBinding{Dataset: "ds", Measure: "msr", Rows: domain.PageRowsDimension}
	chips, err := lowerFilterChips(domain.PageNode{Kind: "component", Type: "FilterChips", Props: map[string]string{"label": "L", "query": "k", "all": "All"}, Binding: b}, r, "p")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(chips[0].Props["chips"], "\n")
	if len(lines) != 2 {
		t.Fatalf("the data added a chip: %q", chips[0].Props["chips"])
	}
	for _, l := range lines {
		if f := strings.Split(l, "\t"); len(f) != 4 || !strings.HasPrefix(f[2], "?") {
			t.Errorf("chip line %q is not label, count, ?address, flag", l)
		}
	}
	sel, err := lowerFilterSelect(domain.PageNode{Kind: "component", Type: "FilterSelect", Props: map[string]string{"label": "L", "query": "k", "all": "All"}, Binding: b}, r, "p")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(sel[0].Props["choices"], "\n"); n != 0 {
		t.Errorf("the data added a choice: %q", sel[0].Props["choices"])
	}
}
