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

// TestGroupDetailIsARuntimePageForOneRecord: /workspace-groups/<id> draws that Group's own screen -- its name and count,
// every member as a checkbox ticked for those in it, a role select per Application starting as the Group's grant, and a
// delete that asks first -- inside the settings frame with Groups marked current; an id the source does not hold is a 404.
func TestGroupDetailIsARuntimePageForOneRecord(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "groupdetail", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	g, err := store.CreateGroup(ctx, wsID, "GD-finance")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, g.ID) })
	other, err := store.CreateGroup(ctx, wsID, "GD-other")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, other.ID) })
	var ids []string
	for _, email := range []string{"groupdetail_in@example.com", "groupdetail_out@example.com"} {
		u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", u.ID) })
		if err := store.AddMember(ctx, wsID, u.ID, email, "member", ""); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}
	if err := store.SetGroupMembers(ctx, g.ID, ids[:1]); err != nil {
		t.Fatal(err)
	}
	app := applicationWithRoles(t, ws)
	if err := store.SetGroupAppRole(ctx, g.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}

	body := getPage(t, h, cookie, "/workspace-groups/"+g.ID)
	for _, want := range []string{
		"GD-finance", "1 member<", `href="/workspace-groups"`, "Members", "Application roles",
		`name="member" value="` + ids[0] + `" checked`, `name="member" value="` + ids[1] + `" class`,
		`hx-post="/runtime-actions/set_group_members?record=` + g.ID + `"`, `hx-post="/runtime-actions/set_group_roles?record=` + g.ID + `"`,
		`hx-post="/runtime-actions/delete_group?record=` + g.ID + `"`, "hx-confirm=", `name="app_role[` + app.ID + `]"`, `<option value="` + app.Roles[0] + `" selected`,
		"Save members", "Save roles", "Delete this group",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Group page does not contain %q", want)
		}
	}
	if strings.Contains(body, "GD-other") {
		t.Error("the page shows another Group")
	}
	if n := strings.Count(body, `aria-current="page"`); n != 1 || !strings.Contains(body, `href="/workspace-groups" aria-current="page"`) {
		t.Errorf("%d current settings rows, want exactly Groups", n)
	}
	if strings.Count(body, "<h1") != 1 {
		t.Errorf("%d headings, want the Group's own and not a second generic one", strings.Count(body, "<h1"))
	}

	req := httptest.NewRequest(http.MethodGet, "/workspace-groups/grp_does_not_exist", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unknown group = %d, want 404", rec.Code)
	}
}

// TestMembersIsARuntimePage: /workspace-members is the runtime's declared page. It lists the members with what each holds in
// one line and whether they are active, the invitations nobody accepted with a Revoke, the form that invites, and a search that
// narrows both from `?q=`; an admin opens it and a member does not.
func TestMembersIsARuntimePage(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "memberspage", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	mk := func(email string, deactivate bool) string {
		u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", u.ID) })
		if err := store.AddMember(ctx, wsID, u.ID, email, "member", ""); err != nil {
			t.Fatal(err)
		}
		if deactivate {
			if err := store.DeactivateMember(ctx, wsID, u.ID); err != nil {
				t.Fatal(err)
			}
		}
		return u.ID
	}
	activeID := mk("mp_active@example.com", false)
	mk("mp_gone@example.com", true)
	if err := store.CreatePendingInvite(ctx, data.PendingInvite{WorkspaceID: wsID, Email: "mp_invited@example.com", WorkspaceRole: "member"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeletePendingInvite(ctx, wsID, "mp_invited@example.com") })
	g, err := store.CreateGroup(ctx, wsID, "MP-group")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, g.ID) })
	if err := store.SetGroupMembers(ctx, g.ID, []string{activeID}); err != nil {
		t.Fatal(err)
	}
	app := applicationWithRoles(t, ws)
	if err := store.SetMemberAppRole(ctx, wsID, activeID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}

	body := getPage(t, h, cookie, "/workspace-members")
	for _, want := range []string{
		"mp_active@example.com", "mp_gone@example.com", "mp_invited@example.com",
		`href="/workspace-members/` + activeID + `/edit"`, ">Active<", ">Deactivated<", ">Invited<",
		"Workspace member", app.Name + ": " + app.Roles[0], "Groups: MP-group",
		"+ Invite member", `hx-post="/runtime-actions/invite_member"`, "Send invitation", `name="email"`, `name="app_role[` + app.ID + `]"`,
		`hx-post="/runtime-actions/revoke_invite?record=mp_invited%40example.com"`, ">Revoke<",
		`role="search"`, `name="q"`, "Search members", `href="/workspace-groups"`, `<nav aria-label="Workspace settings"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Members page does not contain %q", want)
		}
	}
	if n := strings.Count(body, `aria-current="page"`); n != 1 || !strings.Contains(body, `href="/workspace-members" aria-current="page"`) {
		t.Errorf("%d current settings rows, want exactly Members", n)
	}

	narrowed := getPage(t, h, cookie, "/workspace-members?q=MP_GONE")
	if !strings.Contains(narrowed, "mp_gone@example.com") || strings.Contains(narrowed, "mp_active@example.com") || strings.Contains(narrowed, "mp_invited@example.com") {
		t.Error("?q= did not narrow the members and the invitations to what matches")
	}
	if !strings.Contains(narrowed, `value="MP_GONE"`) {
		t.Error("the search box lost the words the request carried")
	}
	invitedOnly := getPage(t, h, cookie, "/workspace-members?q=invited")
	if !strings.Contains(invitedOnly, "mp_invited@example.com") || strings.Contains(invitedOnly, "mp_active@example.com") {
		t.Error("?q= did not reach the invitations")
	}

	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "mp_plain@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", member.ID) })
	if err := store.AddMember(ctx, wsID, member.ID, "mp_plain@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/workspace-members", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, config.Config{SessionSecret: "memberspage-secret"}, member.ID, 0)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "mp_active@example.com") {
		t.Errorf("a member opening the Members page = %d, want 403 and nobody's email", rec.Code)
	}
}
