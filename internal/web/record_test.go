package web

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

func fileFieldMachine() *domain.Machine {
	return &domain.Machine{
		ID: "mch_record_test",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_file", Name: "File", Type: domain.FieldTypeFile},
		},
	}
}

// TestHandleFileUploads_nonMultipartBody is the regression test for ROADMAP.md's Operational
// backlog item "Non-multipart form bodies fail on any Machine with a file field" (pre-existing
// since Phase 19): a plain url-encoded POST to a Machine with a FieldTypeFile field previously
// 400'd, because only http.ErrMissingFile was tolerated from req.FormFile, not
// http.ErrNotMultipart.
func TestHandleFileUploads_nonMultipartBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("fld_title=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}

	uploaded, err := handleFileUploads(req, fileFieldMachine(), files)
	if err != nil {
		t.Fatalf("handleFileUploads(non-multipart body) error = %v, want nil", err)
	}
	if len(uploaded) != 0 {
		t.Errorf("handleFileUploads(non-multipart body) = %v, want empty (no file possible in a non-multipart body)", uploaded)
	}
}

// TestHandleFileUploads_multipartWithFile confirms the fix didn't break the real upload path --
// a genuinely multipart body with a file present still saves it.
func TestHandleFileUploads_multipartWithFile(t *testing.T) {
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("fld_title", "hello"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	part, err := writer.CreateFormFile("fld_file", "doc.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("pdf bytes")); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(maxUploadBytes); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}

	uploaded, err := handleFileUploads(req, fileFieldMachine(), files)
	if err != nil {
		t.Fatalf("handleFileUploads(multipart with file) error = %v, want nil", err)
	}
	if _, ok := uploaded["fld_file"]; !ok {
		t.Errorf("handleFileUploads(multipart with file) = %v, want a key for fld_file", uploaded)
	}
}

// uploadOneFile is the shared multipart-request builder for the content-sniffing tests below --
// TestHandleFileUploads_multipartWithFile already covers the non-rejection path with an arbitrary
// filename/content pair, so this only varies content, matching handleFileUploads' own sniff-by-
// bytes-not-extension behavior (security audit 2026-09-19, H2).
func uploadOneFile(t *testing.T, filename string, content []byte) (*http.Request, *storage.Store) {
	t.Helper()
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("fld_file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(maxUploadBytes); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	return req, files
}

// TestHandleFileUploads_rejectsScriptCapableContent is the core regression test for H2: a payload
// whose real bytes sniff as HTML/SVG/XML/JS must be rejected regardless of the filename/extension
// the uploader chose -- an "evil.pdf" carrying an HTML payload is exactly the disguise the
// pre-fix code let through.
func TestHandleFileUploads_rejectsScriptCapableContent(t *testing.T) {
	cases := []struct {
		name, filename string
		content        []byte
	}{
		{"html disguised as pdf", "evil.pdf", []byte("<html><body><script>alert(document.cookie)</script></body></html>")},
		{"svg with script", "signature.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"plain xml", "data.pdf", []byte(`<?xml version="1.0"?><root/>`)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, files := uploadOneFile(t, c.filename, c.content)
			if _, err := handleFileUploads(req, fileFieldMachine(), files); err == nil {
				t.Errorf("handleFileUploads(%s) error = nil, want a rejection", c.name)
			}
		})
	}
}

// TestHandleFileUploads_allowsRealPDF confirms the fix doesn't reject the app's own legitimate
// case (mch_document.fld_file) -- a real PDF-prefixed payload still saves.
func TestHandleFileUploads_allowsRealPDF(t *testing.T) {
	req, files := uploadOneFile(t, "contract.pdf", []byte("%PDF-1.4\n%real pdf content, not actually parseable but sniffs correctly\n"))
	uploaded, err := handleFileUploads(req, fileFieldMachine(), files)
	if err != nil {
		t.Fatalf("handleFileUploads(real PDF) error = %v, want nil", err)
	}
	if _, ok := uploaded["fld_file"]; !ok {
		t.Errorf("handleFileUploads(real PDF) = %v, want a key for fld_file", uploaded)
	}
}

// loadRealMachines loads the app's own real metadata/workspaces/default.yaml -- the same manifest cmd/server
// loads -- so a showRecordRow test exercises real Field/relation wiring (mch_approval_step's
// fld_document relation back to mch_document, in particular) rather than a hand-rolled stand-in
// that could silently drift from what ships. It also returns the installed Workspace, which the
// caller puts on the request ctx exactly as internal/web's currentWorkspace middleware does in
// production -- detailBackLink's own routeByID(ctx, "nav_approval_inbox") resolves through it and
// panics without it. Before 2026-09-22 this set package state and undid it in a Cleanup; a
// per-request value needs neither.
func loadRealMachines(t *testing.T) (map[string]*domain.Machine, domain.Workspace) {
	t.Helper()
	app, err := metadata.LoadApplication(filepath.Join("..", "..", "metadata", "workspaces", "default.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication: %v", err)
	}

	machines := make(map[string]*domain.Machine, len(app.Machines))
	for _, m := range app.Machines {
		machines[m.ID] = m
	}
	return machines, app.Workspace
}

// TestShowRecordRow_documentDetailIncludesInlineSignaturePlacement is the Fase 0 regression test
// (composable-runtime kajian): signaturePlacementBlock must render inline on a Document's own
// detail page, not just on the dedicated /signature-placement route -- proving the Component is
// genuinely composable into a second Page, with zero metadata change.
func TestShowRecordRow_documentDetailIncludesInlineSignaturePlacement(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "sig_placement_inline_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Signature Placement Inline Test", "signature-placement-inline-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	pdfBytes, err := os.ReadFile(filepath.Join("..", "pdf", "testdata", "blank.pdf"))
	if err != nil {
		t.Fatalf("read testdata/blank.pdf: %v", err)
	}
	key, err := files.Save(action.DocumentMachineID, action.FieldDocumentFile, "contract.pdf", bytes.NewReader(pdfBytes))
	if err != nil {
		t.Fatalf("files.Save: %v", err)
	}

	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	document, err := store.CreateRecord(wsCtx, action.DocumentMachineID, map[string]any{
		"fld_title":              "Test Document",
		action.FieldDocumentFile: key,
	})
	if err != nil {
		t.Fatalf("CreateRecord(document): %v", err)
	}
	if _, err := store.CreateRecord(wsCtx, action.StepMachineID, map[string]any{
		action.FieldStepDocument: document.ID,
		action.FieldStepSequence: float64(1),
		action.FieldStepAssignee: "usr_placeholder",
		action.FieldStepDecision: action.DecisionPending,
	}); err != nil {
		t.Fatalf("CreateRecord(step): %v", err)
	}

	machines, installed := loadRealMachines(t)
	r := chi.NewRouter()
	r.Get("/machines/{machineID}/records/{id}", showRecordRow(machines, store, files, config.Config{}))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/machines/%s/records/%s", action.DocumentMachineID, document.ID), nil)
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), ws.ID), installed, "Test Workspace"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("showRecordRow(document detail) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="sig-body"`) {
		t.Errorf("Document detail page missing inline signaturePlacementBlock (no #sig-body in response)")
	}
	if !strings.Contains(body, "Signature Positions") {
		t.Errorf("Document detail page missing the inline block's own section header")
	}
}

