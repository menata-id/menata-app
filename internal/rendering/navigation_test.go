package rendering

import (
	"testing"

	"menata.app/internal/domain"
)

func TestNavSections_splitsUngroupedPrimaryAndRest(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_dashboard", Label: "Dashboard", Route: "/dashboard", Group: "Document Approval", Priority: 1},
		{ID: "nav_my_tasks", Label: "My Tasks", Route: "/my-tasks", Group: "Project Management", Priority: 1},
	}

	ungrouped, primary, rest := navSections(items, "Document Approval")

	if len(ungrouped) != 1 || ungrouped[0].ID != "nav_home" {
		t.Errorf("ungrouped = %+v, want [nav_home]", ungrouped)
	}
	if primary.Label != "Document Approval" || len(primary.Items) != 1 || primary.Items[0].ID != "nav_dashboard" {
		t.Errorf("primary = %+v, want Document Approval containing nav_dashboard", primary)
	}
	if len(rest) != 1 || rest[0].Label != "Project Management" {
		t.Errorf("rest = %+v, want [Project Management]", rest)
	}
}

func TestNavSections_noNamedGroupsLeavesPrimaryAndRestEmpty(t *testing.T) {
	items := []domain.NavigationItem{{ID: "nav_home", Label: "Home", Route: "/home"}}

	ungrouped, primary, rest := navSections(items, "")

	if len(ungrouped) != 1 {
		t.Errorf("ungrouped = %+v, want 1 item", ungrouped)
	}
	if primary.Label != "" || len(primary.Items) != 0 {
		t.Errorf("primary = %+v, want zero value", primary)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %+v, want empty", rest)
	}
}

// TestNavSections_hiddenPrimaryGroupPromotesNothing is a regression test for the 2026-09-19
// owner-reported bug: hiding Document Approval (this Application's declared-first, primary
// group) via hidden_nav_groups had silently promoted Project Management -- previously collapsed
// behind a dropdown -- into the always-open primary slot, making the topbar look bigger, not
// smaller. primaryLabel here is what domain.Application.PrimaryNavGroup would hold ("Document
// Approval", decided before hiding); items is what's left after hiding removed that group's own
// entries. No group may take its place.
func TestNavSections_hiddenPrimaryGroupPromotesNothing(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_my_tasks", Label: "My Tasks", Route: "/my-tasks", Group: "Project Management", Priority: 1},
	}

	ungrouped, primary, rest := navSections(items, "Document Approval")

	if len(ungrouped) != 1 || ungrouped[0].ID != "nav_home" {
		t.Errorf("ungrouped = %+v, want [nav_home]", ungrouped)
	}
	if primary.Label != "" || len(primary.Items) != 0 {
		t.Errorf("primary = %+v, want zero value -- Project Management must not be promoted", primary)
	}
	if len(rest) != 1 || rest[0].Label != "Project Management" {
		t.Errorf("rest = %+v, want [Project Management] (collapsed, not primary)", rest)
	}
}

// TestRouteByID_survivesHiddenNavGroup is routeByID's own version of
// TestNavSections_hiddenPrimaryGroupPromotesNothing's regression: allNavigation must keep
// resolving an item's route even after hidden_nav_groups has already dropped it from the
// (filtered) navigation ConfigureNavigation's first argument carries -- the whole reason
// AllNavigation exists as a field distinct from Navigation (domain.Application's own doc
// comment).
func TestRouteByID_survivesHiddenNavGroup(t *testing.T) {
	all := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox", Group: "Document Approval"},
	}
	filtered := []domain.NavigationItem{all[0]} // Document Approval hidden from the topbar
	t.Cleanup(func() { ConfigureNavigation(nil, "", nil) })
	ConfigureNavigation(filtered, "", all)

	if got := routeByID("nav_approval_inbox"); got != "/approval-inbox" {
		t.Errorf("routeByID(nav_approval_inbox) after its group is hidden = %q, want /approval-inbox still resolvable", got)
	}
}

func TestRouteByID_unknownIDPanics(t *testing.T) {
	t.Cleanup(func() { ConfigureNavigation(nil, "", nil) })
	ConfigureNavigation(nil, "", nil)

	defer func() {
		if recover() == nil {
			t.Error("routeByID(unknown id) did not panic, want a loud failure on a programmer error, not a silently broken link")
		}
	}()
	routeByID("nav_does_not_exist")
}
