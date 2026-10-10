package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestApplicationMembersPageAndWorkspaceMembersShareOneRole is S2.2's "done means" through the real
// router: a role changed on an Application's Members & roles page is the role the Workspace's Edit
// member screen shows, and the other way round; a non-admin is refused both the page and the write; an
// invalid role and an unknown member are refused without writing.
func TestApplicationMembersPageAndWorkspaceMembersShareOneRole(t *testing.T) {
	h, adminCookie, wsCtx, store, _, installed, _ := routerSetupFor(t, "appmembers", "default")
	cfg := config.Config{SessionSecret: "appmembers-secret"}

	var app domain.Application
	var hub domain.NavigationItem
	for _, a := range installed.Applications {
		for _, item := range a.AllNavigation {
			if item.SettingsHub && len(a.Roles) > 0 && app.ID == "" {
				app, hub = a, item
			}
		}
	}
	if app.ID == "" || len(app.Roles) < 2 {
		t.Fatal("this Workspace installs no Settings hub over an Application with at least two roles")
	}
	member, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": "appmembers_two@example.com"})
	if err != nil {
		t.Fatalf("CreateRecord(member): %v", err)
	}
	wsID, _ := data.WorkspaceScope(wsCtx)
	if err := store.AddMember(wsCtx, wsID, member.ID, "appmembers_two@example.com", "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	memberCookie := sessionCookieValueForTest(t, cfg, member.ID, 0)

	page := hub.Route + "/members"
	do := func(method, path, cookie string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if method == http.MethodPost {
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			token := csrfTokenFor(t, h, path)
			req.Header.Set("X-CSRF-Token", token.value)
			req.AddCookie(token.cookie)
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodGet, page, adminCookie, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", page, rec.Code, firstChars(rec.Body.String()))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "appmembers_two@example.com") {
		t.Error("the page does not list the second member")
	}
	if want := fmt.Sprintf("1 of 2 active members have access to %s.", app.Name); !strings.Contains(body, want) {
		t.Errorf("coverage sentence %q not found", want)
	}
	if !strings.Contains(body, `href="/workspace-members"`) {
		t.Error("the page does not link to the Workspace Members page")
	}
	if hubBody := getPage(t, h, adminCookie, hub.Route); !strings.Contains(hubBody, `href="`+page+`"`) {
		t.Errorf("the hub's Members & roles row does not point at %s", page)
	}

	// Pintu 2 -> Pintu 1.
	chosen := app.Roles[1]
	rec = do(http.MethodPost, page+"/"+member.ID, adminCookie, url.Values{memberRoleFormField: {chosen}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != page {
		t.Fatalf("POST role = %d %q, want 303 back to %s", rec.Code, rec.Header().Get("Location"), page)
	}
	edit := do(http.MethodGet, "/workspace-members/"+member.ID+"/edit", adminCookie, nil).Body.String()
	if !strings.Contains(edit, fmt.Sprintf(`<option value=%q selected`, chosen)) {
		t.Errorf("the Workspace Edit member screen does not show %q selected after it was set from the Application page", chosen)
	}
	if body := do(http.MethodGet, page, adminCookie, nil).Body.String(); !strings.Contains(body, fmt.Sprintf("2 of 2 active members have access to %s.", app.Name)) {
		t.Error("coverage did not rise after granting a role")
	}

	// Pintu 1 -> Pintu 2.
	other := app.Roles[0]
	rec = do(http.MethodPost, "/workspace-members/"+member.ID+"/edit", adminCookie,
		url.Values{"workspace_role": {"member"}, appRoleField(app.ID): {other}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST edit member = %d: %s", rec.Code, firstChars(rec.Body.String()))
	}
	if body := do(http.MethodGet, page, adminCookie, nil).Body.String(); !strings.Contains(body, fmt.Sprintf(`<option value=%q selected`, other)) {
		t.Errorf("the Application page does not show %q selected after it was set from the Workspace screen", other)
	}

	// Refusals write nothing.
	if rec := do(http.MethodPost, page+"/"+member.ID, adminCookie, url.Values{memberRoleFormField: {"not-a-role"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an undeclared role = %d, want 422", rec.Code)
	}
	if rec := do(http.MethodPost, page+"/usr_not_a_member", adminCookie, url.Values{memberRoleFormField: {chosen}}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown member = %d, want 404", rec.Code)
	}
	if m, err := store.GetMembership(wsCtx, wsID, member.ID); err != nil || m.AppRoles[app.ID] != other {
		t.Errorf("after the refusals the stored role = %v (err %v), want %q", m.AppRoles[app.ID], err, other)
	}

	// A non-admin holding a role here may open the Application but not administer who is in it.
	if rec := do(http.MethodGet, page, memberCookie, nil); rec.Code != http.StatusForbidden {
		t.Errorf("a non-admin GET = %d, want 403", rec.Code)
	}
	if rec := do(http.MethodPost, page+"/"+member.ID, memberCookie, url.Values{memberRoleFormField: {chosen}}); rec.Code != http.StatusForbidden {
		t.Errorf("a non-admin POST = %d, want 403", rec.Code)
	}
	if m, _ := store.GetMembership(wsCtx, wsID, member.ID); m.AppRoles[app.ID] != other {
		t.Error("a refused non-admin POST changed the stored role")
	}

	// Clearing the select removes the row.
	do(http.MethodPost, page+"/"+member.ID, adminCookie, url.Values{memberRoleFormField: {""}})
	if m, _ := store.GetMembership(wsCtx, wsID, member.ID); m.AppRoles[app.ID] != "" {
		t.Errorf("an empty role left %q stored", m.AppRoles[app.ID])
	}
}
