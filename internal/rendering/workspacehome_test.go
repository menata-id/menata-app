package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// workspaceHomeLinksTo renders WorkspaceHomePage with the given workspaceRole -- carried on
// Viewer since 2026-09-24, where it belongs: this test is what caught the page and appShell's new
// Workspace menu reading two different sources for the same fact, on the first build after that
// menu existed. Every other
// parameter fixed to a minimal valid fixture, reporting whether the rendered page links to route.
// WorkspaceHomePage resolves every route and label it renders through routeByID/labelByID
// (machine.templ), so a nav fixture must be configured before rendering, same pattern
// TestRouteByID_survivesHiddenNavGroup (navigation_test.go) already uses. Three items are
// declared here: the page's own title and breadcrumb (nav_home), the Application card's subtitle
// (nav_approval_inbox), and nav_workspace_groups (Fase 6a), which the launcher offers and
// membersHiddenFor must therefore filter for the same reason. nav_workspace_settings, this page's
// own closing-row link since 2026-09-25 (Tahap 5), needs no entry here: it is one of
// domain.RuntimeScreens, appended by declaredNavigation regardless of what a fixture declares.
func workspaceHomeLinksTo(t *testing.T, workspaceRole, route string) bool {
	t.Helper()
	all := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_workspace_groups", Label: "Groups", Route: "/workspace-groups"},
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: all}, "Test Workspace", false)

	var buf bytes.Buffer
	c := WorkspaceHomePage("Acme", Viewer{Initials: "AN", WorkspaceRole: workspaceRole}, "", []ApplicationCard{
		{Name: "Task Tracker", Initials: "TT", Role: "member", HomeRoute: "/approval-inbox"},
	})
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return strings.Contains(buf.String(), `href="`+route+`"`)
}

// TestWorkspaceHomePage_membersLinkVisibility is the regression test for a code-review finding
// (2026-09-19): requireWorkspaceAdmin (internal/web/middleware.go) fails *open* for an identity
// with no real membership row at all (the shared admin credential predating per-user accounts),
// letting it reach the Workspace Settings hub (nee straight to /workspace-members) regardless of
// role -- showWorkspaceHome degrades that same case to workspaceRole == "" (data.Membership{}).
// Gating the closing row's link on workspaceRole == "admin" would hide a link that identity can
// still open directly; gating on != "member" (what workspacehome.templ actually does) shows it for
// both a real admin and a missing membership row, hiding it only for a real member -- the same
// three-way split requireWorkspaceAdmin already makes.
//
// Checks the whole rendered page rather than one element, though today there is only one to check:
// this page's own closing row, "Manage members" -> /workspace-members until 2026-09-25, now
// "Workspace settings" -> /workspace-settings (Flow 2 gap study Tahap 5) -- a screenshot caught it
// still pointing at the old route after the hub shipped, since nobody had re-checked this page
// against the mockup's own M03-WorkspaceHome.dc.html, whose closing row already read "Settings ->"
// before this fix. This test used to also guard an offer in the launcher panel
// (`membersHiddenFor`), deleted 2026-09-21 with `appShell`'s own `hiddenNavIDs` parameter -- the
// launcher lists Applications now, not destinations -- so there has been only the one place to
// leak since then, not two.
func TestWorkspaceHomePage_membersLinkVisibility(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "admin", want: true},
		{role: "", want: true}, // no real membership row -- requireWorkspaceAdmin's fail-open case
		{role: "member", want: false},
	}
	// /workspace-groups was checked here alongside Members from Fase 6a until 2026-09-21, when
	// workspace navigation stopped being metadata and the launcher stopped rendering it (it lists
	// Applications now). Groups appears in no menu on this page for any role, so asserting it is
	// hidden from a member would assert something no longer capable of leaking -- a test that
	// passes for the wrong reason. It is still reachable, from the Workspace Settings hub, and
	// still behind requireWorkspaceAdmin; what changed is only that no menu offers it.
	//
	// The closing row stays checked because this page still links it directly ("Workspace
	// settings →", gated by workspaceRole != "member"), which is the half of the 2026-09-19 finding
	// that is still live.
	for _, tt := range tests {
		if got := workspaceHomeLinksTo(t, tt.role, "/workspace-settings"); got != tt.want {
			t.Errorf("workspaceRole = %q: link to /workspace-settings present = %v, want %v", tt.role, got, tt.want)
		}
	}
}
