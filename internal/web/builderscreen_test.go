package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// The inspector's Edit tab is a form for the selected Field -- its name and whether it must be filled in, starting as they are now -- and only for a
// Field. Saving makes the same builder operations a conversation would: the Field is renamed in its Machine's own file and the tree shows the new name.
func TestBuilderEditTabRenamesAndRequiresAField(t *testing.T) {
	s := newInstallTestSetup(t, "bedit")
	field := "app_document_tracking/machines/mch_document/fields/fld_title"
	edit := func(node string) string {
		return s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(node)).Body.String()
	}
	body := edit(field)
	for _, want := range []string{`/runtime-actions/edit_field?record=`, `name="name"`, `name="required"`, "Save field"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Edit tab of a Field is missing %q:\n%s", want, body)
		}
	}
	if body := edit("app_document_tracking/machines/mch_document"); strings.Contains(body, "edit_field") {
		t.Errorf("a part that is not a Field is offered the Field form")
	}
	if body := edit("app_document_tracking/navigation"); !strings.Contains(body, "can be edited so far") {
		t.Errorf("a part that is not a Field does not say so:\n%s", body)
	}

	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	rec := s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"  Headline  "}, "required": {"on"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("saving a Field: status %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/builder/app_document_tracking") || !strings.Contains(loc, "tab=edit") {
		t.Errorf("saving does not return to the part: %q", loc)
	}
	written, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "Headline") {
		t.Errorf("the Field's new name is not in its Machine's file:\n%s", written)
	}
	if body := s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(field)).Body.String(); !strings.Contains(body, `value="Headline"`) || !strings.Contains(body, "checked") {
		t.Errorf("the form does not start from the saved name and requirement:\n%s", body)
	}
	// An unticked box is "not required": the Field's own block loses its `required`.
	if rec := s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Headline"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("saving without the tick: status %d: %s", rec.Code, rec.Body.String())
	}
	written, _ = os.ReadFile(file)
	block, _, _ := strings.Cut(string(written)[strings.Index(string(written), "name: Headline"):], "- id:")
	if strings.Contains(block, "required: true") {
		t.Errorf("the Field is still required after the tick was removed:\n%s", block)
	}

	for name, post := range map[string]struct {
		record string
		form   url.Values
	}{
		"a blank name":              {field, url.Values{"name": {"   "}}},
		"a part not Field":          {"app_document_tracking/machines/mch_document", url.Values{"name": {"x"}}},
		"a Field that is not there": {"app_document_tracking/machines/mch_document/fields/fld_nope", url.Values{"name": {"x"}}},
	} {
		before, _ := os.ReadFile(file)
		if rec := s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(post.record), post.form); rec.Code < 400 {
			t.Errorf("%s was accepted: %d", name, rec.Code)
		}
		if after, _ := os.ReadFile(file); string(after) != string(before) {
			t.Errorf("%s wrote the file", name)
		}
	}
}

// A Field that offers choices shows them one per line in the Edit tab, and saving the lines adds the new ones. A choice no line names would be removed, which
// the plan refuses (records may hold it), and nothing is written; a Field without choices refuses lines; a box emptied of every line changes nothing.
func TestBuilderEditTabEditsTheChoicesOfAField(t *testing.T) {
	s := newInstallTestSetup(t, "bopts")
	field := "app_document_tracking/machines/mch_document/fields/fld_kind"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"  - id: fld_kind\n    name: Kind\n    type: status\n    options: [Draft, Under Review, Approved, Rejected]\n")
	s.reloadWorkspace(t)
	body := s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(field)).Body.String()
	for _, want := range []string{`name="options"`, "Under Review", "Rejected"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Edit tab does not offer the choices (%q):\n%s", want, body)
		}
	}
	post := func(node, lines string) int {
		return s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(node), url.Values{"name": {"Kind"}, "options": {lines}}).Code
	}
	all := "Draft\r\nUnder Review\r\nApproved\r\nRejected"
	read := func() string { b, _ := os.ReadFile(file); return string(b) }

	if code := post(field, all+"\r\n\r\nArchived\r\nArchived"); code != http.StatusSeeOther {
		t.Fatalf("adding a choice: status %d", code)
	}
	if strings.Count(read(), "Archived") != 1 {
		t.Errorf("the new choice is not in the file exactly once:\n%s", read())
	}
	before := read()
	if code := post(field, "Draft\r\nApproved"); code < 400 {
		t.Errorf("removing choices was accepted: %d", code)
	}
	if read() != before {
		t.Errorf("a refused removal wrote the file")
	}
	if code := post(field, " \r\n "); code != http.StatusSeeOther || read() != before {
		t.Errorf("an empty box must change nothing (status %d)", code)
	}
	if code := post("app_document_tracking/machines/mch_document/fields/fld_title", "A\r\nB"); code < 400 {
		t.Errorf("choices for a Field that offers none were accepted: %d", code)
	}
}

