package domain

import "testing"

func TestView_EffectiveType(t *testing.T) {
	if got := (View{}).EffectiveType(); got != ViewTable {
		t.Errorf("EffectiveType() = %q, want table for the zero value", got)
	}
	if got := (View{Type: ViewBoard}).EffectiveType(); got != ViewBoard {
		t.Errorf("EffectiveType() = %q, want board", got)
	}
}

// A Machine declaring no views: must keep behaving exactly as every Machine did before views
// existed -- one implicit table. That is the whole compatibility claim of DefaultView's zero
// value, so it is asserted rather than assumed.
func TestMachine_DefaultView(t *testing.T) {
	var none Machine
	if got := none.DefaultView().EffectiveType(); got != ViewTable {
		t.Errorf("DefaultView() = %q, want an implicit table for a Machine declaring none", got)
	}

	m := &Machine{Views: []View{
		{ID: "vw_board", Type: ViewBoard, GroupBy: "fld_list"},
		{ID: "vw_table", Type: ViewTable},
	}}
	if got := m.DefaultView().ID; got != "vw_board" {
		t.Errorf("DefaultView() = %q, want the first declared view", got)
	}
}

func TestMachine_ViewByID(t *testing.T) {
	m := &Machine{Views: []View{{ID: "vw_table", Type: ViewTable}, {ID: "vw_cards", Type: ViewCards}}}

	v, ok := m.ViewByID("vw_cards")
	if !ok || v.Type != ViewCards {
		t.Errorf("ViewByID(vw_cards) = %+v, %v; want the cards view", v, ok)
	}
	// An id the Machine does not declare must report itself missing rather than resolve to
	// something plausible -- the handler turns this into a 404 instead of quietly rendering a
	// different arrangement than the one that was asked for.
	if _, ok := m.ViewByID("vw_nope"); ok {
		t.Error("ViewByID(vw_nope) = ok; want not found for an undeclared id")
	}
}
