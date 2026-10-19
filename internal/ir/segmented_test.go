package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func segmented(props map[string]string) domain.PageNode {
	return domain.PageNode{Kind: "component", Type: "SegmentedControl", Props: props}
}

var rangeOptions = map[string]string{"label": "Range", "query": "range", "options": "week=Week\nmonth=Month"}

// A SegmentedControl on a page takes its state from the request, not from the page: the choice the request carried is the selected one (the first
// option when it carried none or one that is no option), and every other parameter is kept so choosing changes only its own.
func TestLowerSegmentedControl_takesItsStateFromTheRequestAndKeepsTheRest(t *testing.T) {
	for name, tc := range map[string]struct {
		params map[string]string
		want   string
		keep   string
	}{
		"none carried":       {map[string]string{}, "week", ""},
		"the second":         {map[string]string{"range": "month"}, "month", ""},
		"not an option":      {map[string]string{"range": "year"}, "week", ""},
		"others are kept":    {map[string]string{"range": "month", "project": "rec_1", "week": "2"}, "month", "project=rec_1&week=2"},
		"application is not": {map[string]string{"application": "app_x", "project": "rec_1"}, "week", "project=rec_1"},
	} {
		r := Resolver{Params: func() map[string]string { return tc.params }, Query: func(n string) string { return tc.params[n] }}
		out, err := lowerSegmentedControl(segmented(rangeOptions), r, "p")
		if err != nil || len(out) != 1 {
			t.Fatalf("%s: %v %v", name, out, err)
		}
		if got := out[0].Props["selected"]; got != tc.want {
			t.Errorf("%s: selected = %q, want %q", name, got, tc.want)
		}
		if got := out[0].Props["keep"]; got != tc.keep {
			t.Errorf("%s: keep = %q, want %q", name, got, tc.keep)
		}
	}
}

func TestLowerSegmentedControl_refusesWhatTheRequestAloneKnows(t *testing.T) {
	for name, extra := range map[string]map[string]string{"selected": {"selected": "month"}, "keep": {"keep": "project=x"}} {
		props := map[string]string{}
		for k, v := range rangeOptions {
			props[k] = v
		}
		for k, v := range extra {
			props[k] = v
		}
		if _, err := lowerSegmentedControl(segmented(props), Resolver{}, "p"); err == nil || !strings.Contains(err.Error(), "never written") {
			t.Errorf("a written %s: error = %v, want a refusal", name, err)
		}
	}
	one := map[string]string{"label": "Range", "query": "range", "options": "week=Week"}
	if _, err := lowerSegmentedControl(segmented(one), Resolver{}, "p"); err == nil || !strings.Contains(err.Error(), "fewer than two") {
		t.Errorf("one option: error = %v, want at least two", err)
	}
	// The load-time placeholder has no request: the first option is the one shown.
	out, err := lowerSegmentedControl(segmented(rangeOptions), Resolver{}, "p")
	if err != nil || out[0].Props["selected"] != "week" {
		t.Errorf("placeholder: %v %v", out, err)
	}
}
