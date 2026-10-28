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
	if p := plan(with(1, func(n *GeneratedMenuItem) { n.VisibleTo = nil })); len(p.Refusals) != 1 || !strings.Contains(p.Refusals[0], "cannot be offered to everyone again") {
		t.Errorf("emptying who is offered an item: %+v", p)
	}
	added := append(append([]GeneratedMenuItem(nil), cur...), GeneratedMenuItem{Label: "C", MachineID: "mch_x", Parent: "nav_a"})
	if p := plan(added); len(p.Refusals) != 1 || !strings.Contains(p.Refusals[0], "is new") {
		t.Errorf("a new item carrying a parent: %+v", p)
	}
}
