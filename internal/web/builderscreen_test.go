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
	"time"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
	"menata.app/internal/metadata"
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
	if body := edit("app_document_tracking/navigation"); !strings.Contains(body, "add_menu_item") {
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
	if staged, _ := os.ReadFile(file); strings.Contains(string(staged), "Headline") {
		t.Error("a form wrote the file: it should only stage the change")
	}
	if pub := s.postTo(t, "/runtime-actions/publish_draft?record=app_document_tracking", url.Values{}); pub.Code != http.StatusSeeOther {
		t.Fatalf("publishing the draft: %d %s", pub.Code, pub.Body.String())
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
	if rec := s.postBuilder(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Headline"}}); rec.Code != http.StatusSeeOther {
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
		if rec := s.postBuilder(t, "/runtime-actions/edit_field?record="+url.QueryEscape(post.record), post.form); rec.Code < 400 {
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
		return s.postBuilder(t, "/runtime-actions/edit_field?record="+url.QueryEscape(node), url.Values{"name": {"Kind"}, "options": {lines}}).Code
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
	for _, want := range []string{"/runtime-actions/add_field?record=", `name="type"`, `name="options"`, "Add field", "Choice list (or status)"} {
		if !strings.Contains(body, want) {
			t.Errorf("a Machine's Edit tab is missing %q", want)
		}
	}
	if strings.Contains(body, "edit_field") {
		t.Errorf("a Machine is offered the form that edits a Field")
	}
	// The pickers send the value, not the words: a select's option is `value` with its label, never "value=label" as its value.
	for _, want := range []string{`<option value="text"`, `>Text</option>`, `<option value="status"`, `>Choice list (or status)</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the type picker does not offer %q:\n%s", want, body)
		}
	}
	add := func(form url.Values) int {
		return s.postBuilder(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), form).Code
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
	if code := s.postBuilder(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine+"/fields/fld_title"), url.Values{"name": {"Z"}, "type": {"text"}}).Code; code < 400 {
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
		return s.postBuilder(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), form).Code
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
		return s.postBuilder(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Stage"}, "options": {lines}}).Code
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
		return s.postBuilder(t, "/runtime-actions/set_permission_roles?record="+url.QueryEscape(part), url.Values{"roles": roles}).Code
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
	if code := s.postBuilder(t, "/runtime-actions/set_permission_roles?record="+url.QueryEscape("app_document_tracking/machines/mch_document/permissions/prm_nope"), url.Values{"roles": {"author"}}).Code; code < 400 {
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
		return s.postBuilder(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Title"}, "required": {"on"}, "editable_by": roles}).Code
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

// postBuilder submits one Application Builder form and, when it was staged, publishes the draft, so a test that reads the file reads what the person
// would see after Publish. A refusal at staging comes back as it is; a form that staged nothing (nobody changed it) comes back as the 303 it answered.
func (s *installTestSetup) postBuilder(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := s.postTo(t, path, form)
	if rec.Code != http.StatusSeeOther {
		return rec
	}
	pub := s.postTo(t, "/runtime-actions/publish_draft?record=app_document_tracking", url.Values{})
	if pub.Code == http.StatusConflict {
		return rec
	}
	return pub
}

// The Builder's forms stage, and Publish writes: nothing reaches the files until then, the page says how many changes are waiting and what the plan says
// each would do, the draft is the admin's own, Discard drops it without writing, a change the plan refuses is refused at the form and never staged, and
// two staged changes are written together.
func TestBuilderFormsStageAndPublishWritesTheDraft(t *testing.T) {
	s := newInstallTestSetup(t, "bdraft")
	app := "app_document_tracking"
	field := app + "/machines/mch_document/fields/fld_title"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	before := read()
	page := func() string { return s.getAs(t, "/builder/"+app).Body.String() }
	stage := func(form url.Values) int {
		return s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), form).Code
	}
	if strings.Contains(page(), "not published") {
		t.Error("a draft bar is drawn before anything is staged")
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusConflict {
		t.Errorf("publishing nothing: %d, want 409", code)
	}

	if stage(url.Values{"name": {"Headline"}, "required": {"on"}}) != http.StatusSeeOther || stage(url.Values{"name": {"Headline"}}) != http.StatusSeeOther {
		t.Fatal("staging was refused")
	}
	if read() != before {
		t.Fatal("staging wrote the file")
	}
	bar := page()
	for _, want := range []string{"Draft · 3 changes not published", "rename field", "publish_draft", "discard_draft"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the draft bar is missing %q:\n%s", want, bar)
		}
	}
	if mine, _, err := s.store.GetAppDraft(data.WithWorkspaceScope(context.Background(), s.wsID), s.wsID, app, s.actorID); err != nil || mine == nil {
		t.Errorf("the draft is not kept under the admin who staged it: %v %v", mine, err)
	}
	if other, _, err := s.store.GetAppDraft(data.WithWorkspaceScope(context.Background(), s.wsID), s.wsID, app, "someone-else"); err != nil || other != nil {
		t.Errorf("another admin sees this draft: %v %v", other, err)
	}

	if code := s.postTo(t, "/runtime-actions/discard_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Fatalf("discarding: %d", code)
	}
	if read() != before || strings.Contains(page(), "not published") {
		t.Error("Discard wrote the file or left the bar")
	}

	stage(url.Values{"name": {"Headline"}})
	stage(url.Values{"name": {"Headline"}, "editable_by": {"janitor"}}) // refused: the Application declares no such role
	if !strings.Contains(page(), "Draft · 1 change not published") {
		t.Errorf("a refused form was staged, or the draft was lost:\n%s", page())
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Fatalf("publishing: %d", code)
	}
	if !strings.Contains(read(), "Headline") || strings.Contains(page(), "not published") {
		t.Errorf("Publish did not write the draft or did not clear it:\n%s", read())
	}
}

// The forms start from, and are compared with, the Field as the admin's draft leaves it: ticking a box and unticking it again stages the unticking (and
// Publish leaves the Field as it was), a rename and its reversal likewise, and the form shows what is staged.
func TestBuilderFormsCompareWithTheDraftNotTheInstallation(t *testing.T) {
	s := newInstallTestSetup(t, "bdiffdraft")
	app := "app_document_tracking"
	field := app + "/machines/mch_document/fields/fld_title"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	stage := func(form url.Values) int {
		return s.postTo(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), form).Code
	}
	before := read()
	stage(url.Values{"name": {"Headline"}, "required": {"on"}})
	if body := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(field)).Body.String(); !strings.Contains(body, `value="Headline"`) {
		t.Errorf("the form does not show the staged name:\n%s", body)
	}
	if code := stage(url.Values{"name": {"Title"}}); code != http.StatusSeeOther { // back to the old name, unticked: everything undone
		t.Fatalf("undoing the staged changes: %d", code)
	}
	if strings.Contains(s.getAs(t, "/builder/"+app).Body.String(), "not published") {
		t.Error("a draft whose changes undo one another is still offered for publishing")
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusConflict || read() != before {
		t.Errorf("publishing an undone draft: %d, file changed: %v", code, read() != before)
	}
	// Half undone: the rename reversed, the requirement kept.
	stage(url.Values{"name": {"Headline"}, "required": {"on"}})
	stage(url.Values{"name": {"Title"}, "required": {"on"}})
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Fatalf("publishing: %d", code)
	}
	if strings.Contains(read(), "Headline") || !strings.Contains(read(), "required: true") {
		t.Errorf("the half-undone draft was published wrongly:\n%s", read())
	}
}

// A Field added twice, or under a name that differs only in case or has no letter in it, gets its own id each time -- among the Fields the draft already
// holds -- and the page returns to the Machine, the one part that exists, rather than to a Field that is only staged.
func TestBuilderAddFieldKeepsIdsUniqueAmongStagedFields(t *testing.T) {
	s := newInstallTestSetup(t, "badduniq")
	app := "app_document_tracking"
	machine := app + "/machines/mch_document"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	for _, name := range []string{"Amount", "Amount", "AMOUNT", "é", "é"} {
		rec := s.postTo(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), url.Values{"name": {name}, "type": {"text"}})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("adding %q: %d %s", name, rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); strings.Contains(loc, "fields%2Ffld") || strings.Contains(loc, "/fields/") {
			t.Errorf("the page returns to a Field that is only staged: %q", loc)
		}
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Fatalf("publishing: %d", code)
	}
	b, _ := os.ReadFile(file)
	for _, want := range []string{"fld_amount", "fld_amount_2", "fld_amount_3", "fld_field", "fld_field_2"} {
		if !strings.Contains(string(b), "id: "+want+"\n") {
			t.Errorf("the file lacks %s:\n%s", want, b)
		}
	}
}

// A draft is the admin's until Publish and belongs to the Application as it was when the draft began: if that Application changed meanwhile the draft is
// refused (409) and kept, not replayed over what changed; and a draft dropped at publish does not come back.
func TestBuilderPublishRefusesADraftOfAnApplicationThatChanged(t *testing.T) {
	s := newInstallTestSetup(t, "bstale")
	app := "app_document_tracking"
	machine := app + "/machines/mch_document"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	if code := s.postTo(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), url.Values{"name": {"Note"}, "type": {"text"}}).Code; code != http.StatusSeeOther {
		t.Fatalf("staging: %d", code)
	}
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"  - id: fld_other\n    name: Other\n    type: text\n") // another admin published something
	s.reloadWorkspace(t)
	before, _ := os.ReadFile(file)
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusConflict {
		t.Errorf("publishing a stale draft: %d, want 409", code)
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Error("a stale draft was written")
	}
	if code := s.postTo(t, "/runtime-actions/discard_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Errorf("discarding: %d", code)
	}
	if code := s.postTo(t, "/runtime-actions/add_field?record="+url.QueryEscape(machine), url.Values{"name": {"Note"}, "type": {"text"}}).Code; code != http.StatusSeeOther {
		t.Fatalf("staging again: %d", code)
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Errorf("publishing the fresh draft: %d", code)
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusConflict {
		t.Errorf("publishing twice: %d, want 409 (nothing staged)", code)
	}
}

// A choice whose name holds a bar reads back from its own box, a colour is taken from the last bar only when it is one of the five, and a wish is held to
// a size and a part to the Workspace's own parts.
func TestBuilderChoiceBoxAndAskTabHoldTheirInputsToShape(t *testing.T) {
	got, err := parseChoices("a|b\nplain | done\nx | purple extra")
	if err == nil {
		t.Errorf("a stray bar tail was read as a name: %+v", got)
	}
	got, err = parseChoices("a|b\nplain | done\n  spaced  ")
	if err != nil || len(got) != 3 || got[0].Name != "a|b" || got[0].Category != "" || got[1].Name != "plain" || got[1].Category != "done" || got[2].Name != "spaced" {
		t.Errorf("parseChoices = %+v, %v", got, err)
	}
	s := newInstallTestSetup(t, "bshape")
	part := "app_document_tracking/machines/mch_document/fields/fld_title"
	if rec := s.postTo(t, "/runtime-actions/ask_assistant_about?record="+url.QueryEscape(part), url.Values{"wish": {strings.Repeat("x", 1001)}}); rec.Code < 400 {
		t.Errorf("an enormous wish was accepted: %d", rec.Code)
	}
	for _, record := range []string{"app_nowhere/machines/x", "app_document_tracking/", "app_document_tracking/../x", "app_document_tracking/a\"b"} {
		if rec := s.postTo(t, "/runtime-actions/ask_assistant_about?record="+url.QueryEscape(record), url.Values{"wish": {"hi"}}); rec.Code < 400 {
			t.Errorf("a part outside the tree was accepted: %q -> %d", record, rec.Code)
		}
	}
}

// A transition that writes a Field which names who may change it would be refused for everyone without that role, so the loader refuses the pairing; and a
// choice added and removed again in one draft leaves no colour behind.
func TestEditableByAndTransitionSetsAreRefusedTogether(t *testing.T) {
	s := newInstallTestSetup(t, "bsetsby")
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	appLive, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, "roles: [author, reviewer]\n"+string(appLive))
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"  - id: fld_stage\n    name: Stage\n    type: status\n    options: [Open, Closed]\n  - id: fld_closed_on\n    name: Closed on\n    type: date\n    editable_by: [reviewer]\n"+
		"transitions:\n  - id: trn_close\n    name: Close\n    field: fld_stage\n    from: Open\n    to: Closed\n    action: edit\n    sets: {fld_closed_on: today}\n")
	err := s.deps.ReloadMetadata(s.slug)
	if err == nil || !strings.Contains(err.Error(), "names who may change it") {
		t.Errorf("a transition setting an editable_by Field loaded: %v", err)
	}
}

// The menu is edited from the tree: the menu part adds an item that opens a Machine, and each screen the menu lists can be renamed, moved to another place or
// taken out. The forms stage like the others and Publish writes the Application's file in the order the draft left.
func TestBuilderMenuEditorAddsRenamesMovesAndRemoves(t *testing.T) {
	s := newInstallTestSetup(t, "bmenu")
	app := "app_document_tracking"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	tabEdit := func(node string) string {
		return s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(node)).Body.String()
	}
	first := app + "/navigation/nav_document_tracking"
	body := tabEdit(first)
	for _, want := range []string{"/runtime-actions/edit_menu_item?record=", `name="label"`, `name="position"`, `name="remove"`, "Save menu item"} {
		if !strings.Contains(body, want) {
			t.Errorf("a menu item's Edit tab is missing %q", want)
		}
	}
	if body := tabEdit(app + "/navigation"); !strings.Contains(body, `<option value="mch_document">Tracked Document</option>`) {
		t.Errorf("the Machine picker sends its words instead of the Machine's id:\n%s", body)
	}
	if body := tabEdit(app + "/navigation"); !strings.Contains(body, "/runtime-actions/add_menu_item?record=") || !strings.Contains(body, "Tracked Document") || !strings.Contains(body, "Add to the menu") {
		t.Errorf("the menu's Edit tab does not offer a new item over the Application's Machines:\n%s", body)
	}

	publish := func() {
		t.Helper()
		if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
			t.Fatalf("publishing: %d", code)
		}
	}
	if code := s.postTo(t, "/runtime-actions/add_menu_item?record="+url.QueryEscape(app+"/navigation"), url.Values{"label": {"Everything"}, "machine": {"mch_document"}}).Code; code != http.StatusSeeOther {
		t.Fatalf("adding an item: %d", code)
	}
	if strings.Contains(read(), "Everything") {
		t.Fatal("staging an item wrote the file")
	}
	publish()
	if !strings.Contains(read(), "label: Everything") {
		t.Fatalf("the new item is not in the Application's file:\n%s", read())
	}
	if strings.Index(read(), "Everything") < strings.Index(read(), "Document Tracking") {
		t.Fatalf("the new item is not last:\n%s", read())
	}

	edit := func(node string, form url.Values) int {
		return s.postTo(t, "/runtime-actions/edit_menu_item?record="+url.QueryEscape(node), form).Code
	}
	if code := edit(first, url.Values{"label": {"Papers"}, "position": {"2"}}); code != http.StatusSeeOther {
		t.Fatalf("renaming and moving: %d", code)
	}
	publish()
	text := read()
	if !strings.Contains(text, "label: Papers") || strings.Index(text, "label: Papers") < strings.Index(text, "label: Everything") {
		t.Errorf("the item was not renamed and moved after the other:\n%s", text)
	}
	before := read()
	if code := edit(first, url.Values{"label": {"Papers"}, "position": {"2"}}); code != http.StatusSeeOther || strings.Contains(s.getAs(t, "/builder/"+app).Body.String(), "not published") {
		t.Errorf("a form nobody changed staged something (status %d)", code)
	}
	for name, form := range map[string]url.Values{"a blank name": {"label": {" "}}, "a place that is not there": {"label": {"Papers"}, "position": {"9"}}} {
		if code := edit(first, form); code < 400 {
			t.Errorf("%s was accepted: %d", name, code)
		}
	}
	if read() != before {
		t.Error("a refused form wrote the file")
	}
	if code := edit(app+"/navigation/nav_nowhere", url.Values{"label": {"X"}}); code < 400 {
		t.Errorf("an item that is not there was accepted: %d", code)
	}
	if code := edit(first, url.Values{"label": {"Papers"}, "remove": {"on"}}); code != http.StatusSeeOther {
		t.Fatalf("taking an item out: %d", code)
	}
	publish()
	if strings.Contains(read(), "Papers") || !strings.Contains(read(), "Everything") {
		t.Errorf("the wrong item was taken out:\n%s", read())
	}
}

// A Field's Edit tab says when it is asked for: "Always", or while a status Field of its Machine holds one of its options. Saving stages the operation and
// Publish writes `show_when`; a pairing the loader would refuse (the Field watching itself, an option that is not there) is refused at the form.
func TestBuilderEditTabSetsWhenAFieldIsAskedFor(t *testing.T) {
	s := newInstallTestSetup(t, "bshowwhen")
	app := "app_document_tracking"
	file := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "document.yaml")
	live, _ := os.ReadFile(file)
	writeTestFile(t, file, string(live)+"  - id: fld_kind\n    name: Kind\n    type: status\n    options: [Contract, Invoice]\n  - id: fld_ref\n    name: Contract number\n    type: text\n")
	s.reloadWorkspace(t)
	field := app + "/machines/mch_document/fields/fld_ref"
	read := func() string { b, _ := os.ReadFile(file); return string(b) }
	body := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(field)).Body.String()
	for _, want := range []string{`name="show_when"`, `<option value="fld_kind:Contract"`, "Kind is Contract", `<option value=""`} {
		if !strings.Contains(body, want) {
			t.Errorf("the Edit tab does not offer %q", want)
		}
	}
	if strings.Contains(body, `value="fld_ref:`) {
		t.Error("a Field is offered itself as what it waits for")
	}
	save := func(when string) int {
		return s.postBuilder(t, "/runtime-actions/edit_field?record="+url.QueryEscape(field), url.Values{"name": {"Contract number"}, "show_when": {when}}).Code
	}
	if code := save("fld_kind:Contract"); code != http.StatusSeeOther || !strings.Contains(read(), "show_when") {
		t.Fatalf("asking only for a Contract: %d\n%s", code, read())
	}
	if body := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(field)).Body.String(); !strings.Contains(body, `<option value="fld_kind:Contract" selected`) {
		t.Errorf("the form does not start from the condition the Field has:\n%s", body)
	}
	before := read()
	for name, when := range map[string]string{"itself": "fld_ref:x", "an option that is not there": "fld_kind:Receipt", "a Field that is not there": "fld_nope:Contract"} {
		if code := save(when); code < 400 || read() != before {
			t.Errorf("%s was accepted or wrote (status %d)", name, code)
		}
	}
	if code := save(""); code != http.StatusSeeOther || strings.Contains(read(), "show_when") {
		t.Errorf("asking always again: %d\n%s", code, read())
	}
}

// Taking out a menu item that other items hang under is refused at the form in plain words, not by the loader after the write.
func TestBuilderMenuRefusesTakingOutAnItemOthersHangUnder(t *testing.T) {
	s := newInstallTestSetup(t, "bmenuparent")
	app := "app_document_tracking"
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	live, _ := os.ReadFile(appFile)
	text := string(live) + "  - id: nav_child\n    label: Child\n    route: /machines/mch_document?x=1\n    parent: nav_document_tracking\n"
	writeTestFile(t, appFile, text)
	s.reloadWorkspace(t)
	rec := s.postTo(t, "/runtime-actions/edit_menu_item?record="+url.QueryEscape(app+"/navigation/nav_document_tracking"), url.Values{"label": {"Document Tracking"}, "remove": {"on"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "hang under this one") {
		t.Errorf("taking out a parent: %d %s", rec.Code, rec.Body.String())
	}
}

// A menu item's Edit tab also hangs it under another item, says which roles are offered it and which Dataset it counts -- the three the runtime now has
// (parent, visible_to, badge) -- through the same operations a conversation would make; clearing who is offered an item is refused, and a count the menu declares
// that no Dataset is (approval_inbox_pending) is kept rather than replaced by the first choice.
func TestBuilderMenuEditorSetsParentVisibleToAndBadge(t *testing.T) {
	s := newInstallTestSetup(t, "bmenu3")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	appFile, docFile := filepath.Join(dir, "applications", "tracking.yaml"), filepath.Join(dir, "document.yaml")
	live, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, "roles: [author, reviewer]\n"+string(live))
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+"datasets:\n  - id: ds_all\n    select: records\n    limit: 50\n")
	s.reloadWorkspace(t)
	read := func() string { b, _ := os.ReadFile(appFile); return string(b) }
	post := func(path string, form url.Values) int { return s.postBuilder(t, path, form).Code }
	first, second := app+"/navigation/nav_document_tracking", ""
	if code := post("/runtime-actions/add_menu_item?record="+url.QueryEscape(app+"/navigation"), url.Values{"label": {"Everything"}, "machine": {"mch_document"}}); code != http.StatusSeeOther {
		t.Fatalf("adding an item: %d", code)
	}
	// The writer names the new item; find it.
	text := read()
	i := strings.Index(text, "label: Everything")
	if i < 0 {
		t.Fatalf("the new item is missing:\n%s", text)
	}
	idLine := text[strings.LastIndex(text[:i], "id: "):i]
	second = app + "/navigation/" + strings.TrimSpace(strings.TrimPrefix(strings.Split(idLine, "\n")[0], "id: "))

	body := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(second)).Body.String()
	for _, want := range []string{`name="parent"`, `<option value="nav_document_tracking"`, `name="visible_to"`, `value="reviewer"`, `name="badge"`, `<option value="ds_all"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the item's Edit tab is missing %q", want)
		}
	}
	edit := func(form url.Values) int {
		form.Set("label", "Everything")
		form.Set("position", "2")
		return post("/runtime-actions/edit_menu_item?record="+url.QueryEscape(second), form)
	}
	// Two items at one address are offered to the same roles, so the first is set in the same draft and both are published together.
	stage := func(node string, form url.Values) int {
		return s.postTo(t, "/runtime-actions/edit_menu_item?record="+url.QueryEscape(node), form).Code
	}
	if stage(first, url.Values{"label": {"Document Tracking"}, "position": {"1"}, "visible_to": {"reviewer"}}) != http.StatusSeeOther ||
		stage(second, url.Values{"label": {"Everything"}, "position": {"2"}, "parent": {"nav_document_tracking"}, "visible_to": {"reviewer"}, "badge": {"ds_all"}}) != http.StatusSeeOther {
		t.Fatal("staging the two items was refused")
	}
	if code := s.postTo(t, "/runtime-actions/publish_draft?record="+app, url.Values{}).Code; code != http.StatusSeeOther {
		t.Fatalf("setting parent, visible_to and badge: %d", code)
	}
	for _, want := range []string{"parent: nav_document_tracking", "visible_to:", "reviewer", "badge: ds_all"} {
		if !strings.Contains(read(), want) {
			t.Errorf("the Application's file lacks %q:\n%s", want, read())
		}
	}
	if body := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(second)).Body.String(); !strings.Contains(body, `<option value="nav_document_tracking" selected`) || !strings.Contains(body, `<option value="ds_all" selected`) {
		t.Errorf("the form does not start from what the item has:\n%s", body)
	}
	before := read()
	if code := edit(url.Values{"parent": {"nav_document_tracking"}, "badge": {"ds_all"}}); code < 400 || read() != before {
		t.Errorf("clearing who is offered an item was accepted or wrote (status %d)", code)
	}
	_ = edit
	if code := edit(url.Values{"parent": {"nav_nowhere"}, "visible_to": {"reviewer"}, "badge": {"ds_all"}}); code < 400 || read() != before {
		t.Errorf("a parent that is not an item was accepted or wrote (status %d)", code)
	}
	if code := edit(url.Values{"visible_to": {"reviewer"}, "badge": {"ds_all"}}); code != http.StatusSeeOther || strings.Contains(read(), "parent:") {
		t.Errorf("making the item a top-level one again: %d\n%s", code, read())
	}
	_ = first
}

// A count the menu declares that no Dataset is (`badge: approval_inbox_pending`) is kept by the item's form: the picker offers it, selected, so saving the form does not
// replace it with the first choice.
func TestBuilderMenuFormKeepsADeclaredKindOfCount(t *testing.T) {
	s := newInstallTestSetup(t, "bmenubadge")
	app := "app_document_tracking"
	appFile := filepath.Join(filepath.Dir(s.manifestPath), s.slug, "applications", "tracking.yaml")
	live, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, strings.Replace(string(live), "    route: /machines/mch_document\n", "    route: /machines/mch_document\n    badge: approval_inbox_pending\n", 1))
	s.reloadWorkspace(t)
	body := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(app+"/navigation/nav_document_tracking")).Body.String()
	if !strings.Contains(body, `<option value="approval_inbox_pending" selected`) {
		t.Errorf("the picker does not keep the declared kind of count selected:\n%s", body)
	}
}

// A list the assistant wrote has a View tab: its columns (by their Field names, in order), how it is ordered, the status its chips count by and the text Fields its search looks
// in. Saving stages the set_list_* operations; Publish writes the page and its Datasets again, and the screen the runtime draws then shows the new columns. A screen that is not such a
// list says so; a column that is no Field is refused in words.
func TestBuilderViewTabChangesAListsColumnsOrderChipsAndSearch(t *testing.T) {
	s := newInstallTestSetup(t, "bview")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	doc, _ := os.ReadFile(filepath.Join(dir, "document.yaml"))
	writeTestFile(t, filepath.Join(dir, "document.yaml"), string(doc)+
		"  - id: fld_note\n    name: Note\n    type: text\n  - id: fld_stage\n    name: Stage\n    type: status\n    options: [Open, Closed]\n"+
		"card_fields:\n  - { field: fld_title, role: title }\ndatasets:\n  - id: ds_listing_list\n    select: records\n    sort: [{ field: created_at, direction: desc }]\n    limit: 50\n")
	appFile := filepath.Join(dir, "applications", "tracking.yaml")
	live, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, string(live)+"  - id: nav_listing\n    label: Listing\n    route: /pages/nav_listing\n    icon: board\n    page:\n      layout: stack\n      gap: default\n      children:\n        - component: Table\n          label: Listing\n          empty: Nothing yet.\n          framed: true\n          columns:\n            - { header: Title, field: fld_title, href: record }\n          binding: { dataset: ds_listing_list, rows: records }\n")
	s.reloadWorkspace(t)
	node := app + "/navigation/nav_listing"
	body := s.getAs(t, "/builder/"+app+"?tab=view&node="+url.QueryEscape(node)).Body.String()
	for _, want := range []string{"/runtime-actions/edit_list?record=", `name="columns"`, `name="sort"`, `name="direction"`, `name="filter"`, `name="search"`, `name="density"`, `name="group_by"`, `name="row_tone"`, "Save view"} {
		if !strings.Contains(body, want) {
			t.Errorf("the View tab of a list is missing %q", want)
		}
	}
	if !strings.Contains(body, `<option value="fld_stage"`) || !strings.Contains(body, `<option value="created_at"`) {
		t.Errorf("the pickers do not offer the Machine's Fields:\n%s", body)
	}
	if body := s.getAs(t, "/builder/"+app+"?tab=view&node="+url.QueryEscape(app+"/navigation/nav_document_tracking")).Body.String(); !strings.Contains(body, "can be edited so far") || strings.Contains(body, "edit_list") {
		t.Errorf("a screen that is not a list is offered the list form")
	}
	save := func(form url.Values) int {
		return s.postBuilder(t, "/runtime-actions/edit_list?record="+url.QueryEscape(node), form).Code
	}
	if code := save(url.Values{"columns": {"Title\nNote\nStage"}, "sort": {"fld_title"}, "direction": {"asc"}, "filter": {"fld_stage"}, "search": {"fld_title", "fld_note"}, "density": {"compact"}, "group_by": {"fld_stage"}}); code != http.StatusSeeOther {
		t.Fatalf("saving the view: %d", code)
	}
	pageRec := s.getAs(t, "/pages/nav_listing")
	page := pageRec.Body.String()
	if pageRec.Code != http.StatusOK {
		t.Fatalf("the list page: %d %s", pageRec.Code, page)
	}
	for _, want := range []string{"Open", "Closed", `name="q"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the list the runtime draws lacks %q:\n%s", want, page)
		}
	}
	if written, _ := os.ReadFile(appFile); !strings.Contains(string(written), "density: compact") || !strings.Contains(string(written), "group_by: fld_stage") {
		t.Errorf("the layout is not in the page:\n%s", written)
	}
	if written, _ := os.ReadFile(appFile); !strings.Contains(string(written), "field: fld_note") || !strings.Contains(string(written), "field: fld_stage") {
		t.Errorf("the page's columns are not the ones saved:\n%s", written)
	}
	read, _ := os.ReadFile(filepath.Join(dir, "document.yaml"))
	if strings.Count(string(read), "id: ds_listing_list") != 1 {
		t.Errorf("the old Dataset stayed beside the new:\n%s", read)
	}
	before, _ := os.ReadFile(appFile)
	for name, form := range map[string]url.Values{
		"a column that is no Field": {"columns": {"Title\nNowhere"}},
		"no column":                 {"columns": {"  "}},
		"chips over a text Field":   {"columns": {"Title"}, "filter": {"fld_title"}},
	} {
		if code := save(form); code < 400 {
			t.Errorf("%s was accepted: %d", name, code)
		}
	}
	if after, _ := os.ReadFile(appFile); string(after) != string(before) {
		t.Error("a refused view wrote the file")
	}
}

