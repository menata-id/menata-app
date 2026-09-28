package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// Behaviour coverage for the write routes TestPostRoutesRefuseUnauthenticatedAndUnCSRFed only holds
// structurally -- it refuses them without a session or a CSRF token, and nothing checked that they do
// anything at all.
//
// Each assertion here is about the *stored* consequence rather than the response, because a rendered
// screen is exactly what a silent no-op also produces. That is the shape that caught all three routes
// in writeroutes_test.go, and mutation-proving each one against a handler that renders and writes
// nothing is what makes the coverage worth its lines.
//
// Two routes are deliberately still uncovered and named rather than skipped: /new-application/message
// and /new-application/{session}/discard need a stored assistant conversation with generated metadata
// in it, the same fixture /new-application/{session}/review lacks in the per-record GET sweep. That is
// its own slice.

// --- account ---------------------------------------------------------------------------------------

func TestSubmitProfile_storesTheNameAndRefusesAnEmptyOne(t *testing.T) {
	s := newAccountRouteSetup(t, "profile")

	if rec := s.post(t, "/account-profile", submitProfile(s.store, s.cfg), url.Values{"name": {"Nana Isnawan"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("submit profile = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if got := s.credential(t).FullName; got != "Nana Isnawan" {
		t.Errorf("stored full name = %q, want %q -- the write did not land", got, "Nana Isnawan")
	}

	// An empty name re-renders the form rather than storing it: the interesting half is that the
	// previous value survives, which a handler that wrote first and validated second would lose.
	if rec := s.post(t, "/account-profile", submitProfile(s.store, s.cfg), url.Values{"name": {"   "}}); rec.Code != http.StatusOK {
		t.Errorf("submit an empty name = %d, want 200 with the form's own error", rec.Code)
	}
	if got := s.credential(t).FullName; got != "Nana Isnawan" {
		t.Errorf("stored full name = %q after a refused submit, want it untouched", got)
	}
}

func TestSubmitAccountNotifications_storesEachPreferenceIndependently(t *testing.T) {
	s := newAccountRouteSetup(t, "notifprefs")

	// All three on, then one off: a handler reading the wrong checkbox name would pass the first
	// assertion and fail the second, which is why both are here.
	if rec := s.post(t, "/account-notifications", submitAccountNotifications(s.store, s.cfg),
		url.Values{"notify_assigned": {"on"}, "notify_decided": {"on"}, "notify_sla_breach": {"on"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("submit preferences = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	cred := s.credential(t)
	if !cred.NotifyAssigned || !cred.NotifyDecided || !cred.NotifySLABreach {
		t.Fatalf("preferences = %v/%v/%v, want all true", cred.NotifyAssigned, cred.NotifyDecided, cred.NotifySLABreach)
	}

	if rec := s.post(t, "/account-notifications", submitAccountNotifications(s.store, s.cfg),
		url.Values{"notify_assigned": {"on"}, "notify_sla_breach": {"on"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("submit preferences again = %d, want a redirect", rec.Code)
	}
	cred = s.credential(t)
	if !cred.NotifyAssigned || cred.NotifyDecided || !cred.NotifySLABreach {
		t.Errorf("preferences = %v/%v/%v, want true/false/true -- an unchecked box must clear its own preference and no other",
			cred.NotifyAssigned, cred.NotifyDecided, cred.NotifySLABreach)
	}
}

func TestSubmitSignOutOtherDevices_invalidatesEveryOtherSession(t *testing.T) {
	s := newAccountRouteSetup(t, "signout")

	before, err := s.store.CurrentSessionGeneration(s.ctx, s.userRecordID)
	if err != nil {
		t.Fatalf("CurrentSessionGeneration: %v", err)
	}

	rec := s.post(t, "/account-security/sign-out-other-devices", submitSignOutOtherDevices(s.store, s.cfg), url.Values{})
	if rec.Code != http.StatusOK {
		t.Fatalf("sign out other devices = %d, want 200; body=%s", rec.Code, firstChars(rec.Body.String()))
	}

	after, err := s.store.CurrentSessionGeneration(s.ctx, s.userRecordID)
	if err != nil {
		t.Fatalf("CurrentSessionGeneration: %v", err)
	}
	if after <= before {
		t.Errorf("session generation %d -> %d: it must move, or every other device stays signed in", before, after)
	}
	// And this request's own session survives its own action: the handler re-issues the cookie, which
	// is the difference between "sign out other devices" and "sign out".
	if cookie := sessionCookieFrom(rec); cookie == "" {
		t.Error("no new session cookie was issued -- the acting session signed itself out too")
	}
}

func TestSubmitMarkAllNotificationsRead_marksOnlyThisViewers(t *testing.T) {
	s := newAccountRouteSetup(t, "markread")

	other, err := s.store.CreateRecord(s.ctx, domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateRecord(other): %v", err)
	}
	t.Cleanup(func() { _ = s.store.DeleteRecord(s.ctx, domain.UserMachineID, other.ID) })

	mine := s.notification(t, s.userRecordID)
	theirs := s.notification(t, other.ID)

	if rec := s.post(t, "/notifications/mark-all-read", submitMarkAllNotificationsRead(s.store, s.cfg), url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("mark all read = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if got := s.readState(t, mine); got != "read" {
		t.Errorf("my notification is %q, want read", got)
	}
	// The assertion that matters: a handler that marked every record of the Machine would pass the
	// line above and quietly read somebody else's inbox for them.
	if got := s.readState(t, theirs); got != "unread" {
		t.Errorf("another person's notification is %q, want unread -- this route is scoped to the viewer", got)
	}
}

// --- groups and invitations ------------------------------------------------------------------------

func TestSubmitGroupMembers_membershipLandsAndEffectiveRolesSeeIt(t *testing.T) {
	_, _, ctx, store, _, installed, actorID := routerSetupParts(t, "groupmembers")
	workspaceID, _ := data.WorkspaceScope(ctx)
	group := newTestGroup(t, ctx, store, workspaceID, "Members Group")

	app := applicationWithRoles(t, installed)
	if err := store.SetGroupAppRole(ctx, group.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatalf("SetGroupAppRole: %v", err)
	}

	form := url.Values{"member": {actorID}}
	rec := postAdminForm(t, store, ctx, installed, actorID, "/workspace-groups/{groupID}/members",
		"/workspace-groups/"+group.ID+"/members", submitGroupMembers(store), form)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("set group members = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}

	ids, err := store.GroupMemberIDs(ctx, group.ID)
	if err != nil {
		t.Fatalf("GroupMemberIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != actorID {
		t.Fatalf("group members = %v, want just the actor -- the write did not land", ids)
	}
	// The consequence that matters is authorization, not the row: a Group's role reaches its members
	// through data.EffectiveRoles, and that is what a silent no-op here would quietly withhold.
	members, err := store.ListMembers(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	granted := false
	for _, m := range members {
		if m.UserRecordID != actorID {
			continue
		}
		for _, role := range data.EffectiveRoles(m.AppRoles, m.Groups)[app.ID] {
			if role == app.Roles[0] {
				granted = true
			}
		}
	}
	if !granted {
		t.Errorf("the actor does not hold %q in %s through the Group -- membership landed but grants nothing", app.Roles[0], app.ID)
	}
}

func TestSubmitDeleteGroup_removesTheGroupAndItsGrants(t *testing.T) {
	_, _, ctx, store, _, installed, actorID := routerSetupParts(t, "groupdelete")
	workspaceID, _ := data.WorkspaceScope(ctx)
	group := newTestGroup(t, ctx, store, workspaceID, "Doomed Group")

	app := applicationWithRoles(t, installed)
	if err := store.SetGroupAppRole(ctx, group.ID, app.ID, app.Roles[0]); err != nil {
		t.Fatalf("SetGroupAppRole: %v", err)
	}

	rec := postAdminForm(t, store, ctx, installed, actorID, "/workspace-groups/{groupID}/delete",
		"/workspace-groups/"+group.ID+"/delete", submitDeleteGroup(store), url.Values{})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("delete group = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}

	groups, err := store.ListGroups(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	for _, g := range groups {
		if g.ID == group.ID {
			t.Fatalf("group %s is still listed after delete", group.ID)
		}
	}
	// A deleted Group whose grant row survived would be a role nothing can see and nobody can revoke.
	if _, err := store.GetGroup(ctx, workspaceID, group.ID); err == nil {
		t.Error("GetGroup still finds the deleted Group")
	}
}

func TestSubmitRevokeInvite_removesThePendingInvitation(t *testing.T) {
	_, _, ctx, store, _, installed, actorID := routerSetupParts(t, "revokeinvite")
	workspaceID, _ := data.WorkspaceScope(ctx)

	const invited = "revoke_invite_target@example.com"
	if err := store.CreatePendingInvite(ctx, data.PendingInvite{WorkspaceID: workspaceID, Email: invited, WorkspaceRole: "member"}); err != nil {
		t.Fatalf("CreatePendingInvite: %v", err)
	}
	if len(pendingInviteEmails(t, ctx, store, workspaceID)) != 1 {
		t.Fatal("the invitation was not created -- the fixture never reached the state under test")
	}

	rec := postAdminForm(t, store, ctx, installed, actorID, "/workspace-members/revoke-invite",
		"/workspace-members/revoke-invite", submitRevokeInvite(store), url.Values{"email": {invited}})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("revoke invite = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if emails := pendingInviteEmails(t, ctx, store, workspaceID); len(emails) != 0 {
		t.Errorf("pending invites = %v after revoking, want none", emails)
	}

	// An empty email is refused rather than deleting nothing silently -- the difference between "that
	// invitation is gone" and "the form sent nothing and the screen said it worked".
	if rec := postAdminForm(t, store, ctx, installed, actorID, "/workspace-members/revoke-invite",
		"/workspace-members/revoke-invite", submitRevokeInvite(store), url.Values{}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("revoke with no email = %d, want 422", rec.Code)
	}
}

// --- helpers ---------------------------------------------------------------------------------------

// accountRouteSetup is one authenticated identity with a credential, which the account routes need
// because every one of them writes against the credential rather than a record.
type accountRouteSetup struct {
	store        *data.Store
	cfg          config.Config
	ctx          context.Context
	installed    domain.Workspace
	workspaceID  string
	userRecordID string
	email        string
}

func newAccountRouteSetup(t *testing.T, name string) *accountRouteSetup {
	t.Helper()
	_, _, ctx, store, _, installed, _ := routerSetupParts(t, name)
	workspaceID, _ := data.WorkspaceScope(ctx)

	email := name + "_account_route@example.com"
	record, err := store.CreateRecord(ctx, domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateRecord(member): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, domain.UserMachineID, record.ID) })
	if err := store.AddMember(ctx, workspaceID, record.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	deleteCredentialForTest(t, ctx, email)
	if err := store.CreateCredential(ctx, email, "Before", hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() { deleteCredentialForTest(t, ctx, email) })

	return &accountRouteSetup{store: store, cfg: config.Config{SessionSecret: name + "-account-secret"},
		ctx: ctx, installed: installed, workspaceID: workspaceID, userRecordID: record.ID, email: email}
}

func (s *accountRouteSetup) post(t *testing.T, path string, h http.HandlerFunc, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), s.installed, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, s.userRecordID, 0)})

	r := chi.NewRouter()
	r.Post(path, h)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func (s *accountRouteSetup) credential(t *testing.T) *data.Credential {
	t.Helper()
	cred, err := s.store.GetCredential(s.ctx, s.email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	return cred
}

func (s *accountRouteSetup) notification(t *testing.T, recipient string) string {
	t.Helper()
	rec, err := s.store.CreateRecord(s.ctx, "mch_notification", map[string]any{
		"fld_recipient": recipient, "fld_message": "something happened", "fld_read": "unread",
	})
	if err != nil {
		t.Fatalf("CreateRecord(notification): %v", err)
	}
	t.Cleanup(func() { _ = s.store.DeleteRecord(s.ctx, "mch_notification", rec.ID) })
	return rec.ID
}

func (s *accountRouteSetup) readState(t *testing.T, id string) string {
	t.Helper()
	rec, err := s.store.GetRecord(s.ctx, "mch_notification", id)
	if err != nil {
		t.Fatalf("GetRecord(notification): %v", err)
	}
	return toDisplayString(rec.Values["fld_read"])
}

func sessionCookieFrom(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == authorization.SessionCookieName && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

func newTestGroup(t *testing.T, ctx context.Context, store *data.Store, workspaceID, name string) *data.Group {
	t.Helper()
	group, err := store.CreateGroup(ctx, workspaceID, name)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, workspaceID, group.ID) })
	return group
}

func pendingInviteEmails(t *testing.T, ctx context.Context, store *data.Store, workspaceID string) []string {
	t.Helper()
	invites, err := store.ListPendingInvites(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	var out []string
	for _, inv := range invites {
		out = append(out, inv.Email)
	}
	return out
}

// postAdminForm posts one admin route with its chi pattern mounted, so a {groupID} in the path is
// bound the way the real router binds it.
func postAdminForm(t *testing.T, store *data.Store, ctx context.Context, ws domain.Workspace, actorID, pattern, path string, h http.HandlerFunc, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	workspaceID, _ := data.WorkspaceScope(ctx)
	cfg := config.Config{SessionSecret: "admin-form-secret"}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), workspaceID), ws, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Post(pattern, h)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// --- workspace lifecycle ---------------------------------------------------------------------------

// TestSubmitCreateWorkspace_createsTheWorkspaceWithThisPersonAsAdmin covers the route that brings a
// whole Workspace into being. The stored consequence is threefold and all three matter: the Workspace
// row, the creator's own mch_user record *inside* it, and their admin membership -- a handler that made
// the first two and skipped the third would produce a Workspace nobody can administer.
func TestSubmitCreateWorkspace_createsTheWorkspaceWithThisPersonAsAdmin(t *testing.T) {
	s := newAccountRouteSetup(t, "createws")

	form := url.Values{"workspace_name": {"Created By Test"}}
	req := httptest.NewRequest(http.MethodPost, "/create-workspace", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), s.installed, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, s.userRecordID, 0)})

	userMachine := realUserMachine(t)
	r := chi.NewRouter()
	r.Post("/create-workspace", submitCreateWorkspace(userMachine, s.store, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("create workspace = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}

	created := workspaceBySlug(t, s.ctx, s.store, "created-by-test")
	if created == "" {
		t.Fatal("no Workspace with that slug exists -- the create did not land")
	}
	t.Cleanup(func() { cleanupWorkspaceRows(t, created) })

	memberships, err := s.store.ListMemberships(s.ctx, s.email)
	if err != nil {
		t.Fatalf("ListMemberships: %v", err)
	}
	admin := false
	for _, m := range memberships {
		if m.WorkspaceID == created && m.WorkspaceRole == "admin" {
			admin = true
		}
	}
	if !admin {
		t.Error("the creator is not an admin of the Workspace they just created -- nobody can administer it")
	}
}

// TestSubmitRestoreWorkspaceFromSwitch_refusesAWorkspaceYouDoNotAdminister is the half of the restore
// route worth pinning: the refusal. Restoring is an admin action on *that* Workspace, and the route
// takes the Workspace id straight from the form -- so the guard is the whole security of it.
func TestSubmitRestoreWorkspaceFromSwitch_refusesAWorkspaceYouDoNotAdminister(t *testing.T) {
	s := newAccountRouteSetup(t, "restorews")

	// A Workspace this identity has no membership in at all.
	other, err := s.store.CreateWorkspace(s.ctx, "Not Mine", "not-mine-restore-test")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	t.Cleanup(func() { cleanupWorkspaceRows(t, other.ID) })

	rec := s.post(t, "/switch-workspace/restore", submitRestoreWorkspaceFromSwitch(s.store, s.cfg),
		url.Values{"workspace_id": {other.ID}})
	if rec.Code != http.StatusForbidden {
		t.Errorf("restore a Workspace you do not administer = %d, want 403; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
}

func realUserMachine(t *testing.T) *domain.Machine {
	t.Helper()
	machines, _ := loadRealMachines(t)
	m, ok := machines[domain.UserMachineID]
	if !ok {
		t.Fatalf("the default Workspace installs no %s", domain.UserMachineID)
	}
	return m
}

func workspaceBySlug(t *testing.T, ctx context.Context, store *data.Store, slug string) string {
	t.Helper()
	pool := tracedTestPool(t)
	var id string
	if err := pool.QueryRow(ctx, `SELECT id FROM workspaces WHERE slug = $1`, slug).Scan(&id); err != nil {
		return ""
	}
	return id
}

// cleanupWorkspaceRows removes a Workspace this test created, in foreign-key order -- the same sweep
// cleanupAuthTest performs, narrowed to what these two tests make.
func cleanupWorkspaceRows(t *testing.T, workspaceID string) {
	t.Helper()
	pool := tracedTestPool(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM workspace_members WHERE workspace_id = $1`,
		`DELETE FROM session_generations WHERE subject IN (SELECT id FROM records WHERE workspace_id = $1)`,
		`DELETE FROM records WHERE workspace_id = $1`,
		`DELETE FROM workspaces WHERE id = $1`,
	} {
		if _, err := pool.Exec(ctx, stmt, workspaceID); err != nil {
			t.Errorf("cleanup %s: %v", workspaceID, err)
		}
	}
}
