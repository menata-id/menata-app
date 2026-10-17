package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
)

// TestGroupsIsARuntimePage: /workspace-groups is the runtime's declared page. It lists the Workspace's Groups with their
// member count and what each grants, links each to its own screen, offers the form that creates one, sits in the
// settings frame with its own row marked current, and is opened by an admin only.
func TestGroupsIsARuntimePage(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "groupspage", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	g, err := store.CreateGroup(ctx, wsID, "GP-finance")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, g.ID) })
	empty, err := store.CreateGroup(ctx, wsID, "GP-empty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, empty.ID) })
	app := ws.Applications[0]
	if err := store.SetGroupAppRole(ctx, g.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}

	one, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "groupspage_one@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", one.ID) })
	if err := store.AddMember(ctx, wsID, one.ID, "groupspage_one@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetGroupMembers(ctx, g.ID, []string{one.ID}); err != nil {
		t.Fatal(err)
	}

	body := getPage(t, h, cookie, "/workspace-groups")
	for _, want := range []string{
		"GP-finance", "GP-empty", `href="/workspace-groups/` + g.ID + `"`, "1 member<", "0 members", "Grants no role yet",
		app.Name + " · " + app.Roles[0], "+ New group", `hx-post="/runtime-actions/create_group"`, "Create group",
		`href="/workspace-members"`, `<nav aria-label="Workspace settings"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Groups page does not contain %q", want)
		}
	}
	if n := strings.Count(body, `aria-current="page"`); n != 1 || !strings.Contains(body, `href="/workspace-groups" aria-current="page"`) {
		t.Errorf("%d current settings rows, want exactly the Groups one", n)
	}

	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "groupspage_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", member.ID) })
	if err := store.AddMember(context.Background(), wsID, member.ID, "groupspage_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/workspace-groups", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, config.Config{SessionSecret: "groupspage-secret"}, member.ID, 0)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "GP-finance") {
		t.Errorf("a member opening the Groups page = %d, want 403 and no group names", rec.Code)
	}
}
