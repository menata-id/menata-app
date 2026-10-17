package domain

import "testing"

func TestStatusToneFollowsDeclaredCategories(t *testing.T) {
	m := &Machine{StatusCategories: &StatusCategories{Field: "fld_status", Values: map[string]StatusCategory{
		"open": StatusActive, "late": StatusAttention, "closed": StatusDone, "dropped": StatusStopped,
	}}}
	for option, want := range map[string]BadgeTone{
		"open": ToneInfo, "late": ToneWarn, "closed": ToneGood, "dropped": ToneBad, "other": ToneNeutral,
	} {
		if got := m.StatusTone("fld_status", option); got != want {
			t.Errorf("%s: tone %q, want %q", option, got, want)
		}
	}
	if got := m.StatusTone("fld_other", "open"); got != ToneNeutral {
		t.Errorf("a category belongs to its own Field only, got %q", got)
	}
	if got := (&Machine{}).StatusTone("fld_status", "open"); got != ToneNeutral {
		t.Errorf("no categories declared must stay neutral, got %q", got)
	}
}
