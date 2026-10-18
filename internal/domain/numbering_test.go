package domain

import "testing"

// TestNumberingPatternsAreParsedAndWritten: "CTR/{kind}/{seq:3}" is literal text, a Field's value (or its code), the year,
// and one counter last, written N digits wide; every other shape is an error naming what is wrong.
func TestNumberingPatternsAreParsedAndWritten(t *testing.T) {
	n := FieldNumbering{Pattern: "CTR/{fld_kind}/{seq:3}", Codes: map[string]string{"Cast": "C", "Crew": "K"}}
	for name, tc := range map[string]struct {
		values map[string]any
		want   string
	}{
		"a coded value":              {map[string]any{"fld_kind": "Crew"}, "CTR/K/"},
		"an uncoded one":             {map[string]any{"fld_kind": "Item"}, "CTR/Item/"},
		"padded with a space around": {map[string]any{"fld_kind": " Cast "}, "CTR/C/"},
	} {
		prefix, width, err := n.NumberPrefix(tc.values, 2026)
		if err != nil || prefix != tc.want || width != 3 {
			t.Errorf("%s: prefix %q width %d err %v, want %q width 3", name, prefix, width, err, tc.want)
		}
	}
	if _, _, err := n.NumberPrefix(map[string]any{}, 2026); err == nil {
		t.Error("a number written from a Field with no value")
	}
	year := FieldNumbering{Pattern: "PO/{year}/{seq:4}"}
	if prefix, width, _ := year.NumberPrefix(nil, 2026); prefix != "PO/2026/" || width != 4 {
		t.Errorf("year pattern = %q width %d", prefix, width)
	}
	if got := Number("PP-", 4, 146); got != "PP-0146" {
		t.Errorf("Number = %q", got)
	}
	if got := Number("PP-", 2, 1234); got != "PP-1234" {
		t.Errorf("a counter past its width = %q, want it written whole", got)
	}
	for pattern, want := range map[string]string{
		"CTR/{seq:3}/x":    "comes last",
		"CTR/":             "needs one {seq:N}",
		"CTR/{seq:0}":      "N from 1 to 9",
		"CTR/{seq:10}":     "N from 1 to 9",
		"CTR/{fld_x":       "never closed",
		"CTR/{a b}{seq:2}": "not a token",
		"CTR/{}{seq:2}":    "not a token",
	} {
		if _, _, err := ParseNumbering(pattern); err == nil || !contains(err.Error(), want) {
			t.Errorf("%q: %v, want an error saying %q", pattern, err, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
