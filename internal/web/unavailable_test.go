package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"menata.app/internal/data"
)

// TestBlockUnavailableWorkspace is K09's request-side half: a Workspace whose manifest failed to load answers
// 503 on its screens, another Workspace is untouched, and the routes that leave the Workspace still work.
func TestBlockUnavailableWorkspace(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	broken, err := store.CreateWorkspace(ctx, "Unavailable Test", "unavailable-test-broken")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, broken.ID, "unused_unavailable_test@example.com")
	fine, err := store.CreateWorkspace(ctx, "Available Test", "unavailable-test-fine")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, fine.ID, "unused_available_test@example.com")

	handler := blockUnavailableWorkspace(store, map[string]bool{broken.Slug: true})(noopHandler())
	get := func(workspaceID, path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(data.WithWorkspaceScope(req.Context(), workspaceID))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := get(broken.ID, "/dashboard"); code != http.StatusServiceUnavailable {
		t.Errorf("a screen in the unavailable Workspace = %d, want 503", code)
	}
	if code := get(fine.ID, "/dashboard"); code != http.StatusOK {
		t.Errorf("a screen in another Workspace = %d, want 200 -- one broken Workspace stopped a healthy one", code)
	}
	for _, path := range []string{"/switch-workspace", "/account-profile", "/logout", "/api/account-menu/workspaces", "/create-workspace"} {
		if code := get(broken.ID, path); code != http.StatusOK {
			t.Errorf("%s in the unavailable Workspace = %d, want 200 -- a member must be able to leave", path, code)
		}
	}
}
