package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// Workspace Home names itself in its own heading, so the shell's breadcrumb is the Workspace's name alone (mockup board 03): no
// "/ Applications" after it, whichever of the screen's two names the page passes -- its menu word or its heading. Another
// Workspace-level screen still says which one it is.
func TestShellBreadcrumb_HomeIsTheWorkspaceNameAlone(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Title: "Applications", Heading: "Applications", Route: "/home"},
		{ID: "nav_workspace_groups", Label: "Groups", Route: "/workspace-groups"},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: items}, "Test Workspace", false)
	render := func(title string) string {
		var buf bytes.Buffer
		if err := appShell(title, "Dokter Kecil", Viewer{Initials: "AN", WorkspaceRole: "admin"}, "/switch-workspace").Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	crumb := func(html string) string {
		i := strings.Index(html, `aria-label="Breadcrumb"`)
		j := strings.Index(html[i:], "</nav>")
		return html[i : i+j]
	}
	for _, title := range []string{"Home", "Applications"} {
		if c := crumb(render(title)); strings.Contains(c, ">/<") || strings.Contains(c, title+"</span>") {
			t.Errorf("Home passed as %q still draws a trailing page name in the breadcrumb:\n%s", title, c)
		}
	}
	if c := crumb(render("Groups")); !strings.Contains(c, ">/<") || !strings.Contains(c, "Groups") {
		t.Errorf("another Workspace-level screen lost its breadcrumb:\n%s", c)
	}
}

// The Workspace's name in the shell is a link to its Home and nothing more (owner decision, 2026-10-22): no menu button, no popover.
// "Workspace settings" and "New application" are on Home itself, and "Switch workspace" is the launcher's "All Workspaces".
func TestShellBreadcrumb_theWorkspaceNameIsALinkNotAMenu(t *testing.T) {
	items := []domain.NavigationItem{{ID: "nav_home", Label: "Home", Title: "Applications", Heading: "Applications", Route: "/home"}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: items}, "Test Workspace", false)
	var buf bytes.Buffer
	if err := appShell("Applications", "Dokter Kecil", Viewer{Initials: "AN", WorkspaceRole: "admin"}, "/switch-workspace").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	i := strings.Index(html, `aria-label="Breadcrumb"`)
	crumb := html[i : i+strings.Index(html[i:], "</nav>")]
	if !strings.Contains(crumb, `<a href="/home"`) || !strings.Contains(crumb, ">Dokter Kecil</a>") {
		t.Errorf("the Workspace's name is not a link to Home:\n%s", crumb)
	}
	for _, gone := range []string{"popovertarget", "menata-workspace-menu", "<button"} {
		if strings.Contains(crumb, gone) {
			t.Errorf("the breadcrumb still draws a menu (%q):\n%s", gone, crumb)
		}
	}
	if !strings.Contains(html, `href="/switch-workspace"`) {
		t.Error("with the menu gone the launcher must still offer the Workspace picker")
	}
}
