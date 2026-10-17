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
	"menata.app/internal/domain"
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

// TestMemberIsARuntimePageForOneMember: /workspace-members/<id>/edit draws that member's own screen -- who they are and whether
// they are active, a Workspace role select and a role select per Application starting as what they hold, Deactivate for an
// active member and Reactivate for a deactivated one (never both), and what they can do once their Groups are counted;
// an id the source does not hold is a 404.
func TestMemberIsARuntimePageForOneMember(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "memberpage", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	mk := func(email string) string {
		u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", u.ID) })
		if err := store.AddMember(ctx, wsID, u.ID, email, "member", ""); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	activeID, goneID := mk("mpg_active@example.com"), mk("mpg_gone@example.com")
	if err := store.DeactivateMember(ctx, wsID, goneID); err != nil {
		t.Fatal(err)
	}
	app := applicationWithRoles(t, ws)
	if err := store.SetMemberAppRole(ctx, wsID, activeID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}
	g, err := store.CreateGroup(ctx, wsID, "MPG-group")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, g.ID) })
	if err := store.SetGroupMembers(ctx, g.ID, []string{activeID}); err != nil {
		t.Fatal(err)
	}

	body := getPage(t, h, cookie, "/workspace-members/"+activeID+"/edit")
	for _, want := range []string{
		"mpg_active@example.com", ">Active<", `href="/workspace-members"`,
		`hx-post="/runtime-actions/set_member_access?record=` + activeID + `"`, `name="workspace_role"`, `<option value="member" selected`,
		`name="app_role[` + app.ID + `]"`, `<option value="` + app.Roles[0] + `" selected`, "Save changes",
		`hx-post="/runtime-actions/deactivate_member?record=` + activeID + `"`, "hx-confirm=", "Deactivate member",
		"Effective access", app.Name + " · " + app.Roles[0], "+ MPG-group group",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Member page does not contain %q", want)
		}
	}
	if strings.Contains(body, "Reactivate member") {
		t.Error("an active member is offered Reactivate")
	}
	if n := strings.Count(body, `aria-current="page"`); n != 1 || !strings.Contains(body, `href="/workspace-members" aria-current="page"`) {
		t.Errorf("%d current settings rows, want exactly Members", n)
	}
	if strings.Count(body, "<h1") != 1 {
		t.Errorf("%d headings, want the member's own", strings.Count(body, "<h1"))
	}

	gone := getPage(t, h, cookie, "/workspace-members/"+goneID+"/edit")
	if !strings.Contains(gone, ">Deactivated<") || !strings.Contains(gone, `hx-post="/runtime-actions/reactivate_member?record=`+goneID+`"`) || strings.Contains(gone, "Deactivate member") {
		t.Error("a deactivated member is not offered Reactivate and only Reactivate")
	}
	req := httptest.NewRequest(http.MethodGet, "/workspace-members/usr_nobody/edit", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unknown member = %d, want 404", rec.Code)
	}
}

// TestWorkspaceSettingsIsARuntimePage: /workspace-settings is the runtime's declared hub. Its rows are the settings sections
// (Groups among them, which the hand-written hub never listed), the sidebar-only Danger zone row is not one of them, the
// Installation and Danger zone sections carry the anchors the sidebar links to, and reload, archive and a saved installation's
// restore are the runtime's named actions; an admin opens it and a member does not.
func TestWorkspaceSettingsIsARuntimePage(t *testing.T) {
	h, cookie, ctx, store, _, _, _ := routerSetupFor(t, "settingshub", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	body := getPage(t, h, cookie, "/workspace-settings")
	for _, want := range []string{
		`<nav aria-label="Workspace settings"`, `href="/workspace-groups"`, `href="/workspace-members"`, "Invitations", "Not built yet",
		`id="installation"`, `id="danger-zone"`, `hx-post="/runtime-actions/reload_workspace"`, "Reload from disk",
		`href="/workspace-settings/files"`, "Saved installations", "No saved installations yet.",
		`hx-post="/runtime-actions/archive_workspace"`, "Archive workspace", "hx-confirm=", "Transfer ownership",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the settings hub does not contain %q", want)
		}
	}
	if strings.Contains(body, `href="/workspace-settings#danger-zone"`) {
		t.Error("the page variant draws the sidebar-only Danger zone row")
	}
	if strings.Contains(body, `aria-current="page"`) {
		t.Error("the hub marks a settings row current; it is the list, not one of the rows")
	}

	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "settingshub_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", member.ID) })
	if err := store.AddMember(ctx, wsID, member.ID, "settingshub_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/workspace-settings", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, config.Config{SessionSecret: "settingshub-secret"}, member.ID, 0)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member opening the settings hub = %d, want 403", rec.Code)
	}
}