// The Workflow part of the tree draws the Machine's lifecycle: each status with the meaning it is given, how many records stand in it, the edges that leave it and who may take
// each, and a word about a status nothing leaves that is not a finish. A Machine whose status no transition governs has no process to draw.
func TestBuilderWorkflowPartDrawsTheProcess(t *testing.T) {
	s := newInstallTestSetup(t, "bflow")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	appFile := filepath.Join(dir, "applications", "tracking.yaml")
	live, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, "roles: [author, manager]\n"+string(live))
	doc, _ := os.ReadFile(filepath.Join(dir, "document.yaml"))
	writeTestFile(t, filepath.Join(dir, "document.yaml"), string(doc)+
		"  - id: fld_stage\n    name: Stage\n    type: status\n    default: Open\n    options: [Open, Review, Done, Stuck]\n"+
		"status_categories:\n  field: fld_stage\n  values: { Open: start, Review: active, Done: done, Stuck: attention }\n"+
		"transitions:\n"+
		"  - { id: trn_submit, name: Submit, field: fld_stage, from: Open, to: Review, action: edit, roles: [author] }\n"+
		"  - { id: trn_finish, name: Finish, field: fld_stage, from: Review, to: Done, action: edit, roles: [manager] }\n"+
		"  - { id: trn_park, name: Park, field: fld_stage, from: Review, to: Stuck, action: edit }\n")
	s.reloadWorkspace(t)
	node := app + "/machines/mch_document/workflow"
	body := s.getAs(t, "/builder/"+app+"?node="+url.QueryEscape(node)).Body.String()
	for _, want := range []string{"Submit → Review", "author", "Finish → Done", "manager", "Park → Stuck", "Nothing leaves Stuck", "0 records"} {
		if !strings.Contains(body, want) {
			t.Errorf("the process is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Nothing leaves Done") || strings.Contains(body, "Nothing leaves Open") {
		t.Error("a finish, or a status with a way out, is flagged as a dead end")
	}
	other := s.getAs(t, "/builder/"+app+"?node="+url.QueryEscape(app+"/machines/mch_document/fields/fld_title")).Body.String()
	if strings.Contains(other, "Submit → Review") {
		t.Error("another part shows the process")
	}
}

// The Workflow part's Edit tab adds to the lifecycle: a status with what it means, a move between two statuses with who may take it (the new status may be named in the same form),
// and the move that gets a dead end out is how a dead end stops being one. What is there cannot be changed here -- the policy of an update.
func TestBuilderWorkflowEditAddsAStatusAndAMove(t *testing.T) {
	s := newInstallTestSetup(t, "bflow2")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	appFile := filepath.Join(dir, "applications", "tracking.yaml")
	live, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, "roles: [author, manager]\n"+string(live))
	docFile := filepath.Join(dir, "document.yaml")
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+
		"  - id: fld_stage\n    name: Stage\n    type: status\n    default: Open\n    options: [Open, Review, Done, Stuck]\n"+
		"status_categories:\n  field: fld_stage\n  values: { Open: start, Review: active, Done: done, Stuck: attention }\n"+
		"transitions:\n"+
		"  - { id: trn_submit, name: Submit, field: fld_stage, from: Open, to: Review, action: edit, roles: [author] }\n"+
		"  - { id: trn_finish, name: Finish, field: fld_stage, from: Review, to: Done, action: edit, roles: [manager] }\n"+
		"  - { id: trn_park, name: Park, field: fld_stage, from: Review, to: Stuck, action: edit }\n")
	s.reloadWorkspace(t)
	node := app + "/machines/mch_document/workflow"
	form := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(node)).Body.String()
	for _, want := range []string{"/runtime-actions/edit_workflow?record=", `name="status"`, `name="meaning"`, `name="step"`, `name="from"`, `name="to"`, `<option value="+new"`, `<option value="Stuck"`, `value="manager"`} {
		if !strings.Contains(form, want) {
			t.Errorf("the Workflow part's Edit tab is missing %q", want)
		}
	}
	post := func(v url.Values) int {
		return s.postBuilder(t, "/runtime-actions/edit_workflow?record="+url.QueryEscape(node), v).Code
	}
	// A status that finishes the work, a move into it by a manager, and the move that gets Stuck out of its dead end.
	if code := post(url.Values{"status": {"Archived"}, "meaning": {"stopped"}, "step": {"Archive"}, "from": {"Done"}, "to": {"+new"}, "roles": {"manager"}}); code != http.StatusSeeOther {
		t.Fatalf("adding a status and a move into it: %d", code)
	}
	if code := post(url.Values{"step": {"Reopen"}, "from": {"Stuck"}, "to": {"Review"}}); code != http.StatusSeeOther {
		t.Fatalf("adding a move: %d", code)
	}
	written, _ := os.ReadFile(docFile)
	for _, want := range []string{"Archived", "trn_archive", "trn_reopen", "Archived: stopped"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("the Machine's file lacks %q:\n%s", want, written)
		}
	}
	if i := strings.Index(string(written), "id: trn_archive"); i < 0 || !strings.Contains(string(written)[i:min(i+300, len(written))], "manager") {
		t.Errorf("the move Archive does not carry the role that may take it:\n%s", written)
	}
	body := s.getAs(t, "/builder/"+app+"?node="+url.QueryEscape(node)).Body.String()
	if strings.Contains(body, "Nothing leaves Stuck") || !strings.Contains(body, "Reopen → Review") || !strings.Contains(body, "Archive → Archived") {
		t.Errorf("the process does not show the new moves, or still calls Stuck a dead end:\n%s", body)
	}
	before := string(written)
	for name, v := range map[string]url.Values{
		"nothing named":                  {},
		"a move without an end":          {"step": {"Lost"}, "from": {"Done"}},
		"a meaning not known":            {"status": {"X"}, "meaning": {"purple"}},
		"a move from a status not there": {"step": {"Nope"}, "from": {"Gone"}, "to": {"Done"}},
	} {
		if code := post(v); code < 400 {
			t.Errorf("%s was accepted: %d", name, code)
		}
	}
	if after, _ := os.ReadFile(docFile); string(after) != before {
		t.Error("a refused form wrote the file")
	}
}

