package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/config"
	"menata.app/internal/domain"
)

// `visible_to:` is one rule for the menu and the route: an item naming no roles is everyone's, otherwise the viewer holds one of them in the Application, or is
// a Workspace admin or the configured admin; and the item a request is for is found by its address, record pages included.
func TestNavItemVisibleAndTheItemOfARequest(t *testing.T) {
	app := domain.Application{ID: "app_x", Roles: []string{"clerk", "manager"}, AllNavigation: []domain.NavigationItem{
		{ID: "nav_orders", Route: "/pages/nav_orders"},
		{ID: "nav_reports", Route: "/pages/nav_reports?tab=all", VisibleTo: []string{"manager"}},
	}}
	cfg := config.Config{AdminUserID: "break-glass"}
	member := func(roles ...string) domain.Actor {
		return domain.Actor{ID: "u", WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_x": roles}}
	}
	reports := app.AllNavigation[1]
	for name, tc := range map[string]struct {
		actor domain.Actor
		want  bool
	}{
		"a manager":            {member("manager"), true},
		"a clerk":              {member("clerk"), false},
		"a manager elsewhere":  {domain.Actor{ID: "u", WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_y": {"manager"}}}, false},
		"nobody":               {domain.Actor{}, false},
		"a Workspace admin":    {domain.Actor{WorkspaceRole: domain.WorkspaceRoleAdmin}, true},
		"the configured admin": {domain.Actor{ID: "break-glass"}, true},
	} {
		if got := navItemVisible(app, reports, tc.actor, cfg); got != tc.want {
			t.Errorf("%s: visible = %v, want %v", name, got, tc.want)
		}
	}
	if !navItemVisible(app, app.AllNavigation[0], domain.Actor{}, cfg) {
		t.Error("an item that names no roles is hidden")
	}
	if h := hiddenNavItems(app, member("clerk"), cfg); !h["nav_reports"] || h["nav_orders"] || len(h) != 1 {
		t.Errorf("hidden for a clerk = %v", h)
	}
	if h := hiddenNavItems(app, member("manager"), cfg); len(h) != 0 {
		t.Errorf("hidden for a manager = %v", h)
	}
	for path, want := range map[string]string{
		"/pages/nav_reports":       "nav_reports",
		"/pages/nav_reports/rec_1": "nav_reports",
		"/pages/nav_orders":        "nav_orders",
		"/pages/nav_missing":       "",
		"/machines/mch_x":          "",
	} {
		item, ok := navItemOfRequest(app, path)
		if (want == "") == ok || item.ID != want {
			t.Errorf("navItemOfRequest(%q) = %q, %v; want %q", path, item.ID, ok, want)
		}
	}
}

// A role `visible_to:` names must be one the Application declares: otherwise nobody could ever be offered the item, which reads as a grant and denies everyone.
func TestVisibleToNamesOnlyRolesTheApplicationDeclares(t *testing.T) {
	s := newInstallTestSetup(t, "bvisible")
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	live, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace("roles: [author]\n"+string(live), "    route: /machines/mch_document\n", "    route: /machines/mch_document\n    visible_to: [janitor]\n", 1)
	if text == "roles: [author]\n"+string(live) {
		t.Fatal("the fixture's navigation item moved")
	}
	writeTestFile(t, appFile, text)
	if err := s.deps.ReloadMetadata(s.slug); err == nil || !strings.Contains(err.Error(), "visible_to names role") {
		t.Errorf("a role the Application does not declare loaded: %v", err)
	}
}
