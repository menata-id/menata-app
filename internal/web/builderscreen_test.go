package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// The builder's screen lists an Application's parts as a tree -- Machines with their Fields, its screens, its menu -- marks the part the request
// selects and reads it in the inspector, writes nothing, and answers 404 for an Application the Workspace does not have.
func TestBuilderScreenListsTheApplicationsPartsAndReadsTheSelectedOne(t *testing.T) {
	s := newInstallTestSetup(t, "bscreen")
	page := s.getAs(t, "/builder/app_document_tracking")
	if page.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	for _, want := range []string{"Document Tracking", ">Data &amp; form</summary>", ">Tracked Document</summary>", ">Title<", ">Views</summary>", ">Documents<", ">Menu</summary>", "Choose a part of the application to read it."} {
		if !strings.Contains(body, want) {
			t.Errorf("the builder does not show %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `aria-current="true"`) {
		t.Error("nothing was selected and a part is marked")
	}

	node := "app_document_tracking/machines/mch_document/fields/fld_title"
	sel := s.getAs(t, "/builder/app_document_tracking?node="+strings.ReplaceAll(node, "/", "%2F")).Body.String()
	row := regexp.MustCompile(`(?s)<li[^>]*aria-current="true"[^>]*>(.*?)</li>`).FindAllStringSubmatch(sel, -1)
	if len(row) != 1 || !strings.Contains(row[0][1], ">Title<") {
		t.Fatalf("want exactly the Title field marked, got %v", row)
	}
	if !strings.Contains(sel, "machines/mch_document/fields/fld_title") || strings.Contains(sel, "Choose a part of the application to read it.") {
		t.Errorf("the inspector does not read the selected part:\n%s", sel)
	}

	if rec := s.getAs(t, "/builder/app_nobody"); rec.Code != http.StatusNotFound {
		t.Errorf("an Application the Workspace does not have: %d, want 404", rec.Code)
	}
}

// The builder frames the Application's own screen in builder mode, and that screen names its parts: the menu entry and the page it is on carry
// `data-node-path`, "<Application>/<metadata path>", the very id the tree gives the part. Without `?builder=1`, or for a member who adds it, the
// page is the ordinary one and carries no such attribute.
func TestBuilderPreviewNamesItsPartsOnlyForAnAdminWhoAsks(t *testing.T) {
	s := newInstallTestSetup(t, "bpreview")

	shell := s.getAs(t, "/builder/app_document_tracking").Body.String()
	if m := regexp.MustCompile(`<iframe[^>]*src="([^"]*)"`).FindStringSubmatch(shell); m == nil || !strings.Contains(m[1], "builder=1") || !strings.HasPrefix(m[1], "/") {
		t.Fatalf("the builder does not frame the application's screen in builder mode: %v", m)
	}
	if !strings.Contains(shell, `data-frame-query="node"`) {
		t.Error("the frame does not say which parameter a chosen part is carried in")
	}

	page := s.getAs(t, "/machines/mch_document?builder=1").Body.String()
	want := `data-node-path="app_document_tracking/navigation/nav_document_tracking"`
	if strings.Count(page, want) != 2 {
		t.Errorf("want the menu entry and the page itself to name navigation/nav_document_tracking once each, got %d:\n%s", strings.Count(page, want), page)
	}
	if !strings.Contains(page, "menataNode") || !strings.Contains(page, "htmx:beforeRequest") {
		t.Error("the preview does not report a click to its frame")
	}
	plain := s.getAs(t, "/machines/mch_document").Body.String()
	if strings.Contains(plain, "data-node-path") || strings.Contains(plain, "menataNode") {
		t.Error("an ordinary request carries builder markup")
	}

	memberID, err := s.store.CreateRecord(data.WithWorkspaceScope(context.Background(), s.wsID), "mch_user", map[string]any{"fld_email": "bpreview-member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.AddMember(context.Background(), s.wsID, memberID.ID, "bpreview-member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/machines/mch_document?builder=1", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.deps.Cfg, memberID.ID, 0)})
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the member could not open the ordinary page (%d), so the check below measures nothing", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "data-node-path") {
		t.Errorf("a member got builder markup (status %d)", rec.Code)
	}
}

// The sweep behind Fase 3's "every visible part maps to exactly one metadata path": every installed Application's home screen, drawn in builder
// mode, names only parts the builder's own tree lists for that Application -- the same ids, so a click in the preview always finds its row.
func TestBuilderPreviewNamesOnlyPartsTheTreeLists(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "bsweep")
	machines := map[string]map[string]bool{}
	for _, m := range fx.ws.Machines {
		machines[m.ID] = map[string]bool{}
		for _, f := range m.Fields {
			machines[m.ID][f.ID] = true
		}
	}
	listed := func(appID, path string) bool {
		for _, app := range fx.ws.Applications {
			if app.ID != appID {
				continue
			}
			if path == "navigation" {
				return true
			}
			for _, item := range app.AllNavigation {
				if path == "navigation/"+item.ID {
					return true
				}
			}
			for _, id := range app.Machines {
				if path == "machines/"+id {
					return true
				}
				if field, ok := strings.CutPrefix(path, "machines/"+id+"/fields/"); ok && machines[id][field] {
					return true
				}
			}
		}
		return false
	}
	pattern := regexp.MustCompile(`data-node-path="([^"]*)"`)
	swept, named, fields := 0, 0, 0
	for _, app := range fx.ws.Applications {
		if app.Inactive {
			continue
		}
		for _, item := range app.AllNavigation {
			if item.Route == "" || strings.ContainsAny(item.Route, "{}") {
				continue
			}
			u, err := url.Parse(item.Route)
			if err != nil {
				continue
			}
			q := u.Query()
			q.Set("builder", "1")
			u.RawQuery = q.Encode()
			req := httptest.NewRequest(http.MethodGet, u.String(), nil)
			req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				continue // a screen that does not draw for this fixture is the route sweep's business
			}
			swept++
			for _, m := range pattern.FindAllStringSubmatch(rec.Body.String(), -1) {
				named++
				appID, path, ok := strings.Cut(m[1], "/")
				if strings.Contains(path, "/fields/") {
					fields++
				}
				// A screen of a Machine another Application shares is named by the Application the request resolved to; every name must be one of its parts.
				if !ok || !listed(appID, path) {
					t.Errorf("%s names %q, which the builder's tree does not list for that Application", item.Route, m[1])
				}
			}
		}
	}
	if swept < 10 || named < swept {
		t.Fatalf("swept %d screens and %d named parts -- the sweep measures too little", swept, named)
	}
	if fields == 0 {
		t.Fatal("no control of any form named its Field -- the form naming stopped, and every name that is left is still listed")
	}
	t.Logf("swept %d screens, %d named parts, %d of them Fields", swept, named, fields)
}

// The preview is the one response that may be framed, and only by this origin; every other response still refuses a frame. (Without this the
// browser shows "refused to connect" in the builder and no click ever reaches it.)
func TestBuilderPreviewMayBeFramedByThisOriginOnly(t *testing.T) {
	s := newInstallTestSetup(t, "bframe")
	for path, want := range map[string][2]string{
		"/machines/mch_document?builder=1": {"SAMEORIGIN", "frame-ancestors 'self'"},
		"/machines/mch_document":           {"DENY", "frame-ancestors 'none'"},
		"/builder/app_document_tracking":   {"DENY", "frame-ancestors 'none'"},
	} {
		h := s.getAs(t, path).Header()
		if h.Get("X-Frame-Options") != want[0] || !strings.Contains(h.Get("Content-Security-Policy"), want[1]) {
			t.Errorf("%s: X-Frame-Options %q, CSP %q; want %q and %q", path, h.Get("X-Frame-Options"), h.Get("Content-Security-Policy"), want[0], want[1])
		}
	}
}

// The address the builder frames keeps whatever query the home route already has.
func TestBuilderPreviewRouteKeepsTheHomeRoutesOwnQuery(t *testing.T) {
	for route, want := range map[string]string{"/machines/x": "/machines/x?builder=1", "/a?tab=mine": "/a?builder=1&tab=mine", "": ""} {
		if got := builderPreviewRoute(domain.Application{HomeRoute: route}, ""); got != want {
			t.Errorf("%q -> %q, want %q", route, got, want)
		}
	}
}

// The inspector's Code tab shows the metadata file that declares the selected part -- the Machine's own file for a Field, the Application's file for a
// screen -- and only on that tab, for only the selected part.
func TestBuilderInspectorCodeTabShowsTheFileThatDeclaresThePart(t *testing.T) {
	s := newInstallTestSetup(t, "bcode")
	at := func(node, tab string) string {
		q := "node=" + strings.ReplaceAll(node, "/", "%2F")
		if tab != "" {
			q += "&tab=" + tab
		}
		return s.getAs(t, "/builder/app_document_tracking?"+q).Body.String()
	}
	field := "app_document_tracking/machines/mch_document/fields/fld_title"
	if body := at(field, "code"); !strings.Contains(body, "id: mch_document") || !strings.Contains(body, "Tracked Document") {
		t.Errorf("the Code tab does not show the Machine's file:\n%s", body)
	}
	if body := at(field, ""); strings.Contains(body, "id: mch_document") {
		t.Errorf("the Details tab shows the file:\n%s", body)
	}
	if body := at("app_document_tracking/navigation/nav_document_tracking", "code"); !strings.Contains(body, "id: app_document_tracking") || strings.Contains(body, "id: mch_document\n") {
		t.Errorf("a screen's Code tab shows the Application's file, not a Machine's:\n%s", body)
	}
	if body := at(field, "code"); strings.Contains(body, "id: app_document_tracking") {
		t.Errorf("only the selected part's file is read, and the Application's file is not the Machine's")
	}
}

// The way in is where the mockup puts it: each row of Workspace settings' Applications section links to that Application's builder ("Buka di builder",
// WorkspaceApps), built from the Application's id by the source's record route and never typed.
func TestWorkspaceSettingsLinksEachApplicationToItsBuilder(t *testing.T) {
	s := newInstallTestSetup(t, "bentry")
	body := s.getAs(t, "/workspace-settings").Body.String()
	if !strings.Contains(body, `href="/builder/app_document_tracking"`) || !strings.Contains(body, "Open in builder") {
		t.Errorf("the Applications section does not link the Application to its builder:\n%s", body)
	}
}

// Preview as a role: the builder offers the admin and each role the Application declares, keeps the part and tab already chosen when the viewer
// changes, frames the screen with that choice, and the framed screen is drawn for someone who holds exactly that role -- a role the Application
// does not declare holds nothing there, so the screen refuses, which is the honest preview of "this person cannot open it".
func TestBuilderPreviewAsARole(t *testing.T) {
	h, cookie, fx := newPerRecordSweepSetup(t, "bas")
	get := func(path string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	tested := false
	for _, app := range fx.ws.Applications {
		if app.Inactive || app.HomeRoute == "" || len(app.Roles) == 0 {
			continue
		}
		role := app.Roles[0]
		code, page := get("/builder/" + app.ID + "?node=x&tab=code&as=" + url.QueryEscape(role))
		if code != http.StatusOK {
			t.Fatalf("builder for %s as %s: %d", app.ID, role, code)
		}
		if !strings.Contains(page, "As the builder") || !strings.Contains(page, ">As "+role+"<") {
			t.Errorf("%s: the builder does not offer the admin and the role %q:\n%s", app.ID, role, page)
		}
		if m := regexp.MustCompile(`<iframe[^>]*src="([^"]*)"`).FindStringSubmatch(page); m == nil || !strings.Contains(m[1], "as="+url.QueryEscape(role)) {
			t.Errorf("%s: the frame does not carry the previewed role: %v", app.ID, m)
		}
		if !strings.Contains(page, "tab=code") || !strings.Contains(page, "node=x") {
			t.Errorf("%s: changing the viewer loses the part or the tab", app.ID)
		}
		if code, _ := get(app.HomeRoute + "?builder=1&as=" + url.QueryEscape(role)); code != http.StatusOK {
			t.Errorf("%s as %s: %d, want 200", app.HomeRoute, role, code)
		}
		// The shell of the preview is a member's: the admin's own Administration rows are not drawn when previewed as a role.
		_, asAdmin := get(app.HomeRoute + "?builder=1")
		_, asRole := get(app.HomeRoute + "?builder=1&as=" + url.QueryEscape(role))
		if !strings.Contains(asAdmin, "Workspace settings") || strings.Contains(asRole, "Workspace settings") {
			t.Errorf("%s: the Administration rows must show for the builder and not for a previewed role (admin %v, role %v)", app.HomeRoute, strings.Contains(asAdmin, "Workspace settings"), strings.Contains(asRole, "Workspace settings"))
		}
		if code, _ := get(app.HomeRoute + "?builder=1&as=nobody_holds_this"); code != http.StatusForbidden {
			t.Errorf("%s as a role the Application does not declare: %d, want 403 (fails closed)", app.HomeRoute, code)
		}
		if code, _ := get(app.HomeRoute + "?as=nobody_holds_this"); code == http.StatusForbidden {
			t.Errorf("%s: `as` without builder mode changed who the admin is", app.HomeRoute)
		}
		tested = true
	}
	if !tested {
		t.Fatal("no installed Application declares roles and a home screen -- the preview-as-a-role check measures nothing")
	}
}
