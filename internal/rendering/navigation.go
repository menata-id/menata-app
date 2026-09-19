package rendering

import (
	"menata.app/internal/domain"
	"menata.app/internal/experience"
)

// navSections splits the Application's own declared menu (experience.GroupNavigation) into the
// three parts pageShell renders differently: the ungrouped items (e.g. Home -- always visible,
// no label), the one group named primaryLabel (this Application's own current-priority menu --
// shown inline and labeled, never collapsed) and every other named group (shown behind a
// <details> dropdown, since most sessions won't need them on every page). primaryLabel is
// decided once at metadata-load time from the full declared order (domain.Application.
// PrimaryNavGroup), not by whichever group happens to be first in items here -- so a group
// hidden_nav_groups removed can never promote a different group into its place (owner
// correction, 2026-09-19: the previous "first group encountered" rule did exactly that when
// Document Approval was hidden, silently expanding Project Management from a collapsed dropdown
// to always-open). This is a presentation choice (which group stays open), not a derivation one,
// so it lives here rather than in internal/experience.
func navSections(items []domain.NavigationItem, primaryLabel string) (ungrouped []domain.NavigationItem, primary experience.NavGroup, rest []experience.NavGroup) {
	for _, g := range experience.GroupNavigation(items) {
		switch g.Label {
		case "":
			ungrouped = g.Items
		case primaryLabel:
			primary = g
		default:
			rest = append(rest, g)
		}
	}
	return
}