// TestApplicationMembersNamesItsApplicationByItsHubAndRefusesWhatIsNone: /settings/<hub>/members is one runtime page for every
// Application, the one whose settings hub the route names; a hub that is no Application's is a 404, and an Application that
// declares no roles has none to assign.
func TestApplicationMembersNamesItsApplicationByItsHubAndRefusesWhatIsNone(t *testing.T) {
	h, cookie, _, _, _, ws, _ := routerSetupFor(t, "appmembers404", "default")
	var withRoles, withoutRoles string
	for _, app := range ws.Applications {
		for _, item := range app.AllNavigation {
			if !item.SettingsHub {
				continue
			}
			if len(app.Roles) > 0 && withRoles == "" {
				withRoles = item.Route
			}
			if len(app.Roles) == 0 && withoutRoles == "" {
				withoutRoles = item.Route
			}
		}
	}
	if withRoles == "" {
		t.Fatal("the fixture installs no Application with roles and a hub")
	}
	get := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := get(withRoles + "/members"); code != http.StatusOK {
		t.Errorf("an Application with roles = %d, want 200", code)
	}
	if code := get("/settings/nav_nobody/members"); code != http.StatusNotFound {
		t.Errorf("a hub nobody declares = %d, want 404", code)
	}
	if withoutRoles != "" {
		if code := get(withoutRoles + "/members"); code != http.StatusNotFound {
			t.Errorf("an Application with no roles = %d, want 404", code)
		}
	}
}

// A deactivated member holds a role but cannot act on it, so coverage counts active members only, and the row says so.
func TestApplicationMembersCoverageCountsActiveMembersOnly(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "appmemberscover", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	app := applicationWithRoles(t, ws)
	var hub domain.NavigationItem
	for _, item := range app.AllNavigation {
		if item.SettingsHub {
			hub = item
		}
	}
	if hub.ID == "" {
		t.Skip("the Application has no settings hub")
	}
	u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "amc_gone@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", u.ID) })
	if err := store.AddMember(ctx, wsID, u.ID, "amc_gone@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberAppRole(ctx, wsID, u.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.DeactivateMember(ctx, wsID, u.ID); err != nil {
		t.Fatal(err)
	}
	body := getPage(t, h, cookie, hub.Route+"/members")
	if !strings.Contains(body, "amc_gone@example.com") || !strings.Contains(body, "Deactivated") {
		t.Error("a deactivated member is not listed as such")
	}
	if strings.Contains(body, "2 of 2 active") || !strings.Contains(body, "of 1 active member have access to "+app.Name) {
		t.Errorf("coverage counted the deactivated member: %s", firstChars(body))
	}
}

// A role a Group adds is shown beside the member's own and is not editable on this screen: a person who holds a role both ways is
// shown it once, as their direct role.
func TestApplicationMembersShowsWhatAGroupAddsAndNotWhatTheMemberAlreadyHolds(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "appmembersgroup", "default")
	wsID, _ := data.WorkspaceScope(ctx)
	app := applicationWithRoles(t, ws)
	var hub domain.NavigationItem
	for _, item := range app.AllNavigation {
		if item.SettingsHub {
			hub = item
		}
	}
	if hub.ID == "" || len(app.Roles) < 2 {
		t.Skip("needs an Application with a settings hub and two roles")
	}
	u, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "amg_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", u.ID) })
	if err := store.AddMember(ctx, wsID, u.ID, "amg_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	g, err := store.CreateGroup(ctx, wsID, "AMG-group")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, wsID, g.ID) })
	if err := store.SetGroupMembers(ctx, g.ID, []string{u.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberAppRole(ctx, wsID, u.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.SetGroupAppRole(ctx, g.ID, app.ID, app.Roles[1]); err != nil {
		t.Fatal(err)
	}
	body := getPage(t, h, cookie, hub.Route+"/members")
	if !strings.Contains(body, "+ "+app.Roles[1]+" through a Group") {
		t.Error("a role a Group adds is not shown")
	}
	if strings.Contains(body, "+ "+app.Roles[0]+" through a Group") {
		t.Error("the member's own role is shown again as a Group's")
	}
	if err := store.SetGroupAppRole(ctx, g.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatal(err)
	}
	if body := getPage(t, h, cookie, hub.Route+"/members"); strings.Contains(body, "through a Group") {
		t.Error("a role held both directly and through a Group is shown as the Group's too")
	}
}