// A Machine's Edit tab is a form that adds a Field to it: named, typed, with the choices a status needs. The id is made from the name and kept unique,
// and the Machine's own page offers the new Field at once -- the point of the editor is that nothing else has to change.
func TestBuilderEditTabAddsAFieldToAMachine(t *testing.T) {
	s := newInstallTestSetup(t, "badd")
	machine := "app_document_tracking/machines/mch_document"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	body := s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(machine)).Body.String()
	for _, want := range []string{"/runtime-actions/add_field?record=", `name="type"`, `name="options"`, "Add field"} {
		if !strings.Contains(body, want) {
			t.Errorf("a Machine's Edit tab is missing %q", want)
		}
	}
	if strings.Contains(body, "edit_field") {
		t.Errorf("a Machine is offered the form that edits a Field")
	}
	add := func(form url.Values) int {
		return s.postTo(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), form).Code
	}
	if code := add(url.Values{"name": {"Reference no."}, "type": {"text"}, "required": {"on"}}); code != http.StatusSeeOther {
		t.Fatalf("adding a Field: status %d", code)
	}
	if code := add(url.Values{"name": {"Reference no."}, "type": {"number"}}); code != http.StatusSeeOther {
		t.Fatalf("adding a second Field of the same name: status %d", code)
	}
	if code := add(url.Values{"name": {"Stage"}, "type": {"status"}, "options": {"Open\r\nClosed\r\nOpen"}}); code != http.StatusSeeOther {
		t.Fatalf("adding a status: status %d", code)
	}
	written, _ := os.ReadFile(file)
	for _, want := range []string{"id: fld_reference_no", "id: fld_reference_no_2", "id: fld_stage", "Closed"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("the file lacks %q:\n%s", want, written)
		}
	}
	if page := s.getAs(t, "/machines/mch_document").Body.String(); !strings.Contains(page, "Reference no.") {
		t.Errorf("the Machine's page does not offer the new Field")
	}
	if page := s.getAs(t, "/builder/app_document_tracking").Body.String(); !strings.Contains(page, "Stage") {
		t.Errorf("the builder's tree does not list the new Field")
	}
	before := string(written)
	for name, form := range map[string]url.Values{
		"no name":              {"name": {" "}, "type": {"text"}},
		"a type not offered":   {"name": {"X"}, "type": {"relation"}},
		"a status with no one": {"name": {"Y"}, "type": {"status"}},
	} {
		if code := add(form); code < 400 {
			t.Errorf("%s was accepted: %d", name, code)
		}
	}
	if after, _ := os.ReadFile(file); string(after) != before {
		t.Errorf("a refused Field wrote the file")
	}
	if code := s.postTo(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine+"/fields/fld_title"), url.Values{"name": {"Z"}, "type": {"text"}}).Code; code < 400 {
		t.Errorf("a Field was added to a part that is not a Machine: %d", code)
	}
}