// "Try the flow" writes nothing: standing at a status as a role (the preview chips set the role), each status says whether you are there or a move you may take leads
// to it, and the link on a status moves the trial there.
func TestBuilderWorkflowTrialShowsWhereARoleCouldGo(t *testing.T) {
	s := newInstallTestSetup(t, "bflow3")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	appFile := filepath.Join(dir, "applications", "tracking.yaml")
	live, _ := os.ReadFile(appFile)
	writeTestFile(t, appFile, "roles: [author, manager]\n"+string(live))
	docFile := filepath.Join(dir, "document.yaml")
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+
		"  - id: fld_stage\n    name: Stage\n    type: status\n    default: Open\n    options: [Open, Review, Done]\n"+
		"status_categories:\n  field: fld_stage\n  values: { Open: start, Review: active, Done: done }\n"+
		"transitions:\n"+
		"  - { id: trn_submit, name: Submit, field: fld_stage, from: Open, to: Review, action: edit, roles: [author] }\n"+
		"  - { id: trn_finish, name: Finish, field: fld_stage, from: Review, to: Done, action: edit, roles: [manager] }\n")
	s.reloadWorkspace(t)
	node := url.QueryEscape(app + "/machines/mch_document/workflow")
	at := func(query string) string { return s.getAs(t, "/builder/"+app+"?node="+node+query).Body.String() }
	before, _ := os.ReadFile(docFile)
	if body := at("&try=Open&as=author"); !strings.Contains(body, "you are here") || !strings.Contains(body, "Submit gets you here") || strings.Contains(body, "Finish gets you here") {
		t.Errorf("an author at Open: %s", body)
	}
	if body := at("&try=Review&as=author"); strings.Contains(body, "Finish gets you here") {
		t.Errorf("an author was shown a move only a manager may take")
	}
	if body := at("&try=Review&as=manager"); !strings.Contains(body, "Finish gets you here") {
		t.Errorf("a manager at Review cannot finish")
	}
	if body := at("&try=Review"); !strings.Contains(body, "Finish gets you here") {
		t.Errorf("the builder, with no role chosen, can take any move")
	}
	if body := at("&try=Open&as=author"); !strings.Contains(body, "try=Review") {
		t.Errorf("a status does not link to trying from it:\n%s", body)
	}
	if after, _ := os.ReadFile(docFile); string(after) != string(before) {
		t.Error("trying the flow wrote a file")
	}
}

