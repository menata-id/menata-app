package experience

import (
	"sort"

	"menata.app/internal/domain"
)

// NavGroup is one section of the topbar: the ungrouped items that render flat at the top level
// (Label == "") or one Application-declared submenu (e.g. "Document Approval") rendered as a
// dropdown. Mirrors GroupRecords' own "columns in first-appearance order" shape (ROADMAP.md
// Phase 5), applied to the Application's own navigation: list instead of a Machine's records.
type NavGroup struct {
	Label string
	Items []domain.NavigationItem
}

// GroupNavigation orders items by Priority (stable, so declaration order breaks ties) and
// buckets them by their own Group field. The ungrouped bucket (Group == "") is always returned
// first regardless of its items' own Priority numbers, so a metadata author choosing priorities
// for one group can't accidentally reorder an unrelated group -- "Home" belongs at the front
// because it has no Group, not because of a priority race across groups. A Group with no items
// is never returned.
func GroupNavigation(items []domain.NavigationItem) []NavGroup {
	sorted := make([]domain.NavigationItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	groups := []NavGroup{{}}
	index := map[string]int{"": 0}
	for _, item := range sorted {
		idx, ok := index[item.Group]
		if !ok {
			idx = len(groups)
			index[item.Group] = idx
			groups = append(groups, NavGroup{Label: item.Group})
		}
		groups[idx].Items = append(groups[idx].Items, item)
	}
	if len(groups[0].Items) == 0 {
		groups = groups[1:]
	}
	return groups
}
