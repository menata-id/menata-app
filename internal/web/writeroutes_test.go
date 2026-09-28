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

// The two other write routes the POST sweep found named by no test, and the two where a silent no-op
// matters most after revise: granting an Application role to a Group, and changing a password.
//
// TestPostRoutesRefuseUnauthenticatedAndUnCSRFed covers every POST structurally -- refuses without a
// session, refuses without a CSRF token. That is not behaviour coverage, and these two are what
// behaviour coverage looks like where getting it wrong is an authorization or credential failure
// rather than a blank screen.

// TestSubmitGroupRoles_grantsTheRoleAndRefusesAnUndeclaredOne is about a write that changes who can do
// what: a Group's grant reaches every member of that Group (data.EffectiveRoles merges both paths), so
// a grant that silently does nothing is an authorization failure behind a screen that looks fine.
func TestSubmitGroupRoles_grantsTheRoleAndRefusesAnUndeclaredOne(t *testing.T) {
	_, _, ctx, store, _, installed, actorID := routerSetupParts(t, "grouproles")
	workspaceID, _ := data.WorkspaceScope(ctx)

	group, err := store.CreateGroup(ctx, workspaceID, "Roles Group")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteGroup(ctx, workspaceID, group.ID) })

	// The role comes from the Application's own declared vocabulary rather than a literal here -- the
	// same reason submittedAppRoles validates against it instead of against a list in Go.
	app := applicationWithRoles(t, installed)
	role := app.Roles[0]

	rec := postGroupRoles(t, store, ctx, installed, actorID, group.ID, map[string]string{appRoleField(app.ID): role})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("grant %q = %d, want a redirect; body=%s", role, rec.Code, firstChars(rec.Body.String()))
	}
	if got := grantedGroupRole(t, ctx, store, workspaceID, group.ID, app.ID); got != role {
		t.Errorf("group holds %q for %s, want %q -- the grant did not land", got, app.ID, role)
	}

	// A role the Application does not declare is refused, and the grant already there is untouched --
	// the arm that stops a tampered form inventing a role nothing gates on.
	rec = postGroupRoles(t, store, ctx, installed, actorID, group.ID, map[string]string{appRoleField(app.ID): "not-a-declared-role"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("grant of an undeclared role = %d, want 422; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if got := grantedGroupRole(t, ctx, store, workspaceID, group.ID, app.ID); got != role {
		t.Errorf("group holds %q after a refused grant, want it untouched at %q", got, role)
	}
}

func applicationWithRoles(t *testing.T, ws domain.Workspace) domain.Application {
	t.Helper()
	for _, app := range ws.Applications {
		if len(app.Roles) > 0 {
			return app
		}
	}
	t.Fatal("no installed Application declares a role vocabulary -- this test has nothing to grant")
	return domain.Application{}
}

func grantedGroupRole(t *testing.T, ctx context.Context, store *data.Store, workspaceID, groupID, appID string) string {
	t.Helper()
	groups, err := store.ListGroups(ctx, workspaceID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	for _, g := range groups {
		if g.ID == groupID {
			return g.Grants[appID]
		}
	}
	t.Fatalf("group %s is gone", groupID)
	return ""
}

func postGroupRoles(t *testing.T, store *data.Store, ctx context.Context, ws domain.Workspace, actorID, groupID string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	cfg := config.Config{SessionSecret: "grouproles-secret"}
	req := httptest.NewRequest(http.MethodPost, "/workspace-groups/"+groupID+"/roles", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), workspaceID), ws, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Post("/workspace-groups/{groupID}/roles", submitGroupRoles(store))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestSubmitChangePassword_theOldPasswordStopsWorking is the only assertion that means anything about
// a password change: not that the screen rendered, but that authentication now takes the new password
// and refuses the old one.
func TestSubmitChangePassword_theOldPasswordStopsWorking(t *testing.T) {
	const (
		email = "change_password_test@example.com"
		old   = "the-old-password"
		fresh = "a-brand-new-password"
	)
	_, _, ctx, store, _, installed, _ := routerSetupParts(t, "changepassword")
	workspaceID, _ := data.WorkspaceScope(ctx)

	// A second identity with a known password, rather than the fixture's own member: this test has to
	// know the password before the change to have anything to assert after it.
	record, err := store.CreateRecord(ctx, domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateRecord(member): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, domain.UserMachineID, record.ID) })
	if err := store.AddMember(ctx, workspaceID, record.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	hash, err := authorization.HashPassword(old)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// The credential is keyed by email, not by Workspace, so it outlives this test's own Workspace
	// cleanup and a second run collides on the primary key. Found by mutation-testing this very test:
	// the mutation failed on the duplicate key rather than on the assertion, which would have read as
	// "the mutation was caught" and proved nothing.
	deleteCredentialForTest(t, ctx, email)
	if err := store.CreateCredential(ctx, email, "Test Person", hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() { deleteCredentialForTest(t, ctx, email) })
	if outcome := authenticateMember(ctx, store, email, old); outcome != loginOK {
		t.Fatalf("the old password does not work *before* the change (%v) -- the fixture never reached the state under test", outcome)
	}

	cfg := config.Config{SessionSecret: "changepassword-secret"}
	form := url.Values{"current_password": {old}, "new_password": {fresh}, "confirm_password": {fresh}}
	req := httptest.NewRequest(http.MethodPost, "/account-security/change-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), workspaceID), installed, "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, record.ID, 0)})

	r := chi.NewRouter()
	r.Post("/account-security/change-password", submitChangePassword(store, cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("change password = %d, want 200; body=%s", rec.Code, firstChars(rec.Body.String()))
	}

	if outcome := authenticateMember(ctx, store, email, fresh); outcome != loginOK {
		t.Errorf("the new password does not authenticate (%v) -- the change did not land", outcome)
	}
	if outcome := authenticateMember(ctx, store, email, old); outcome == loginOK {
		t.Error("the old password still authenticates -- the screen rendered but nothing was written")
	}
}

func firstChars(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// deleteCredentialForTest removes one credential by email -- the sweep cleanupAuthTest performs for
// the tests that hold a pool. Its own pool handle rather than a method on data.Store: a delete that
// exists only for tests does not belong in production code.
func deleteCredentialForTest(t *testing.T, ctx context.Context, email string) {
	t.Helper()
	pool := tracedTestPool(t)
	if _, err := pool.Exec(ctx, `DELETE FROM credentials WHERE email = $1`, email); err != nil {
		t.Fatalf("delete credential %s: %v", email, err)
	}
}
