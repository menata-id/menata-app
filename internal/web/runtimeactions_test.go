package web

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/registry"
)

func TestRuntimeActionsAndRunnersAgree(t *testing.T) {
	var runners []string
	for id := range runtimeActionRunners {
		runners = append(runners, id)
	}
	slices.Sort(runners)
	if got := registry.RuntimeActionIDs(); !slices.Equal(got, runners) {
		t.Errorf("registry actions %v, runners %v: an action with no runner answers 404, a runner with no action can never be reached", got, runners)
	}
}

func TestRuntimeAction_createsAndDeletesAGroupForAnAdminOnly(t *testing.T) {
	h, cookie, ctx, store, _, _, _ := routerSetupFor(t, "runtimeaction", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	post := func(ck, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: ck})
		req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "t"})
		req.Header.Set("X-CSRF-Token", "t")
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	groupNamed := func(name string) *data.Group {
		gs, _ := store.ListGroups(ctx, wsID)
		for i := range gs {
			if gs[i].Name == name {
				return &gs[i]
			}
		}
		return nil
	}
	t.Cleanup(func() {
		if g := groupNamed("RA-group"); g != nil {
			_ = store.DeleteGroup(ctx, wsID, g.ID)
		}
	})

	if rec := post(cookie, "/runtime-actions/create_group?name=RA-group"); rec.Code != http.StatusOK || rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("create_group = %d (HX-Refresh %q); body=%s", rec.Code, rec.Header().Get("HX-Refresh"), rec.Body.String())
	}
	g := groupNamed("RA-group")
	if g == nil {
		t.Fatal("create_group created no group")
	}
	if rec := post(cookie, "/runtime-actions/create_group?name=%20%20"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a blank name = %d, want 422", rec.Code)
	}

	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "ra_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", member.ID) })
	if err := store.AddMember(ctx, wsID, member.ID, "ra_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	mck := sessionCookieValueForTest(t, config.Config{SessionSecret: "runtimeaction-secret"}, member.ID, 0)
	if rec := post(mck, "/runtime-actions/delete_group?record="+g.ID); rec.Code != http.StatusForbidden {
		t.Errorf("a member deleting a group = %d, want 403", rec.Code)
	}
	if groupNamed("RA-group") == nil {
		t.Error("a refused delete removed the group")
	}
	for name, path := range map[string]string{
		"an unknown action":          "/runtime-actions/nothing",
		"a record action with no id": "/runtime-actions/delete_group",
		"an id on a non-record one":  "/runtime-actions/create_group?name=x&record=" + g.ID,
	} {
		if rec := post(cookie, path); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 404 or 400", name, rec.Code)
		}
	}
	rec := post(cookie, "/runtime-actions/delete_group?record="+g.ID)
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Redirect") != "/workspace-groups" {
		t.Errorf("delete_group = %d (HX-Redirect %q), want 200 to the Groups list", rec.Code, rec.Header().Get("HX-Redirect"))
	}
	if groupNamed("RA-group") != nil {
		t.Error("delete_group left the group")
	}
}

// An action that says it is not admin-only would need a route of its own: this one rides the admin group, so it fails
// closed rather than being run.
func TestRuntimeAction_anActionNotMarkedAdminIsNotRunByTheAdminRoute(t *testing.T) {
	h, cookie, _, _, _, _, _ := routerSetupFor(t, "runtimeactionopen", "default")
	registry.RuntimeActions["zz_open"] = registry.RuntimeAction{ID: "zz_open", Admin: false}
	ran := false
	runtimeActionRunners["zz_open"] = func(http.ResponseWriter, *http.Request, actionEnv, string) (string, bool) {
		ran = true
		return "", true
	}
	t.Cleanup(func() { delete(registry.RuntimeActions, "zz_open"); delete(runtimeActionRunners, "zz_open") })
	req := httptest.NewRequest(http.MethodPost, "/runtime-actions/zz_open", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "t"})
	req.Header.Set("X-CSRF-Token", "t")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || ran {
		t.Errorf("a non-admin action through the admin route = %d, ran=%v, want 403 and not run", rec.Code, ran)
	}
}

// A workspace role is admin or member and nothing else, and a refused write changes nothing.
func TestRuntimeAction_setMemberAccessRefusesAnInvalidWorkspaceRole(t *testing.T) {
	h, cookie, ctx, store, _, _, _ := routerSetupFor(t, "memberaccess", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "ma_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", u.ID) })
	if err := store.AddMember(ctx, wsID, u.ID, "ma_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	post := func(role string) int {
		req := httptest.NewRequest(http.MethodPost, "/runtime-actions/set_member_access?record="+u.ID+"&workspace_role="+role, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "t"})
		req.Header.Set("X-CSRF-Token", "t")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post("emperor"); code != http.StatusUnprocessableEntity {
		t.Errorf("an invalid workspace role = %d, want 422", code)
	}
	if m, _ := store.GetMembership(ctx, wsID, u.ID); m == nil || m.WorkspaceRole != "member" {
		t.Errorf("a refused write changed the role: %+v", m)
	}
	if code := post("admin"); code != http.StatusSeeOther {
		t.Errorf("a valid workspace role = %d, want the redirect", code)
	}
	if m, _ := store.GetMembership(ctx, wsID, u.ID); m == nil || m.WorkspaceRole != "admin" {
		t.Errorf("the role did not land: %+v", m)
	}
}

func TestWebProvidedRuntimeSourcesHaveAReader(t *testing.T) {
	// Every source the registry leaves to the handler has a reader the handler supplies, and the handler supplies none for a
	// source composition provides.
	supplied := webRuntimeProviders(nil, config.Config{})
	for id, src := range registry.RuntimeSources {
		_, has := supplied[id]
		switch {
		case src.Provider == "web" && !has:
			t.Errorf("registry leaves source %q to the web package and nothing here supplies a reader for it", id)
		case src.Provider != "web" && has:
			t.Errorf("the web package supplies a reader for %q, which composition provides", id)
		}
	}
	for id := range supplied {
		if _, ok := registry.RuntimeSources[id]; !ok {
			t.Errorf("the web package supplies a reader for %q, which is no source", id)
		}
	}
}
