package domain

import "testing"

func TestWorkspaceLinkTargetsAreItsOwnPagesTheRuntimesScreensAndEveryApplicationsMenu(t *testing.T) {
	ws := Workspace{
		Navigation:   []NavigationItem{{ID: "nav_ws_page", Route: "/pages/nav_ws_page"}},
		Applications: []Application{{ID: "app_a", AllNavigation: []NavigationItem{{ID: "nav_a_one"}, {ID: "nav_a_two"}}}, {ID: "app_b", AllNavigation: []NavigationItem{{ID: "nav_b_one"}}}},
	}
	got := map[string]bool{}
	for _, n := range ws.LinkTargets() {
		got[n.ID] = true
	}
	for _, want := range []string{"nav_ws_page", "nav_home", "nav_workspace_members", "nav_a_one", "nav_a_two", "nav_b_one"} {
		if !got[want] {
			t.Errorf("LinkTargets lacks %q", want)
		}
	}
}