// Rules are parts of the tree: each Event of a Machine is said in a line ("When Status becomes Approved: tell the person in Requester"), with whether it is on and when it last ran; the Details
// of a rule list its runs -- ran, quiet, failed, and why -- which is how a rule that is silent is told from one that is broken; the Edit tab switches it, and a switched-off rule does not run.
func TestBuilderRulesAreListedSwitchedAndTheirRunsShown(t *testing.T) {
	s := newInstallTestSetup(t, "brules")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	docFile := filepath.Join(dir, "document.yaml")
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+
		"  - id: fld_stage\n    name: Stage\n    type: status\n    default: Open\n    options: [Open, Done]\n"+
		"events:\n  - id: evt_done\n    on: fld_stage\n    when_equals: Done\n    then: { service: log_activity, summary: \"{fld_title} is done\" }\n")
	s.reloadWorkspace(t)
	rule := app + "/machines/mch_document/events/evt_done"
	tree := s.getAs(t, "/builder/"+app).Body.String()
	for _, want := range []string{"Rules", "When Stage becomes Done: write an activity entry"} {
		if !strings.Contains(tree, want) {
			t.Errorf("the tree lacks %q", want)
		}
	}
	if body := s.getAs(t, "/builder/"+app+"?node="+url.QueryEscape(rule)).Body.String(); !strings.Contains(body, "never ran") {
		t.Errorf("a rule that has not run says so:\n%s", body)
	}
	// The history: a run is shown with its outcome and its reason.
	wsCtx := data.WithWorkspaceScope(context.Background(), s.wsID)
	if err := s.store.RecordEventRun(wsCtx, "mch_document", "evt_done", "rec_1", data.EventQuietOutcome, "nobody to tell: the field fld_x is empty"); err != nil {
		t.Fatal(err)
	}
	if err := s.store.RecordEventRun(wsCtx, "mch_document", "evt_done", "rec_2", data.EventFailedOutcome, "sending email to a@b: refused"); err != nil {
		t.Fatal(err)
	}
	body := s.getAs(t, "/builder/"+app+"?node="+url.QueryEscape(rule)).Body.String()
	for _, want := range []string{"Quiet", "nobody to tell", "Failed", "sending email to a@b: refused", "record rec_2"} {
		if !strings.Contains(body, want) {
			t.Errorf("the rule's runs lack %q:\n%s", want, body)
		}
	}
	// The switch.
	form := s.getAs(t, "/builder/"+app+"?tab=edit&node="+url.QueryEscape(rule)).Body.String()
	if !strings.Contains(form, "/runtime-actions/edit_rule?record=") || !strings.Contains(form, `name="enabled"`) {
		t.Errorf("the rule's Edit tab has no switch:\n%s", form)
	}
	switchTo := func(values url.Values) int {
		return s.postBuilder(t, "/runtime-actions/edit_rule?record="+url.QueryEscape(rule), values).Code
	}
	if code := switchTo(url.Values{}); code != http.StatusSeeOther {
		t.Fatalf("switching off: %d", code)
	}
	if written, _ := os.ReadFile(docFile); !strings.Contains(string(written), "enabled: false") {
		t.Errorf("the rule is not switched off in its file:\n%s", written)
	}
	if code := switchTo(url.Values{}); code != http.StatusSeeOther {
		t.Errorf("switching off a switched-off rule: %d", code)
	}
	if code := switchTo(url.Values{"enabled": {"on"}}); code != http.StatusSeeOther {
		t.Fatalf("switching on: %d", code)
	}
	if written, _ := os.ReadFile(docFile); strings.Contains(string(written), "enabled: false") {
		t.Errorf("the rule stayed switched off:\n%s", written)
	}
	if code := s.postBuilder(t, "/runtime-actions/edit_rule?record="+url.QueryEscape(app+"/machines/mch_document/events/evt_nowhere"), url.Values{}).Code; code < 400 {
		t.Errorf("a rule that is not there was switched: %d", code)
	}
}

