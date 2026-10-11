package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// lower() routes any node with a Binding to lowerBound before lowerWindow and
// lowerLink run, so step:/window: on a bound component are silently dropped instead of refused. writing-guide.md
// says both are "valid only on" a link-with-to / a static node.
func TestStepAndWindowOnABoundComponentAreRefusedNotDropped(t *testing.T) {
	res := fixedRows(Row{Label: "a", Value: "1"})
	res.Window = func(string) (string, error) { return "W", nil }
	for name, mutate := range map[string]func(*domain.PageNode){
		"step":   func(n *domain.PageNode) { n.Step = &domain.PageStep{Parameter: "week", By: 1} },
		"window": func(n *domain.PageNode) { n.Window = "ds_w" },
	} {
		m := metricBound(map[string]string{"tone": "warn"})
		mutate(&m)
		if _, err := Lower(domain.PageNode{Kind: "layout", Type: "grid", Children: []domain.PageNode{m}}, res); err == nil {
			t.Errorf("%s: on a bound Metric was silently accepted", name)
		}
	}
}

// the authored-current refusal looks only at n.Props, so `from: {current: role}`
// on a section inside a records template lets record data set aria-current.
func TestCurrentCannotArriveThroughFrom(t *testing.T) {
	sec := domain.PageNode{Kind: "layout", Type: "section", From: map[string]string{"current": "title"},
		Children: []domain.PageNode{{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}}}}
	root := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{listBound(sec)}}
	got, err := Lower(root, fixedRecords(map[string]string{"title": "true"}))
	if err == nil && strings.Contains(strings.ReplaceAll(strings.Join(propsOf(got), " "), " ", ""), "current:true") {
		t.Errorf("a record value became a section's current prop: %+v", got)
	}
}

func propsOf(n UINode) []string {
	var out []string
	for k, v := range n.Props {
		out = append(out, k+":"+v)
	}
	for _, c := range n.Children {
		out = append(out, propsOf(c)...)
	}
	return out
}
