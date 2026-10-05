package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// allInstalledWorkspaces loads every manifest under metadata/workspaces/, keyed by slug -- the
// population Deps.Workspaces holds in production.
func allInstalledWorkspaces(t *testing.T) map[string]domain.Workspace {
	t.Helper()
	installed, err := metadata.LoadWorkspaces(filepath.Join("..", "..", "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("LoadWorkspaces: %v", err)
	}
	out := make(map[string]domain.Workspace, len(installed))
	for slug, ws := range installed {
		out[slug] = ws.Workspace
	}
	return out
}

func realLibraryApplications(t *testing.T) []domain.Application {
	t.Helper()
	templates, err := installer.Templates(realLibrary(t))
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	var apps []domain.Application
	for _, tm := range templates {
		apps = append(apps, *tm.Application)
	}
	return apps
}

// TestApplicationOwnedRoutesComeFromNavigation holds the derivation, not a list: a route is owned
// because an Application declares it, the query string is not part of it, and a Workspace-level
// destination is never taken away.
func TestApplicationOwnedRoutesComeFromNavigation(t *testing.T) {
	owned := applicationOwnedRoutes(allInstalledWorkspaces(t), realLibraryApplications(t))

	for _, want := range []string{"/calendar", "/dashboard", "/approval-inbox", "/document-approval/settings"} {
		if !owned[want] {
			t.Errorf("%s is declared by an Application's navigation but is not owned", want)
		}
	}
	for path := range owned {
		if strings.Contains(path, "?") {
			t.Errorf("owned route %q carries a query string; /approval-inbox?tab=mine is the path /approval-inbox", path)
		}
	}
	for _, s := range domain.RuntimeScreens {
		if owned[s.Route] {
			t.Errorf("runtime screen %s (%s) is owned by an Application; a Workspace-level destination must never be 404", s.ID, s.Route)
		}
	}
	for _, never := range []string{"/home", "/login", "/"} {
		if owned[never] {
			t.Errorf("%s is runtime-level and must not be owned", never)
		}
	}
}

// TestRequireInstalledApplication is the middleware's own contract, without a database: an owned
// route in a request that resolved no Application is a 404; the same route once an Application
// resolved passes; and a route nobody owns passes whatever the Application.
func TestRequireInstalledApplication(t *testing.T) {
	owned := map[string]bool{"/calendar": true}
	reached := false
	h := requireInstalledApplication(owned)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
	}))

	cases := []struct {
		name    string
		path    string
		inApp   bool
		want404 bool
	}{
		{"owned, Application not installed", "/calendar", false, true},
		{"owned, Application installed", "/calendar", true, false},
		{"not owned by anyone", "/home", false, false},
	}
	for _, c := range cases {
		reached = false
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		if c.inApp {
			req = req.WithContext(rendering.WithCurrentApplication(req.Context(), domain.Application{ID: "app_x"}))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Code == http.StatusNotFound; got != c.want404 {
			t.Errorf("%s: status %d, want 404=%v", c.name, rec.Code, c.want404)
		}
		if reached == c.want404 {
			t.Errorf("%s: handler reached=%v, want %v", c.name, reached, !c.want404)
		}
	}
}

// installedRoutesRatchet grandfathers (manifest|route) pairs that still panic or answer 5xx in an
// installed Workspace for a reason this gate does not own. **It may only shrink**; an entry that now
// behaves fails too.
var installedRoutesRatchet = map[string]string{}

// TestNoRouteBreaksInAWorkspaceThatDidNotInstallItsApplication sweeps every authenticated GET route
// that takes no path parameter across **every** installed Workspace, which the other sweeps cannot
// do: they build one Workspace (default, which installs everything), so a route that only breaks
// where an Application is *absent* is outside their population. Measured 2026-10-05, that was eight
// routes in each of four Workspaces -- panics in routeByID, and 500s -- found by a throwaway probe,
// not by any gate.
//
// A route an Application owns, in a Workspace that did not install it, must be 404. Anything else
// must neither panic nor answer 5xx (a 503 is the assistant being unconfigured, an environment fact).
func TestNoRouteBreaksInAWorkspaceThatDidNotInstallItsApplication(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "metadata", "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	owned := applicationOwnedRoutes(allInstalledWorkspaces(t), realLibraryApplications(t))
	routes := authenticatedGetRoutes(t)

	var checked, notFound int
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		manifest := strings.TrimSuffix(e.Name(), ".yaml")
		h, cookie, _, _, _, installed, _ := routerSetupFor(t, "installedroutes-"+manifest, manifest)

		hasRoute := map[string]bool{}
		for _, app := range installed.Applications {
			for _, item := range app.AllNavigation {
				path, _, _ := strings.Cut(item.Route, "?")
				hasRoute[path] = true
			}
		}

		for _, route := range routes {
			key := manifest + "|" + route
			status, panicked := serveRecovering(h, route, cookie)
			checked++
			wantNotFound := owned[route] && !hasRoute[route]
			var problem string
			switch {
			case panicked != "":
				problem = "panicked: " + panicked
			case status >= 500 && status != http.StatusServiceUnavailable:
				problem = fmt.Sprintf("answered %d", status)
			case wantNotFound && status != http.StatusNotFound:
				problem = fmt.Sprintf("is owned by an Application this Workspace did not install, answered %d (want 404)", status)
			}
			if wantNotFound && status == http.StatusNotFound {
				notFound++
			}
			_, grandfathered := installedRoutesRatchet[key]
			seen[key] = problem != ""
			if problem != "" && !grandfathered {
				t.Errorf("%s: %s\n  installedRoutesRatchet is not the way to pass", key, problem)
			}
			if problem == "" && grandfathered {
				t.Errorf("%s is in installedRoutesRatchet but now behaves -- remove the entry", key)
			}
		}
	}
	var stale []string
	for key := range installedRoutesRatchet {
		if _, ok := seen[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("installedRoutesRatchet names %s, which this sweep never exercised", key)
	}
	t.Logf("checked %d (Workspace, route) pairs; %d answered 404 for an Application not installed", checked, notFound)
	if notFound == 0 {
		t.Fatal("no route answered 404 anywhere: the population this sweep exists for was never reached")
	}
}

func serveRecovering(h http.Handler, route, cookie string) (status int, panicked string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprint(r)
		}
	}()
	req := httptest.NewRequest(http.MethodGet, route, nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, ""
}