// "Try the rule" asks a rule what it would do for a real record and does none of it: the answer is in words on the rule's page, no mail goes, no record is written, no run is remembered.
func TestBuilderTryARuleOnARealRecordWritesNothing(t *testing.T) {
	s := newInstallTestSetup(t, "btry")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	docFile := filepath.Join(dir, "document.yaml")
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+
		"  - id: fld_stage\n    name: Stage\n    type: status\n    default: Open\n    options: [Open, Done]\n"+
		"card_fields:\n  - { field: fld_title, role: title }\n"+
		"events:\n  - id: evt_done\n    on: fld_stage\n    when_equals: Done\n    then: { service: log_activity, summary: \"{fld_title} is done\" }\n")
	s.reloadWorkspace(t)
	wsCtx := data.WithWorkspaceScope(context.Background(), s.wsID)
	rec, err := s.store.CreateRecord(wsCtx, "mch_document", map[string]any{"fld_title": "Quarterly report", "fld_stage": "Open"})
	if err != nil {
		t.Fatal(err)
	}
	rule := app + "/machines/mch_document/events/evt_done"
	form := s.getAs(t, "/builder/"+app+"?tab=test&node="+url.QueryEscape(rule)).Body.String()
	if !strings.Contains(form, "/runtime-actions/try_rule?record=") || !strings.Contains(form, `<option value="`+rec.ID+`"`) || !strings.Contains(form, "Quarterly report") {
		t.Errorf("the Try tab does not offer the record to try it on:\n%s", form)
	}
	before, _ := s.store.ListRecords(wsCtx, "mch_activity")
	post := s.postTo(t, "/runtime-actions/try_rule?record="+url.QueryEscape(rule), url.Values{"record": {rec.ID}})
	if post.Code != http.StatusSeeOther || !strings.Contains(post.Header().Get("Location"), "rec="+rec.ID) {
		t.Fatalf("trying: %d %s", post.Code, post.Header().Get("Location"))
	}
	said := s.getAs(t, post.Header().Get("Location")).Body.String()
	for _, want := range []string{"becomes Done", "write an activity entry: Quarterly report is done"} {
		if !strings.Contains(said, want) {
			t.Errorf("the trial's answer lacks %q:\n%s", want, said)
		}
	}
	if other := s.getAs(t, "/builder/"+app+"?tab=test&rec=rec_nowhere&node="+url.QueryEscape(rule)).Body.String(); !strings.Contains(other, "not one of this Machine") {
		t.Errorf("a record that is not the Machine's:\n%s", other)
	}
	after, _ := s.store.ListRecords(wsCtx, "mch_activity")
	if len(after) != len(before) {
		t.Errorf("trying a rule wrote %d activity entries", len(after)-len(before))
	}
	if runs, _ := s.store.RecentEventRuns(wsCtx, "mch_document", "evt_done", 5); len(runs) != 0 {
		t.Errorf("a trial was remembered as a run: %+v", runs)
	}
	if code := s.postTo(t, "/runtime-actions/try_rule?record="+url.QueryEscape(rule), url.Values{}).Code; code < 400 {
		t.Errorf("trying with no record: %d", code)
	}
}