// A status choice's colour is written after a bar ("Closed | done"): a new choice carries one into the Machine's status_categories, a Field added as a status
// can colour its choices, the box shows the colours back, and a colour that is not one of the five, or a change to a choice already there, writes nothing.
func TestBuilderEditTabColoursTheChoicesOfAStatus(t *testing.T) {
	s := newInstallTestSetup(t, "bcolour")
	machine := "app_document_tracking/machines/mch_document"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	add := func(form url.Values) int {
		return s.postTo(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), form).Code
	}
	if code := add(url.Values{"name": {"Stage"}, "type": {"status"}, "options": {"Open | start\r\nClosed | done"}}); code != http.StatusSeeOther {
		t.Fatalf("adding a coloured status: %d", code)
	}
	if got := read(); !strings.Contains(got, "status_categories") || !strings.Contains(got, "Closed: done") || !strings.Contains(got, "Open: start") {
		t.Fatalf("the colours are not in the file:\n%s", got)
	}
	field := machine + "/fields/fld_stage"
	if body := s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(field)).Body.String(); !strings.Contains(body, "Closed | done") {
		t.Errorf("the box does not show the colours back:\n%s", body)
	}
	edit := func(lines string) int {
		return s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Stage"}, "options": {lines}}).Code
	}
	if code := edit("Open | start\r\nClosed | done\r\nLate | attention"); code != http.StatusSeeOther {
		t.Fatalf("a new coloured choice: %d", code)
	}
	if !strings.Contains(read(), "Late: attention") {
		t.Errorf("the new choice's colour is not in the file:\n%s", read())
	}
	before := read()
	for name, lines := range map[string]string{
		"an unknown colour":              "Open | start\r\nClosed | done\r\nLate | attention\r\nX | purple",
		"a colour changed":               "Open | stopped\r\nClosed | done\r\nLate | attention",
		"a colour given to a plain one":  "Open\r\nClosed | done\r\nLate | attention",
		"a colour taken from a coloured": "Open | start\r\nClosed\r\nLate | attention",
	} {
		if code := edit(lines); code < 400 {
			t.Errorf("%s was accepted: %d", name, code)
		}
		if read() != before {
			t.Errorf("%s wrote the file", name)
		}
	}
}

// "Who may" is a part of the tree for each Permission of a Machine, and its Edit tab ticks the Application's roles that may do it. Saving writes the same
// operation a conversation would; at least one role stays, a role the Application does not declare is stopped, and a form nobody changed writes nothing.
func TestBuilderEditTabSetsWhoMayDoWhatAPermissionGoverns(t *testing.T) {
	s := newInstallTestSetup(t, "bperm")
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"permissions:\n  - id: prm_create_doc\n    action: create\n    roles: [author, reviewer]\n")
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	appLive, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, appFile, "roles: [author, reviewer]\n"+string(appLive))
	s.reloadWorkspace(t)
	part := "app_document_tracking/machines/mch_document/permissions/prm_create_doc"
	page := func() string {
		return s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(part)).Body.String()
	}
	body := page()
	for _, want := range []string{"Who may create", "/runtime-actions/set_permission_roles?record=", `value="author"`, `value="reviewer"`, "Save who may"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Permission's Edit tab is missing %q", want)
		}
	}
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	save := func(roles ...string) int {
		return s.postTo(t, "/runtime-actions/set_permission_roles?record="+url.QueryEscape(part), url.Values{"roles": roles}).Code
	}
	if code := save("author"); code != http.StatusSeeOther {
		t.Fatalf("narrowing who may: %d", code)
	}
	if got := read(); strings.Contains(got[strings.Index(got, "prm_create_doc"):], "reviewer") {
		t.Errorf("the role was not taken away:\n%s", got)
	}
	before := read()
	for name, roles := range map[string][]string{"nobody": nil, "a role the Application lacks": {"author", "janitor"}} {
		if code := save(roles...); code < 400 {
			t.Errorf("%s was accepted: %d", name, code)
		}
		if read() != before {
			t.Errorf("%s wrote the file", name)
		}
	}
	if code := save("author"); code != http.StatusSeeOther || read() != before {
		t.Errorf("an unchanged form must write nothing (status %d)", code)
	}
	if code := s.postTo(t, "/runtime-actions/set_permission_roles?record="+url.QueryEscape("app_document_tracking/machines/mch_document/permissions/prm_nope"), url.Values{"roles": {"author"}}).Code; code < 400 {
		t.Errorf("a Permission that is not there was accepted: %d", code)
	}
}

