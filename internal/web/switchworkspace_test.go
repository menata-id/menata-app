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

// sessionCookieValueForTest returns the raw cookie value authorization.SetSessionCookie would
// hand a real ResponseWriter, so a test can attach it to a *http.Request without going through a
// real login round trip.
func sessionCookieValueForTest(t *testing.T, cfg config.Config, subject string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	authorization.SetSessionCookie(rec, cfg.SessionSecret, subject, cfg.SecureCookies)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("SetSessionCookie produced no cookie")
	}
	return cookies[0].Value
}

// TestSwitchWorkspace covers the mid-session switcher end to end: an identity with two Workspace
// memberships sees both on showSwitchWorkspace, and submitSwitchWorkspace re-points the session
// cookie at the picked Workspace's own mch_user record -- the same re-check
// submitChooseWorkspace's login-time flow already does, just sourced from the active session
// instead of the pending-email cookie.
func TestSwitchWorkspace(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "switch-workspace-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "switch_flow_test@example.com"

	ws1, err := store.CreateWorkspace(ctx, "Switch Test Alpha", "switch-test-alpha")
	if err != nil {
		t.Fatalf("CreateWorkspace(alpha): %v", err)
	}
	cleanupAuthTest(t, pool, ws1.ID, email)
	user1, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws1.ID), "mch_user", map[string]any{"fld_name": "Switch One", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(user1): %v", err)
	}
	if err := store.AddMember(ctx, ws1.ID, user1.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember(alpha): %v", err)
	}

	ws2, err := store.CreateWorkspace(ctx, "Switch Test Beta", "switch-test-beta")
	if err != nil {
		t.Fatalf("CreateWorkspace(beta): %v", err)
	}
	cleanupAuthTest(t, pool, ws2.ID, email)
	user2, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws2.ID), "mch_user", map[string]any{"fld_name": "Switch Two", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(user2): %v", err)
	}
	if err := store.AddMember(ctx, ws2.ID, user2.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember(beta): %v", err)
	}

	// Signed into Workspace Alpha, showSwitchWorkspace should list both Workspaces (this
	// identity's own two memberships), not just the current one.
	req := httptest.NewRequest(http.MethodGet, "/switch-workspace", nil)
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws1.ID))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user1.ID)})
	rec := httptest.NewRecorder()
	showSwitchWorkspace(store, cfg)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("showSwitchWorkspace status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Switch Test Alpha") || !strings.Contains(body, "Switch Test Beta") {
		t.Errorf("showSwitchWorkspace body missing one of the two Workspace names; body=%s", body)
	}

	// Posting Beta's id, still while the session names Alpha, should re-point the session cookie
	// at Beta's own mch_user record -- not Alpha's, and not a tampered id outside this identity's
	// own memberships (that path is TestSwitchWorkspace_rejectsForeignWorkspace below).
	form := strings.NewReader("workspace_id=" + ws2.ID)
	req2 := httptest.NewRequest(http.MethodPost, "/switch-workspace", form)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2 = req2.WithContext(data.WithWorkspaceScope(req2.Context(), ws1.ID))
	req2.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user1.ID)})
	rec2 := httptest.NewRecorder()
	submitSwitchWorkspace(store, cfg)(rec2, req2)
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("submitSwitchWorkspace status = %d, want 303; body=%s", rec2.Code, rec2.Body.String())
	}
	if got := rec2.Header().Get("Location"); got != "/home" {
		t.Errorf("submitSwitchWorkspace redirected to %q, want /home", got)
	}
	newCookies := rec2.Result().Cookies()
	if len(newCookies) == 0 {
		t.Fatal("submitSwitchWorkspace set no session cookie")
	}
	verifyReq := &http.Request{Header: http.Header{}}
	verifyReq.AddCookie(newCookies[0])
	gotUserID, ok := authorization.CurrentUserID(verifyReq, cfg.SessionSecret)
	if !ok || gotUserID != user2.ID {
		t.Errorf("session after switch = (%q, %v), want (%q, true)", gotUserID, ok, user2.ID)
	}
}

// TestSwitchWorkspace_rejectsForeignWorkspace mirrors submitChooseWorkspace's own tamper check
// (a posted workspace_id this identity doesn't belong to must not be honored) for the mid-session
// path, since resolveWorkspaceMembership is shared between the two but exercised here from the
// session-cookie side rather than the pending-email side.
func TestSwitchWorkspace_rejectsForeignWorkspace(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "switch-workspace-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "switch_flow_reject_test@example.com"

	ws1, err := store.CreateWorkspace(ctx, "Reject Test Home", "reject-test-home")
	if err != nil {
		t.Fatalf("CreateWorkspace(home): %v", err)
	}
	cleanupAuthTest(t, pool, ws1.ID, email)
	user1, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws1.ID), "mch_user", map[string]any{"fld_name": "Reject One", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(user1): %v", err)
	}
	if err := store.AddMember(ctx, ws1.ID, user1.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember(home): %v", err)
	}

	// A Workspace this identity has no membership in at all.
	const otherEmail = "switch_flow_reject_other@example.com"
	wsOther, err := store.CreateWorkspace(ctx, "Reject Test Foreign", "reject-test-foreign")
	if err != nil {
		t.Fatalf("CreateWorkspace(foreign): %v", err)
	}
	cleanupAuthTest(t, pool, wsOther.ID, otherEmail)

	form := strings.NewReader("workspace_id=" + wsOther.ID)
	req := httptest.NewRequest(http.MethodPost, "/switch-workspace", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws1.ID))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user1.ID)})
	rec := httptest.NewRecorder()
	submitSwitchWorkspace(store, cfg)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("submitSwitchWorkspace(foreign workspace_id) status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}
