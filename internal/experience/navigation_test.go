package experience

import (
	"testing"

	"menata.app/internal/domain"
)

func TestGroupNavigation_ungroupedFirstRegardlessOfPriority(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_dashboard", Label: "Dashboard", Group: "Document Approval", Priority: 0},
		{ID: "nav_home", Label: "Home", Priority: 5},
	}

	groups := GroupNavigation(items)

	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if groups[0].Label != "" || len(groups[0].Items) != 1 || groups[0].Items[0].ID != "nav_home" {
		t.Errorf("groups[0] = %+v, want the ungrouped bucket containing nav_home", groups[0])
	}
	if groups[1].Label != "Document Approval" || len(groups[1].Items) != 1 || groups[1].Items[0].ID != "nav_dashboard" {
		t.Errorf("groups[1] = %+v, want Document Approval containing nav_dashboard", groups[1])
	}
}

func TestGroupNavigation_ordersWithinGroupByPriorityThenDeclaration(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_a", Group: "g", Priority: 2},
		{ID: "nav_b", Group: "g", Priority: 1},
		{ID: "nav_c", Group: "g", Priority: 1},
	}

	groups := GroupNavigation(items)

	if len(groups) != 1 || groups[0].Label != "g" {
		t.Fatalf("groups = %+v, want one group %q", groups, "g")
	}
	got := []string{groups[0].Items[0].ID, groups[0].Items[1].ID, groups[0].Items[2].ID}
	want := []string{"nav_b", "nav_c", "nav_a"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("groups[0].Items order = %v, want %v", got, want)
			break
		}
	}
}

func TestGroupNavigation_noUngroupedItemsOmitsEmptyBucket(t *testing.T) {
	items := []domain.NavigationItem{{ID: "nav_a", Group: "g"}}

	groups := GroupNavigation(items)

	if len(groups) != 1 || groups[0].Label != "g" {
		t.Fatalf("groups = %+v, want exactly one group %q, no leading empty bucket", groups, "g")
	}
}

func TestGroupNavigation_preservesFirstAppearanceGroupOrder(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_a", Group: "second", Priority: 1},
		{ID: "nav_b", Group: "first", Priority: 2},
		{ID: "nav_c", Group: "second", Priority: 3},
	}

	groups := GroupNavigation(items)

	if len(groups) != 2 || groups[0].Label != "second" || groups[1].Label != "first" {
		t.Fatalf("groups = %+v, want [second, first] in first-appearance order", groups)
	}
}
