package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// A menu has one level under an item: a parent is another item of the list that has no parent itself (which also rules out a cycle), never the item itself,
// and a settings hub is not nested.
func TestValidateNavigationAllowsOneLevelUnderAnItem(t *testing.T) {
	item := func(id, parent string) domain.NavigationItem {
		return domain.NavigationItem{ID: id, Label: id, Route: "/" + id, Parent: parent}
	}
	if issues := validateNavigation([]domain.NavigationItem{item("nav_a", ""), item("nav_b", "nav_a"), item("nav_c", "nav_a")}); len(issues) != 0 {
		t.Errorf("one level was refused: %v", issues)
	}
	hub := item("nav_hub", "")
	hub.SettingsHub = true
	for name, tc := range map[string]struct {
		items []domain.NavigationItem
		want  string
	}{
		"the item itself":     {[]domain.NavigationItem{item("nav_a", "nav_a")}, "names the item itself"},
		"an item not there":   {[]domain.NavigationItem{item("nav_a", "nav_gone")}, "is not an item of this list"},
		"two levels":          {[]domain.NavigationItem{item("nav_a", ""), item("nav_b", "nav_a"), item("nav_c", "nav_b")}, "one level under an item"},
		"a cycle":             {[]domain.NavigationItem{item("nav_a", "nav_b"), item("nav_b", "nav_a")}, "one level under an item"},
		"a hub as the parent": {[]domain.NavigationItem{hub, item("nav_a", "nav_hub")}, "settings hub"},
	} {
		issues := strings.Join(validateNavigation(tc.items), "\n")
		if !strings.Contains(issues, tc.want) {
			t.Errorf("%s: issues %q lack %q", name, issues, tc.want)
		}
	}
}
