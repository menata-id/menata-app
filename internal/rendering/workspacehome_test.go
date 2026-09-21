package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// workspaceHomeLinksTo renders WorkspaceHomePage with the given workspaceRole and every other
// parameter fixed to a minimal valid fixture, reporting whether the rendered page links to route.
// WorkspaceHomePage resolves every route and label it renders through routeByID/labelByID
// (machine.templ), so a nav fixture must be configured before rendering, same pattern
// TestRouteByID_survivesHiddenNavGroup (navigation_test.go) already uses. Four items are needed:
// the page's own title and breadcrumb (nav_home), its "Manage members" link
// (nav_workspace_members), the Application card's subtitle (nav_approval_inbox), and -- since
// Fase 6a declared it -- nav_workspace_groups, which the launcher now offers and membersHiddenFor
// must therefore filter for the same reason.
func workspaceHomeLinksTo(t *testing.T, workspaceRole, route string) bool {
	t.Helper()
	all := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_workspace_members", Label: "Workspace Members", Route: "/workspace-members"},
		{ID: "nav_workspace_groups", Label: "Groups", Route: "/workspace-groups"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
	}
	t.Cleanup(func() { ConfigureWorkspace(domain.Workspace{}) })
	ConfigureWorkspace(domain.Workspace{Navigation: all})

	var buf bytes.Buffer
	c := WorkspaceHomePage("Acme", workspaceRole, Viewer{Initials: "AN"}, "", []ApplicationCard{
		{Name: "Task Tracker", Initials: "TT", Role: "member", HomeRoute: "/approval-inbox"},
	})
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return strings.Contains(buf.String(), `href="`+route+`"`)
}

// TestWorkspaceHomePage_membersLinkVisibility is the regression test for a code-review finding
// (2026-09-19): requireWorkspaceAdmin (internal/web/middleware.go) fails *open* for an identity
// with no real membership row at all (the shared admin credential predating per-user accounts),
// letting it reach /workspace-members regardless of role -- showWorkspaceHome degrades that same
// case to workspaceRole == "" (data.Membership{}). Gating the Members link on
// workspaceRole == "admin" would hide a link that identity can still open directly; gating on
// != "member" (what workspacehome.templ actually does) shows it for both a real admin and a
// missing membership row, hiding it only for a real member -- the same three-way split
// requireWorkspaceAdmin already makes.
//
// Since Fase 2 it guards two places at once, because it asserts on the whole rendered page rather
// than on one element: the page's own "Manage members" link, and appShell's launcher panel, which
// lists declared navigation and would otherwise offer every viewer nav_workspace_members. It
// caught exactly that regression the first time the launcher shipped -- see membersHiddenFor
// (workspacehome.templ), whose result feeds both.
func TestWorkspaceHomePage_membersLinkVisibility(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "admin", want: true},
		{role: "", want: true}, // no real membership row -- requireWorkspaceAdmin's fail-open case
		{role: "member", want: false},
	}
	// Both routes sit behind the same requireWorkspaceAdmin group in router.go, so the visibility
	// question is one question and the table answers it once for both. /workspace-groups is here
	// from Fase 6a: Fase 4 shipped it gated but undeclared, so it reached no launcher and nothing
	// could leak it; declaring it put it in AllNavigation, which appLauncher reads before any
	// filtering. Without membersHiddenFor naming it too, a plain member would be offered a link
	// that only ever 403s -- the exact regression this test caught when the launcher first shipped.
	for _, route := range []string{"/workspace-members", "/workspace-groups"} {
		for _, tt := range tests {
			if got := workspaceHomeLinksTo(t, tt.role, route); got != tt.want {
				t.Errorf("workspaceRole = %q: link to %s present = %v, want %v", tt.role, route, got, tt.want)
			}
		}
	}
}
