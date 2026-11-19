package aiassist

import (
	"strings"
	"testing"
)

// The menu keys an update may set on an item that already exists: a parent and a count are changes, who is offered the item may change but not be emptied again,
// and a new item cannot carry them (it is added first).
func TestPlanMenuKeys(t *testing.T) {
	cur := []GeneratedMenuItem{{ID: "nav_a", Label: "A", MachineID: "mch_x"}, {ID: "nav_b", Label: "B", MachineID: "mch_x", VisibleTo: []string{"manager"}}}
	plan := func(desired []GeneratedMenuItem) Plan {
		var p Plan
		planMenu(&p, cur, desired)
		return p
	}
	with := func(i int, edit func(*GeneratedMenuItem)) []GeneratedMenuItem {
		out := append([]GeneratedMenuItem(nil), cur...)
		edit(&out[i])
		return out
	}
	if p := plan(with(0, func(n *GeneratedMenuItem) { n.Parent, n.Badge = "nav_b", "ds_open" })); len(p.Refusals) != 0 || len(p.Items) != 2 {
		t.Errorf("hanging an item under another and giving it a count: %+v", p)
	}
	if p := plan(with(1, func(n *GeneratedMenuItem) { n.VisibleTo = []string{"manager", "reviewer"} })); len(p.Refusals) != 0 || len(p.Items) != 1 {
		t.Errorf("widening who is offered an item: %+v", p)
	}
	if p := plan(with(1, func(n *GeneratedMenuItem) { n.VisibleTo = nil })); len(p.Refusals) != 1 || !strings.Contains(p.Refusals[0].Message, "cannot be offered to everyone again") {
		t.Errorf("emptying who is offered an item: %+v", p)
	}
	added := append(append([]GeneratedMenuItem(nil), cur...), GeneratedMenuItem{Label: "C", MachineID: "mch_y", Parent: "nav_a"})
	if p := plan(added); len(p.Refusals) != 1 || !strings.Contains(p.Refusals[0].Message, "is new") {
		t.Errorf("a new item carrying a parent: %+v", p)
	}
}

// A list is the same list whether it names no columns as nil or as an empty list, and whether the default order is written or left out.
func TestSameListIgnoresEmptyVersusNilAndDefaultOrder(t *testing.T) {
	a := &GeneratedList{Scope: ListScopeAll, Limit: 10}
	b := &GeneratedList{Scope: ListScopeAll, Limit: 10, Columns: []string{}, SearchFields: []string{}, SortField: "created_at", Direction: "desc"}
	if !sameList(a, b) {
		t.Error("empty slices or the default order made a list a different list")
	}
	if sameList(a, &GeneratedList{Scope: ListScopeAll, Limit: 10, Direction: "asc"}) || sameList(a, nil) {
		t.Error("a real difference was ignored")
	}
}

// A list returned with an empty list of columns where it was read with none is not "changed" in the plan.
func TestPlanMenuDoesNotCallAnEchoedListChanged(t *testing.T) {
	cur := []GeneratedMenuItem{{ID: "nav_a", Label: "A", MachineID: "mch_x", List: &GeneratedList{Scope: ListScopeAll, Limit: 10}}}
	desired := []GeneratedMenuItem{{ID: "nav_a", Label: "A", MachineID: "mch_x", List: &GeneratedList{Scope: ListScopeAll, Limit: 10, Columns: []string{}, SearchFields: []string{}}}}
	var p Plan
	planMenu(&p, cur, desired)
	if len(p.Items) != 0 || len(p.Refusals) != 0 {
		t.Errorf("an echoed list is a change: %+v", p)
	}
}

// A refusal must not send the model to a second refusal. An item that opens a screen the model cannot name (no Machine) cannot be removed, so the
// refusal of pointing it elsewhere must not say "remove it"; an item that opens a Machine can be removed, so the advice to remove it is real.
// (The log of 2026-10-27: one session alternated between "remove it and add a new one" and "cannot be removed" until it gave up.)
func TestMenuRefusalsDoNotContradictEachOther(t *testing.T) {
	cur := []GeneratedMenuItem{
		{ID: "nav_screen", Label: "My Tasks"},
		{ID: "nav_machine", Label: "Tasks", MachineID: "mch_task"},
	}
	plan := func(desired []GeneratedMenuItem) Plan {
		var p Plan
		planMenu(&p, cur, desired)
		return p
	}
	repoint := plan([]GeneratedMenuItem{{ID: "nav_screen", Label: "My Tasks", MachineID: "mch_task"}, cur[1]})
	if len(repoint.Refusals) != 1 || strings.Contains(repoint.Refusals[0].Message, "remove it") || !strings.Contains(repoint.Refusals[0].Message, "cannot be pointed somewhere else") || !strings.Contains(repoint.Refusals[0].Message, "add a new menu item that opens mch_task") {
		t.Errorf("repointing a screen item: %+v", repoint.Refusals)
	}
	drop := plan([]GeneratedMenuItem{cur[1]})
	if len(drop.Refusals) != 1 || !strings.Contains(drop.Refusals[0].Message, "keep it in the menu") {
		t.Errorf("removing a screen item: %+v", drop.Refusals)
	}
	// The advice given for an item that opens a Machine is feasible: removing it is allowed.
	if p := plan([]GeneratedMenuItem{cur[0]}); len(p.Refusals) != 0 {
		t.Errorf("an item that opens a Machine could not be removed, yet the refusal of repointing it says to remove it: %+v", p.Refusals)
	}
	if p := plan([]GeneratedMenuItem{cur[0], {ID: "nav_machine", Label: "Tasks", MachineID: "mch_other"}}); len(p.Refusals) != 1 || !strings.Contains(p.Refusals[0].Message, "remove it and add a new one") {
		t.Errorf("repointing a Machine item: %+v", p.Refusals)
	}
}
