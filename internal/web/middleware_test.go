package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func passOrFail(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestRequireAuth_acceptsCurrentGeneration is the baseline for the M2 revocation tests below: a
// freshly issued cookie (generation 0, nothing ever bumped) must pass through normally.
func TestRequireAuth_acceptsCurrentGeneration(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	const email = "require_auth_fresh_test@example.com"
	userID := newTestMember(t, pool, store, "Require Auth Fresh Test", "require-auth-fresh-test-workspace", email)
	cfg := config.Config{SessionSecret: "require-auth-test-secret", SecureCookies: false}

	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, userID, 0)})
	rec := httptest.NewRecorder()
	requireAuth(store, "", cfg)(passOrFail(t)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("requireAuth(fresh cookie) status = %d, want 200", rec.Code)
	}
}

// TestRequireAuth_rejectsGenerationMismatch is the core regression test for security audit
// 2026-09-19's M2: once a subject's generation is bumped (logout, password reset), a cookie
// issued under the old generation must be rejected, not merely a cookie with a bad signature.
func TestRequireAuth_rejectsGenerationMismatch(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "require_auth_stale_test@example.com"
	userID := newTestMember(t, pool, store, "Require Auth Stale Test", "require-auth-stale-test-workspace", email)
	cfg := config.Config{SessionSecret: "require-auth-test-secret", SecureCookies: false}

	staleCookie := sessionCookieValueForTest(t, cfg, userID, 0)
	if err := store.BumpSessionGeneration(ctx, userID); err != nil {
		t.Fatalf("BumpSessionGeneration: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: staleCookie})
	rec := httptest.NewRecorder()
	requireAuth(store, "", cfg)(passOrFail(t)).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("requireAuth let a stale-generation cookie through, want it rejected")
	}

	// A freshly issued cookie, using the now-current generation, must work again.
	freshCookie := sessionCookieValueForTest(t, cfg, userID, 1)
	req2 := httptest.NewRequest(http.MethodGet, "/home", nil)
	req2.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: freshCookie})
	rec2 := httptest.NewRecorder()
	requireAuth(store, "", cfg)(passOrFail(t)).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("requireAuth(cookie at the current generation) status = %d, want 200", rec2.Code)
	}
}

// TestLogout_bumpsGenerationAndClearsCookie covers logout's own half of M2: the session it clears
// must not still work if a copy of its cookie exists elsewhere.
func TestLogout_bumpsGenerationAndClearsCookie(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "logout_bump_test@example.com"
	userID := newTestMember(t, pool, store, "Logout Bump Test", "logout-bump-test-workspace", email)
	cfg := config.Config{SessionSecret: "logout-test-secret", SecureCookies: false}

	cookieValue := sessionCookieValueForTest(t, cfg, userID, 0)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookieValue})
	rec := httptest.NewRecorder()
	logout(store, cfg)(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want 303", rec.Code)
	}

	gen, err := store.CurrentSessionGeneration(ctx, userID)
	if err != nil {
		t.Fatalf("CurrentSessionGeneration: %v", err)
	}
	if gen == 0 {
		t.Error("session generation still 0 after logout, want it bumped")
	}

	// A copy of the now-logged-out cookie, presented to requireAuth afterward, must be rejected.
	authedReq := httptest.NewRequest(http.MethodGet, "/home", nil)
	authedReq.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookieValue})
	authedRec := httptest.NewRecorder()
	requireAuth(store, "", cfg)(passOrFail(t)).ServeHTTP(authedRec, authedReq)
	if authedRec.Code == http.StatusOK {
		t.Error("requireAuth let a post-logout cookie copy through, want it rejected")
	}
}

// TestRequireApplicationAccess is the owner's rule of 2026-09-21 at its chokepoint: *someone who
// is not a member of an application cannot do anything in it* -- not even look.
//
// It exercises the middleware's own pure decision through the real router-shaped chain
// (currentApplication resolves the Application, this one reads it), because what it guards is
// completeness: the value of gating here rather than in each handler is that /dashboard and
// /approval-inbox -- the two screens that read across everyone's documents -- cannot be forgotten.
func TestRequireApplicationAccess(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "App Access", "app-access-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "app_access@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	member, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Rina", "fld_email": "app_access@example.com"})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}
	if err := store.AddMember(wsCtx, ws.ID, member.ID, "app_access@example.com", "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	workspace := domain.Workspace{
		Slug: ws.Slug,
		Applications: []domain.Application{
			// Declares roles, so entry requires one.
			{ID: "app_document_approval", Name: "Document Approval", Machines: []string{"mch_document"}, Roles: []string{"approver", "submitter", "reviewer"}},
			// Declares none: nobody can hold a role there, so requiring one would lock out
			// everyone -- it stays open.
			{ID: "app_project_management", Name: "Project Management", Machines: []string{"mch_task"}},
		},
	}
	cfg := config.Config{SessionSecret: "test-secret-for-app-access"}

	get := func(path string) int {
		r := chi.NewRouter()
		// The Workspace now arrives on ctx (set by currentWorkspace in the real router), so the
		// test supplies it the same way rather than handing it to the middleware directly.
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(rendering.WithCurrentWorkspace(req.Context(), workspace, "Test Workspace", false)))
			})
		})
		r.Use(currentApplication())
		r.Use(requireApplicationAccess(store, cfg))
		r.Get("/machines/{machineID}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		r.Get("/home", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, member.ID, 0)})
		req = req.WithContext(wsCtx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	// No role in Document Approval yet: every one of its routes is closed, reading included.
	if got := get("/machines/mch_document"); got != http.StatusForbidden {
		t.Errorf("no role: status = %d, want 403 -- a non-member cannot even look", got)
	}
	// A Workspace-level route is not inside any Application, so it is untouched.
	if got := get("/home"); got != http.StatusOK {
		t.Errorf("workspace route: status = %d, want 200 -- it belongs to no application", got)
	}
	// An Application declaring no roles gates on nothing.
	if got := get("/machines/mch_task"); got != http.StatusOK {
		t.Errorf("role-less application: status = %d, want 200 -- requiring a role nobody can hold would deny everyone", got)
	}

	// The weakest role is enough to get in, which is the point of the rule: reviewer grants
	// nothing else anywhere, and still opens every screen.
	if err := store.SetMemberAppRole(wsCtx, ws.ID, member.ID, "app_document_approval", "reviewer"); err != nil {
		t.Fatalf("SetMemberAppRole: %v", err)
	}
	if got := get("/machines/mch_document"); got != http.StatusOK {
		t.Errorf("reviewer: status = %d, want 200 -- a reviewer may look", got)
	}
}
