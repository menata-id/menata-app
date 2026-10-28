package web

import "testing"

func TestWantsCode_onlyTheSelectedPartOnTheCodeTab(t *testing.T) {
	for _, c := range []struct {
		id, selected, tab string
		want              bool
	}{
		{"a/x", "a/x", "Code", true},
		{"a/x", "a/x", "Details", false},
		{"a/y", "a/x", "Code", false},
		{"", "", "Code", false},
	} {
		if got := wantsCode(c.id, c.selected, c.tab); got != c.want {
			t.Errorf("wantsCode(%q, %q, %q) = %v, want %v", c.id, c.selected, c.tab, got, c.want)
		}
	}
}
