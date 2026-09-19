package rendering

import (
	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

// navSections splits the Application's own declared menu (experience.GroupNavigation) into the
// three parts pageShell renders differently: the ungrouped items (e.g. Home -- always visible,
// no label), the first named group (this Application's own current-priority menu, by
// declaration order in navigation: -- shown inline and labeled, never collapsed) and every
// other named group (shown behind a <details> dropdown, since most sessions won't need them on
// every page). This is a presentation choice (which group stays open), not a derivation one, so
// it lives here rather than in internal/experience -- nothing here names a specific Group value,
// so it stays correct however metadata's own navigation: list is ordered.
func navSections(items []domain.NavigationItem) (ungrouped []domain.NavigationItem, primary experience.NavGroup, rest []experience.NavGroup) {
	primarySet := false
	for _, g := range experience.GroupNavigation(items) {
		switch {
		case g.Label == "":
			ungrouped = g.Items
		case !primarySet:
			primary = g
			primarySet = true
		default:
			rest = append(rest, g)
		}
	}
	return
}
