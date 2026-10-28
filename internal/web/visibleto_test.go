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
		{ID: "nav_reports", Route: "/pages/nav_reports", VisibleTo: []string{"manager"}},
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
		items := navItemsOfRequest(app, "GET", path, "")
		if (want == "") != (len(items) == 0) || (want != "" && items[0].ID != want) {
			t.Errorf("navItemsOfRequest(%q) = %v; want %q", path, items, want)
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

// A counting badge (`badge: ds_x`) must name a Dataset of the Application's own Machines that lists records; one that names nothing is a load error, not a badge
// that never shows.
func TestACountingBadgeMustNameADatasetOfTheApplication(t *testing.T) {
	s := newInstallTestSetup(t, "bbadge")
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	live, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(live), "    route: /machines/mch_document\n", "    route: /machines/mch_document\n    badge: ds_nowhere\n", 1)
	if text == string(live) {
		t.Fatal("the fixture's navigation item moved")
	}
	writeTestFile(t, appFile, text)
	if err := s.deps.ReloadMetadata(s.slug); err == nil || !strings.Contains(err.Error(), "is not a Dataset of the application's Machines") {
		t.Errorf("a badge naming no Dataset loaded: %v", err)
	}
}

// `visible_to:` judges **screens**: a Machine's own pages (GET /machines/<id>/...) are the item's when the item opens that Machine, a tab of one path is its own
// address, two items at one address are offered if either is, and a child is offered only where its parent is. Writes and the JSON twins are not screens.
func TestVisibleToJudgesMachineScreensTabsAndChildren(t *testing.T) {
	app := domain.Application{ID: "app_x", Roles: []string{"clerk", "manager"}, AllNavigation: []domain.NavigationItem{
		{ID: "nav_secret", Route: "/machines/mch_secret", VisibleTo: []string{"manager"}},
		{ID: "nav_mine", Route: "/approval-inbox?tab=mine"},
		{ID: "nav_assigned", Route: "/approval-inbox?tab=assigned", VisibleTo: []string{"manager"}},
		{ID: "nav_group", Route: "/pages/nav_group", VisibleTo: []string{"manager"}},
		{ID: "nav_child", Route: "/pages/nav_child", Parent: "nav_group"},
	}}
	cfg := config.Config{}
	clerk := domain.Actor{ID: "u", WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_x": {"clerk"}}}
	manager := domain.Actor{ID: "u", WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_x": {"manager"}}}
	for name, tc := range map[string]struct {
		method, path, query string
		refusedForClerk     bool
	}{
		"the Machine's list":            {"GET", "/machines/mch_secret", "", true},
		"a record of it":                {"GET", "/machines/mch_secret/records/r1", "", true},
		"its edit form":                 {"GET", "/machines/mch_secret/records/r1/edit", "", true},
		"a write is not a screen":       {"POST", "/machines/mch_secret/records", "", false},
		"the JSON twin is not a screen": {"GET", "/api/machines/mch_secret/records", "", false},
		"the open tab":                  {"GET", "/approval-inbox", "tab=mine", false},
		"the restricted tab":            {"GET", "/approval-inbox", "tab=assigned", true},
		"a tab no item names":           {"GET", "/approval-inbox", "tab=other", false},
		"a child of a hidden parent":    {"GET", "/pages/nav_child", "", true},
		"an address no item names":      {"GET", "/pages/nav_nothing", "", false},
	} {
		if got := navAddressRefused(app, tc.method, tc.path, tc.query, clerk, cfg); got != tc.refusedForClerk {
			t.Errorf("%s: refused for a clerk = %v, want %v", name, got, tc.refusedForClerk)
		}
		if navAddressRefused(app, tc.method, tc.path, tc.query, manager, cfg) {
			t.Errorf("%s: refused for a manager", name)
		}
	}
	// Two items at one address: offered if either is.
	twice := domain.Application{ID: "app_x", AllNavigation: []domain.NavigationItem{
		{ID: "nav_a", Route: "/p", VisibleTo: []string{"manager"}}, {ID: "nav_b", Route: "/p"},
	}}
	if navAddressRefused(twice, "GET", "/p", "", clerk, cfg) {
		t.Error("an address offered by one open item was refused")
	}
}

// A counting badge cannot name a Dataset that needs a parameter no menu can supply (it would answer 500 on every page that draws the menu).
func TestACountingBadgeCannotNeedAParameter(t *testing.T) {
	s := newInstallTestSetup(t, "bbadgeparam")
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"  - id: fld_stage\n    name: Stage\n    type: status\n    options: [Open, Closed]\ndatasets:\n  - id: ds_by_stage\n    select: records\n    where: { field: fld_stage, op: equals, value: $parameters.stage }\n    limit: 10\n")
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	appLive, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, strings.Replace(string(appLive), "    route: /machines/mch_document\n", "    route: /machines/mch_document\n    badge: ds_by_stage\n", 1))
	if err := s.deps.ReloadMetadata(s.slug); err == nil || !strings.Contains(err.Error(), "needs $parameters.stage") {
		t.Errorf("a badge over a Dataset that needs a parameter loaded: %v", err)
	}
}
