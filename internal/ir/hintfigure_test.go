package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func totalMetric(hint *domain.PageCount, props map[string]string) domain.PageNode {
	p := map[string]string{"label": "All cards"}
	for k, v := range props {
		p[k] = v
	}
	return domain.PageNode{Kind: "component", Type: string(domain.ComponentMetric), Props: p, HintFigure: hint,
		Binding: &domain.PageBinding{Dataset: "ds_a", Measure: "msr_all", Rows: domain.PageRowsTotal}}
}

func totals(figures map[string]string) Resolver {
	return Resolver{Total: func(b domain.PageBinding) (string, error) { return figures[b.Measure], nil }}
}

// The hint carries another Measure's figure from the same resolver, worded for one or for the rest.
func TestLower_hintFigureWordsTheHintAroundAnotherMeasure(t *testing.T) {
	h := &domain.PageCount{Of: "msr_projects", One: "In {n} project", Other: "In {n} projects"}
	for figure, want := range map[string]string{"1": "In 1 project", "2": "In 2 projects", "0": "In 0 projects"} {
		got, err := Lower(inStack(totalMetric(h, nil)), totals(map[string]string{"msr_all": "11", "msr_projects": figure}))
		if err != nil {
			t.Fatal(err)
		}
		if p := got.Children[0].Props; p["hint"] != want || p["value"] != "11" {
			t.Errorf("figure %s: props = %v, want hint %q beside value 11", figure, p, want)
		}
	}
}

func TestLower_hintFigureRefusals(t *testing.T) {
	r := totals(map[string]string{"msr_all": "1", "msr_projects": "2"})
	h := &domain.PageCount{Of: "msr_projects", Other: "In {n} projects"}
	onDimension := totalMetric(h, nil)
	onDimension.Binding = &domain.PageBinding{Dataset: "ds_a", Measure: "msr_all", Rows: domain.PageRowsDimension}
	delete(onDimension.Props, "label")
	unbound := domain.PageNode{Kind: "static", Type: "caption", Props: map[string]string{"text": "x"}, HintFigure: h}
	for name, tc := range map[string]struct {
		root domain.PageNode
		want string
	}{
		"a written hint beside it": {totalMetric(h, map[string]string{"hint": "typed"}), "may not also be written"},
		"no {n} in other":          {totalMetric(&domain.PageCount{Of: "msr_projects", Other: "In projects"}, nil), "must carry {n}"},
		"no of":                    {totalMetric(&domain.PageCount{Other: "In {n}"}, nil), "of: names the Measure"},
		"on a dimension Metric":    {onDimension, "rows: total"},
		"on an unbound node":       {unbound, "rows: total"},
	} {
		if _, err := Lower(inStack(tc.root), r); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want one mentioning %q", name, err, tc.want)
		}
	}
}

// With no `one:`, a figure of 1 is worded by `other` -- the installed "{n}% of all cards" at exactly 1% must say
// "1% of all cards", never an empty hint.
func TestLower_hintFigureWithoutOneUsesOtherForOne(t *testing.T) {
	h := &domain.PageCount{Of: "msr_share", Other: "{n}% of all cards"}
	got, err := Lower(inStack(totalMetric(h, nil)), totals(map[string]string{"msr_all": "100", "msr_share": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if hint := got.Children[0].Props["hint"]; hint != "1% of all cards" {
		t.Errorf("hint = %q, want %q", hint, "1% of all cards")
	}
}