// TestShowRecordRow_nonDocumentDetailHasNoSignaturePlacement is the negative-side regression
// guard for the same change: threading files into showRecordRow/documentSignaturePlacementView
// must not affect any other Machine's own detail page.
func TestShowRecordRow_nonDocumentDetailHasNoSignaturePlacement(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "sig_placement_inline_negative_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Signature Placement Inline Negative Test", "signature-placement-inline-negative-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}

	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	project, err := store.CreateRecord(wsCtx, "mch_project", map[string]any{"fld_name": "Test Project"})
	if err != nil {
		t.Fatalf("CreateRecord(project): %v", err)
	}

	machines, installed := loadRealMachines(t)
	r := chi.NewRouter()
	r.Get("/machines/{machineID}/records/{id}", showRecordRow(machines, store, files, config.Config{}))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/machines/mch_project/records/%s", project.ID), nil)
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), ws.ID), installed, "Test Workspace"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("showRecordRow(project detail) status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, `id="sig-body"`) {
		t.Errorf("mch_project's own detail page unexpectedly contains signaturePlacementBlock's #sig-body")
	}
}

// realMachines is loadRealMachines for the callers that want only the Machine set -- they render
// no page whose links resolve through navigation, so they need no Workspace on ctx.
func realMachines(t *testing.T) map[string]*domain.Machine {
	t.Helper()
	machines, _ := loadRealMachines(t)
	return machines
}

// testWorkspaceFor returns an installed Workspace carrying every Machine in machines, for tests
// that exercise a handler directly rather than through the router's currentWorkspace middleware.
// The narrowing itself is covered by its own tests; here it would only be scaffolding in the way.
func testWorkspaceFor(machines map[string]*domain.Machine) domain.Workspace {
	ws := domain.Workspace{Slug: "test"}
	for id := range machines {
		ws.MachineIDs = append(ws.MachineIDs, id)
	}
	return ws
}
