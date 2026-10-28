package rendering

import (
	"testing"

	"menata.app/internal/domain"
)

func nested() []domain.NavigationItem {
	return []domain.NavigationItem{
		{ID: "nav_orders", Label: "Orders", Route: "/pages/nav_orders", Priority: 1},
		{ID: "nav_pending", Label: "Pending", Route: "/pages/nav_pending", Priority: 2, Parent: "nav_orders"},
		{ID: "nav_archive", Label: "Archive", Route: "/pages/nav_archive", Priority: 3, Parent: "nav_orders"},
		{ID: "nav_reports", Label: "Reports", Route: "/pages/nav_reports", Priority: 4},
	}
}

// An item hung under another (`parent:`) is drawn in its parent's own list: the row of tabs holds only the top-level items, the parent carries its
// children in menu order, and when a child is the page the child is the one entry marked as the page while its parent is marked as the place you are in.
func TestAChildItemLivesUnderItsParentAndMarksItInside(t *testing.T) {
	menu := defaultAppMenu(nested(), "/pages/nav_archive", "")
	if len(menu) != 2 || menu[0].ID != "nav_orders" || menu[1].ID != "nav_reports" {
		t.Fatalf("the tabs are %+v, want Orders and Reports", menu)
	}
	if got := menu[0].Children; len(got) != 2 || got[0].ID != "nav_pending" || got[1].ID != "nav_archive" {
		t.Fatalf("Orders' children are %+v, want Pending then Archive", got)
	}
	if menu[0].Active || !menu[0].Within || !menu[0].Children[1].Active || menu[0].Children[0].Active || menu[1].Active {
		t.Errorf("exactly the child should be the page and its parent within: %+v", menu)
	}
	if got := activeMenuNode(menu); got != "navigation/nav_archive" {
		t.Errorf("the page's metadata path is %q, want the child's", got)
	}
	// Nothing matches: the first tab stands in, and no child is marked.
	stand := defaultAppMenu(nested(), "/elsewhere", "")
	if !stand[0].Active || !stand[0].Fallback || stand[0].Children[0].Active {
		t.Errorf("fallback = %+v", stand[0])
	}
}

// A phone's sheet and the desktop's More list have no second level, so a child is a row of its own reading "Parent › Child": the sheet holds the rest of the
// menu with its children, then the children of the three tabs the bar draws.
func TestTheSheetFlattensChildrenAsParentArrowChild(t *testing.T) {
	items := nested()
	items = append(items,
		domain.NavigationItem{ID: "nav_a", Label: "A", Route: "/a", Priority: 5},
		domain.NavigationItem{ID: "nav_b", Label: "B", Route: "/b", Priority: 6},
		domain.NavigationItem{ID: "nav_b1", Label: "B one", Route: "/b1", Priority: 7, Parent: "nav_b"})
	menu := defaultAppMenu(items, "/pages/nav_archive", "")
	// Top level: Orders, Reports, A, B -- the bar draws the first three; B is overflow with its child.
	var labels []string
	for _, r := range bottomBarOverflow(menu) {
		labels = append(labels, r.Label)
	}
	want := []string{"B", "B › B one", "Orders › Pending", "Orders › Archive"}
	if len(labels) != len(want) {
		t.Fatalf("sheet rows = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("sheet row %d = %q, want %q (all: %v)", i, labels[i], want[i], labels)
		}
	}
	if got := bottomBarItems(menu); len(got) != 3 || got[0].ID != "nav_orders" {
		t.Errorf("the bar's tabs are %+v, want the first three top-level items", got)
	}
}
