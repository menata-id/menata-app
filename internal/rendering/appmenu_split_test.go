package rendering

import "testing"

func TestSplitAppMenu(t *testing.T) {
	mk := func(n int) []navMenuItem {
		items := make([]navMenuItem, n)
		for i := range items {
			items[i] = navMenuItem{Label: string(rune('A' + i))}
		}
		return items
	}
	for _, c := range []struct{ n, tabs, overflow int }{{0, 0, 0}, {3, 3, 0}, {appMenuTabLimit, appMenuTabLimit, 0}, {appMenuTabLimit + 1, appMenuTabLimit, 1}, {9, appMenuTabLimit, 9 - appMenuTabLimit}} {
		tabs, overflow := splitAppMenu(mk(c.n))
		if len(tabs) != c.tabs || len(overflow) != c.overflow {
			t.Errorf("%d items: got %d tabs + %d overflow, want %d + %d", c.n, len(tabs), len(overflow), c.tabs, c.overflow)
		}
	}
}

func TestAnyActive(t *testing.T) {
	if anyActive([]navMenuItem{{}, {}}) {
		t.Error("no item is active")
	}
	if !anyActive([]navMenuItem{{}, {Active: true}}) {
		t.Error("the second item is active")
	}
}