// A Field's Edit tab ticks who may change it: the roles of the Application, starting as the Field has them. Saving writes `editable_by`; narrowing or
// widening is a change, and clearing the last one is refused because it would open to everyone what was held back.
func TestBuilderEditTabSetsWhoMayChangeAField(t *testing.T) {
	s := newInstallTestSetup(t, "beditby")
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	appLive, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, appFile, "roles: [author, reviewer]\n"+string(appLive))
	s.reloadWorkspace(t)
	field := "app_document_tracking/machines/mch_document/fields/fld_title"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	body := s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(field)).Body.String()
	for _, want := range []string{`name="editable_by"`, `value="author"`, `value="reviewer"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the Edit tab does not offer %q", want)
		}
	}
	save := func(roles ...string) int {
		return s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Title"}, "required": {"on"}, "editable_by": roles}).Code
	}
	if code := save("reviewer"); code != http.StatusSeeOther || !strings.Contains(read(), "editable_by") {
		t.Fatalf("setting who may change: %d\n%s", code, read())
	}
	if body := s.getAs(t, "/builder/app_document_tracking?tab=edit&node="+url.QueryEscape(field)).Body.String(); !strings.Contains(body, `value="reviewer" checked`) {
		t.Errorf("the form does not start from the roles the Field has:\n%s", body)
	}
	before := read()
	if code := save(); code < 400 || read() != before {
		t.Errorf("clearing who may change was accepted or wrote (status %d)", code)
	}
	if code := save("janitor"); code < 400 || read() != before {
		t.Errorf("a role the Application lacks was accepted or wrote (status %d)", code)
	}
	if code := save("reviewer"); code != http.StatusSeeOther || read() != before {
		t.Errorf("an unchanged form must write nothing (status %d)", code)
	}
}

// The Ask AI tab opens the assistant's conversation about the selected part: the wish arrives with the part's Application, place and path in it, so the
// assistant is asked about that Field; it sends structure only, writes nothing, and wants a wish.
func TestBuilderAskTabOpensTheConversationAboutThePart(t *testing.T) {
	s := newInstallTestSetup(t, "bask")
	part := "app_document_tracking/machines/mch_document/fields/fld_title"
	body := s.getAs(t, "/builder/app_document_tracking?tab=ask&node="+url.QueryEscape(part)).Body.String()
	for _, want := range []string{"/runtime-actions/ask_assistant_about?record=", `name="wish"`, "Ask the assistant"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Ask AI tab is missing %q", want)
		}
	}
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	before, _ := os.ReadFile(file)
	rec := s.postTo(t, "/runtime-actions/ask_assistant_about?record="+url.QueryEscape(part), url.Values{"wish": {"make it a long text"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("asking: %d %s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || loc.Path != "/new-application" {
		t.Fatalf("the person is not sent to the conversation: %q", rec.Header().Get("Location"))
	}
	idea := loc.Query().Get("idea")
	for _, want := range []string{"app_document_tracking", "machines/mch_document/fields/fld_title", "make it a long text"} {
		if !strings.Contains(idea, want) {
			t.Errorf("the idea does not name %q: %s", want, idea)
		}
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Error("asking the assistant wrote a file")
	}
	if rec := s.postTo(t, "/runtime-actions/ask_assistant_about?record="+url.QueryEscape(part), url.Values{"wish": {"  "}}); rec.Code < 400 {
		t.Errorf("an empty wish was accepted: %d", rec.Code)
	}
}

// "Needs checking" lists what an Application declares and nothing uses -- a role no permission, transition, event or field names; a status choice with no
// colour beside coloured ones -- as parts of the tree, derived from the metadata, so the list is empty for an Application with nothing to look at and
// grows when something is declared and left alone.
func TestBuilderListsWhatNeedsChecking(t *testing.T) {
	s := newInstallTestSetup(t, "bchecks")
	tree := func() string { return s.getAs(t, "/builder/app_document_tracking").Body.String() }
	if body := tree(); strings.Contains(body, "Needs checking") {
		t.Errorf("an Application with nothing to look at lists checks:\n%s", body)
	}
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	appLive, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, "roles: [author, janitor]\n"+string(appLive))
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"  - id: fld_stage\n    name: Stage\n    type: status\n    options: [Open, Closed]\npermissions:\n  - id: prm_create_doc\n    action: create\n    roles: [author]\nstatus_categories:\n  field: fld_stage\n  values:\n    Open: start\n")
	s.reloadWorkspace(t)
	body := tree()
	for _, want := range []string{"Needs checking", "Role janitor is named by nothing", "no colour for Closed"} {
		if !strings.Contains(body, want) {
			t.Errorf("the tree does not list %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Role author is named by nothing") {
		t.Error("a role a permission names is listed as unused")
	}
}
