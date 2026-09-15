package domain

import "testing"

func TestView_EffectiveLayout(t *testing.T) {
	if got := (View{}).EffectiveLayout(); got != LayoutTable {
		t.Errorf("EffectiveLayout() = %q, want table for the zero value", got)
	}
	if got := (View{Layout: LayoutBoard}).EffectiveLayout(); got != LayoutBoard {
		t.Errorf("EffectiveLayout() = %q, want board", got)
	}
}
