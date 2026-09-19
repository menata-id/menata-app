package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
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