// A Machine's Rules tab starts a rule from an example: the form offers the four, the Fields to watch, and refuses in words what the example cannot make; what it makes is an ordinary rule
// that then appears in the tree and can be switched off.
func TestBuilderRulesTabStartsARuleFromAnExample(t *testing.T) {
	s := newInstallTestSetup(t, "bnewrule")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	docFile := filepath.Join(dir, "document.yaml")
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+"  - id: fld_owner\n    name: Owner\n    type: person\n")
	// A rule that tells someone writes to the notification Machine, which the Workspace must hold.
	notification, err := os.ReadFile(filepath.Join("..", "..", "metadata", "notification.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "notification.yaml"), string(notification))
	manifest, _ := os.ReadFile(s.manifestPath)
	writeTestFile(t, s.manifestPath, strings.Replace(string(manifest), "machines:\n", "machines:\n  - "+s.slug+"/notification.yaml\n", 1))
	s.reloadWorkspace(t)
	machine := app + "/machines/mch_document"
	form := s.getAs(t, "/builder/"+app+"?tab=rules&node="+url.QueryEscape(machine)).Body.String()
	for _, want := range []string{"/runtime-actions/add_rule?record=", `name="example"`, `<option value="assigned"`, `<option value="close"`, `name="watch"`, `<option value="fld_owner"`, "Add the rule"} {
		if !strings.Contains(form, want) {
			t.Errorf("the Rules tab is missing %q", want)
		}
	}
	add := func(v url.Values) int {
		return s.postBuilder(t, "/runtime-actions/add_rule?record="+url.QueryEscape(machine), v).Code
	}
	if code := add(url.Values{"example": {"assigned"}, "watch": {"fld_title"}}); code < 400 {
		t.Errorf("an assigned rule over a text field: %d", code)
	}
	if code := add(url.Values{"example": {"teleport"}, "watch": {"fld_owner"}}); code < 400 {
		t.Errorf("an example that is not one: %d", code)
	}
	if code := add(url.Values{"example": {"assigned"}, "watch": {"fld_owner"}}); code != http.StatusSeeOther {
		t.Fatalf("an assigned rule over a person field: %d", code)
	}
	if written, _ := os.ReadFile(docFile); !strings.Contains(string(written), "recipient_field: fld_owner") {
		t.Errorf("the rule is not in the Machine's file:\n%s", written)
	}
	if tree := s.getAs(t, "/builder/"+app).Body.String(); !strings.Contains(tree, "tell the person in Owner") {
		t.Errorf("the new rule is not in the tree:\n%s", tree)
	}
}

// The two rules on a schedule the examples make actually run on their own: the one that closes a record by itself moves an overdue record to its last status once, and writes the
// entry the runtime reads to tell it has already run (so a second tick does nothing); the history remembers the run. (The fixture holds the activity Machine that entry is written to.)
func TestTheCloseExampleRunsOnItsOwnAndOnlyOnce(t *testing.T) {
	s := newInstallTestSetup(t, "bcloses")
	app := "app_document_tracking"
	dir := filepath.Join(filepath.Dir(s.manifestPath), s.slug)
	docFile := filepath.Join(dir, "document.yaml")
	doc, _ := os.ReadFile(docFile)
	writeTestFile(t, docFile, string(doc)+"  - id: fld_due\n    name: Due\n    type: date\n  - id: fld_stage\n    name: Stage\n    type: status\n    default: Open\n    options: [Open, Closed]\ncard_fields:\n  - { field: fld_title, role: title }\n")
	s.reloadWorkspace(t)
	machine := app + "/machines/mch_document"
	if code := s.postBuilder(t, "/runtime-actions/add_rule?record="+url.QueryEscape(machine), url.Values{"example": {"close"}, "watch": {"fld_due"}, "other": {"fld_stage"}, "value": {"Closed"}}).Code; code != http.StatusSeeOther {
		t.Fatalf("adding the close rule: %d", code)
	}
	loaded, err := metadata.LoadApplication(s.manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	machines := map[string]*domain.Machine{}
	var list []*domain.Machine
	for _, m := range loaded.Workspace.Machines {
		machines[m.ID] = m
		list = append(list, m)
	}
	wsCtx := data.WithWorkspaceScope(context.Background(), s.wsID)
	late, err := s.store.CreateRecord(wsCtx, "mch_document", map[string]any{"fld_title": "Late", "fld_due": "2020-01-01", "fld_stage": "Open"})
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.RunScheduledEvents(wsCtx, s.store, nil, list, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.store.GetRecord(wsCtx, "mch_document", late.ID)
	if got.Values["fld_stage"] != "Closed" {
		t.Fatalf("the overdue record was not closed by the rule: %v", got.Values)
	}
	activity, _ := s.store.ListRecords(wsCtx, "mch_activity")
	if err := execution.RunScheduledEvents(wsCtx, s.store, nil, list, time.Now()); err != nil {
		t.Fatal(err)
	}
	again, _ := s.store.ListRecords(wsCtx, "mch_activity")
	if len(again) != len(activity) {
		t.Errorf("a second tick ran the rules again: %d -> %d activity entries", len(activity), len(again))
	}
	runs, _ := s.store.RecentEventRuns(wsCtx, "mch_document", current0(t, s, "close"), 5)
	if len(runs) == 0 || runs[0].Outcome != data.EventRanOutcome || !strings.Contains(runs[0].Detail, "Closed") {
		t.Errorf("the run is not remembered as ran-and-closed: %+v", runs)
	}
	_ = machines
}

// current0 is the id of the rule an example made, found by the word in its id.
func current0(t *testing.T, s *installTestSetup, word string) string {
	t.Helper()
	loaded, err := metadata.LoadApplication(s.manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range loaded.Workspace.Machines {
		for _, e := range m.Events {
			if strings.HasSuffix(e.ID, "_"+word) {
				return e.ID
			}
		}
	}
	t.Fatalf("no rule ends in _%s", word)
	return ""
}
