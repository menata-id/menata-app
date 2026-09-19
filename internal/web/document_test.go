package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/data"
	"menata.app/internal/storage"
)

// newUploadOwnedByRecord saves content under machineID/fieldID (storage.Store.Save's own key
// shape) and creates a matching record in workspaceID naming that key in fieldID -- the shared
// setup every serveUpload ownership test below needs. Uses CreateRecord directly (no
// data.ValidateRecord/Machine involved) since serveUpload's own ownership check
// (recordOwnsUpload) only ever looks at the stored data, the same way the real write path leaves
// it, not at Machine-level validation.
func newUploadOwnedByRecord(t *testing.T, ctx context.Context, store *data.Store, files *storage.Store, workspaceID, machineID, fieldID, filename string, content string) string {
	t.Helper()
	key, err := files.Save(machineID, fieldID, filename, strings.NewReader(content))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := store.CreateRecord(data.WithWorkspaceScope(ctx, workspaceID), machineID, map[string]any{fieldID: key}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	return key
}

// TestServeUpload_forcesAttachmentForNonSafeContent is the H2 regression test for serveUpload
// itself (security audit 2026-09-19): even a file that somehow made it onto disk with HTML
// content (e.g. stored before rejectDangerousUpload existed) must be served as a download, never
// rendered inline, regardless of what extension its stored key carries.
func TestServeUpload_forcesAttachmentForNonSafeContent(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "serve_upload_html_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Serve Upload HTML Test", "serve-upload-html-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	key := newUploadOwnedByRecord(t, ctx, store, files, ws.ID, "mch_document", "fld_file", "evil.pdf",
		"<html><body><script>alert(1)</script></body></html>")

	r := chi.NewRouter()
	r.Get("/uploads/*", serveUpload(store, files))
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+key, nil)
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws.ID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("serveUpload status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if disposition := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment (never inline for HTML content)", disposition)
	}
}

// TestServeUpload_allowsInlinePDF confirms the app's own legitimate case (a real PDF) still opens
// inline, since that's the UX the signature-placement/PDF-preview screens depend on.
func TestServeUpload_allowsInlinePDF(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "serve_upload_pdf_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Serve Upload PDF Test", "serve-upload-pdf-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	key := newUploadOwnedByRecord(t, ctx, store, files, ws.ID, "mch_document", "fld_file", "contract.pdf",
		"%PDF-1.4\nreal pdf-looking bytes\n")

	r := chi.NewRouter()
	r.Get("/uploads/*", serveUpload(store, files))
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+key, nil)
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws.ID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("serveUpload status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if disposition := rec.Header().Get("Content-Disposition"); strings.HasPrefix(disposition, "attachment") {
		t.Errorf("Content-Disposition = %q, want inline for a real PDF", disposition)
	}
}

// TestServeUpload_rejectsForeignWorkspaceUpload is the core regression test for M1 (IDOR): a
// member of one Workspace must not be able to read another Workspace's uploaded file just by
// knowing or guessing its storage key.
func TestServeUpload_rejectsForeignWorkspaceUpload(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const ownerEmail = "serve_upload_idor_owner@example.com"
	const attackerEmail = "serve_upload_idor_attacker@example.com"

	owner, err := store.CreateWorkspace(ctx, "Serve Upload IDOR Owner", "serve-upload-idor-owner-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace(owner): %v", err)
	}
	cleanupAuthTest(t, pool, owner.ID, ownerEmail)

	attacker, err := store.CreateWorkspace(ctx, "Serve Upload IDOR Attacker", "serve-upload-idor-attacker-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace(attacker): %v", err)
	}
	cleanupAuthTest(t, pool, attacker.ID, attackerEmail)

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	key := newUploadOwnedByRecord(t, ctx, store, files, owner.ID, "mch_document", "fld_file", "confidential.pdf",
		"%PDF-1.4\nthe owner workspace's own confidential document\n")

	r := chi.NewRouter()
	r.Get("/uploads/*", serveUpload(store, files))
	// Requested with the *attacker's* Workspace in context, same key -- as if the attacker guessed
	// or otherwise learned the owner's storage key.
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+key, nil)
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), attacker.ID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("serveUpload(foreign workspace) status = %d, want 404", rec.Code)
	}
}
