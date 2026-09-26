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

// noopHandler is the trivial "did the gate let this through" probe every test below wraps.
func noopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestBlockWritesToArchivedWorkspace_refusesNonGETOnArchived is Tahap 7's own gate, exercised
// directly (Flow 2 gap study): a write against an archived Workspace's own scope is refused, and
// the identical request against a live Workspace passes through untouched.
func TestBlockWritesToArchivedWorkspace_refusesNonGETOnArchived(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Block Writes Test", "block-writes-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "unused_block_writes_test@example.com")

	handler := blockWritesToArchivedWorkspace(store)(noopHandler())

	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/machines/mch_task/records", nil)
		req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws.ID))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("POST against a live workspace = %d, want 200 (not yet archived)", rec.Code)
	}

	if err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	if rec := post(); rec.Code != http.StatusForbidden {
		t.Errorf("POST against an archived workspace = %d, want 403", rec.Code)
	}

	// GET must still pass -- read-only means writes are refused, not the whole workspace.
	getReq := httptest.NewRequest(http.MethodGet, "/machines/mch_task", nil)
	getReq = getReq.WithContext(data.WithWorkspaceScope(getReq.Context(), ws.ID))
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Errorf("GET against an archived workspace = %d, want 200", getRec.Code)
	}
}

// TestBlockWritesToArchivedWorkspace_allowsAllowlist proves the named exceptions
// (archivedWriteAllowlist) stay reachable -- a member sitting inside an archived Workspace must
// still be able to sign out, switch to a different Workspace, create a new one, or restore this
// one.
func TestBlockWritesToArchivedWorkspace_allowsAllowlist(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Allowlist Test", "allowlist-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "unused_allowlist_test@example.com")
	if err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	handler := blockWritesToArchivedWorkspace(store)(noopHandler())

	for path := range archivedWriteAllowlist {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws.ID))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s against an archived workspace = %d, want 200 (allow-listed)", path, rec.Code)
		}
	}
}

// TestSubmitArchiveWorkspace covers the Danger Zone action end to end: it archives the ctx-scoped
// Workspace and redirects to Choose Workspace, not /home, since the Workspace the request was
// sitting in is now the one that just became read-only.
func TestSubmitArchiveWorkspace(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "submit-archive-test-secret", SecureCookies: false}
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Submit Archive Test", "submit-archive-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "unused_submit_archive_test@example.com")

	req := httptest.NewRequest(http.MethodPost, "/workspace-settings/archive", nil)
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws.ID))
	rec := httptest.NewRecorder()
	submitArchiveWorkspace(store, cfg)(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("submitArchiveWorkspace status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/choose-workspace" {
		t.Errorf("submitArchiveWorkspace redirected to %q, want /choose-workspace", got)
	}
	row, err := store.GetWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if !row.Archived {
		t.Error("submitArchiveWorkspace did not archive the workspace")
	}
}

// TestSubmitRestoreWorkspaceFromChoose_requiresAdmin is Restore's own authorization check: an
// identity with no admin membership on the target Workspace (whether a plain member, or not a
// member at all) is refused, and the Workspace stays archived either way.
func TestSubmitRestoreWorkspaceFromChoose_requiresAdmin(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "submit-restore-test-secret", SecureCookies: false}
	ctx := context.Background()
	const adminEmail = "submit_restore_admin_test@example.com"
	const memberEmail = "submit_restore_member_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Submit Restore Test", "submit-restore-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, adminEmail)
	cleanupAuthTest(t, pool, ws.ID, memberEmail)
	admin, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": "Restore Admin", "fld_email": adminEmail})
	if err != nil {
		t.Fatalf("CreateRecord(admin): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, admin.ID, adminEmail, "admin", ""); err != nil {
		t.Fatalf("AddMember(admin): %v", err)
	}
	member, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), "mch_user", map[string]any{"fld_name": "Restore Member", "fld_email": memberEmail})
	if err != nil {
		t.Fatalf("CreateRecord(member): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, member.ID, memberEmail, "member", ""); err != nil {
		t.Fatalf("AddMember(member): %v", err)
	}
	if err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	restoreAs := func(email string) *httptest.ResponseRecorder {
		form := strings.NewReader("workspace_id=" + ws.ID)
		req := httptest.NewRequest(http.MethodPost, "/choose-workspace/restore", form)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		authorization.SetPendingEmailCookie(rec, cfg.SessionSecret, email, cfg.SecureCookies)
		req.AddCookie(rec.Result().Cookies()[0])
		rec = httptest.NewRecorder()
		submitRestoreWorkspaceFromChoose(store, cfg)(rec, req)
		return rec
	}

	if rec := restoreAs(memberEmail); rec.Code != http.StatusForbidden {
		t.Errorf("submitRestoreWorkspaceFromChoose(plain member) status = %d, want 403", rec.Code)
	}
	if row, err := store.GetWorkspace(ctx, ws.ID); err != nil || !row.Archived {
		t.Fatalf("workspace after a refused restore: row=%+v err=%v, want still archived", row, err)
	}

	if rec := restoreAs(adminEmail); rec.Code != http.StatusSeeOther {
		t.Fatalf("submitRestoreWorkspaceFromChoose(admin) status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	row, err := store.GetWorkspace(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if row.Archived {
		t.Error("submitRestoreWorkspaceFromChoose(admin) did not restore the workspace")
	}
}
