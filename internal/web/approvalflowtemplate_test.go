package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// approvalFlowTemplateTestFixture builds the one Workspace/identity/Group shape every test below
// needs: a submitter allowed to post the wizard, and a Group approver so the same coverage
// TestSubmitDocumentWizard_writesBothApproverKinds already gives real Approval Steps extends to
// the saved template's own steps.
func approvalFlowTemplateTestFixture(t *testing.T, workspaceName, slug, email string) (store *data.Store, wsCtx context.Context, machines map[string]*domain.Machine, cfg config.Config, submitter *data.Record, group *data.Group) {
	t.Helper()
	pool := authTestPool(t)
	store = data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, workspaceName, slug)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx = data.WithWorkspaceScope(ctx, ws.ID)

	// Carries the Application's own navigation (nav_my_documents et al.), same as every request in
	// the real router (currentWorkspace middleware) -- submitDocumentWizard's own Draft branch
	// resolves its redirect through navRouteByID, which panics without this. testWorkspaceFor
	// (record_test.go) is not enough here: it carries MachineIDs only, no Applications/navigation
	// at all, so loadRealMachines' own second return value (the real, loaded Workspace) is used
	// instead.
	var loadedWorkspace domain.Workspace
	machines, loadedWorkspace = loadRealMachines(t)
	wsCtx = rendering.WithCurrentWorkspace(wsCtx, loadedWorkspace, workspaceName, false)

	group, err = store.CreateGroup(wsCtx, ws.ID, "Legal Group")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	submitter, err = store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Rina Nur", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}
	if err := store.SetMemberAppRole(wsCtx, ws.ID, submitter.ID, "app_document_approval", "submitter"); err != nil {
		t.Fatalf("SetMemberAppRole: %v", err)
	}
	cfg = config.Config{SessionSecret: "test-secret-for-approval-flow-template"}
	return store, wsCtx, machines, cfg, submitter, group
}

func postWizard(t *testing.T, machines map[string]*domain.Machine, store *data.Store, cfg config.Config, submitterID string, wsCtx context.Context, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	r := chi.NewRouter()
	r.Post("/documents", submitDocumentWizard(store, files, cfg))
	req := httptest.NewRequest(http.MethodPost, "/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, submitterID, 0)})
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestSubmitDocumentWizard_savesDefaultApprovalFlowWhenChecked is CAP-V28's own "save" half
// (ROADMAP.md, 2026-09-27): checking the box writes one Approval Flow Template plus one step per
// approver row, with the same Document Type/Mode/rows the real submission used.
func TestSubmitDocumentWizard_savesDefaultApprovalFlowWhenChecked(t *testing.T) {
	store, wsCtx, machines, cfg, submitter, group := approvalFlowTemplateTestFixture(t, "Save Default Flow", "save-default-flow-workspace", "save_default_flow@example.com")

	contentType, body := wizardForm(t, "Vendor Contract", "Kontrak", "sequential", testPDF(t), []stepInput{
		{name: "Finance Review", approverType: domain.ActorKindUser, assignee: submitter.ID},
		{name: "Legal Review", approverType: domain.ActorKindGroup, approverGroup: group.ID},
	}, true)
	rec := postWizard(t, machines, store, cfg, submitter.ID, wsCtx, contentType, body)
	if rec.Code >= 400 {
		t.Fatalf("submit status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	templates, err := store.ListRecordsBy(wsCtx, action.TemplateMachineID, action.FieldTemplateDocumentType, "Kontrak")
	if err != nil {
		t.Fatalf("ListRecordsBy(templates): %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("templates for Kontrak = %d, want 1", len(templates))
	}
	if got := fmt.Sprint(templates[0].Values[action.FieldTemplateMode]); got != "sequential" {
		t.Errorf("template mode = %q, want %q", got, "sequential")
	}

	steps, err := store.ListRecordsBy(wsCtx, action.TemplateStepMachineID, action.FieldTemplateStepTemplate, templates[0].ID)
	if err != nil {
		t.Fatalf("ListRecordsBy(template steps): %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("template steps = %d, want 2", len(steps))
	}
	bySeq := map[float64]*data.Record{}
	for _, s := range steps {
		seq, _ := s.Values[action.FieldTemplateStepSequence].(float64)
		bySeq[seq] = s
	}
	if got := fmt.Sprint(bySeq[1].Values[action.FieldTemplateStepAssignee]); got != submitter.ID {
		t.Errorf("step 1 assignee = %q, want %q", got, submitter.ID)
	}
	if got := fmt.Sprint(bySeq[2].Values[action.FieldTemplateStepApproverGroup]); got != group.ID {
		t.Errorf("step 2 approver group = %q, want %q", got, group.ID)
	}
	if got := fmt.Sprint(bySeq[2].Values[action.FieldTemplateStepName]); got != "Legal Review" {
		t.Errorf("step 2 name = %q, want %q", got, "Legal Review")
	}
}

// TestSubmitDocumentWizard_doesNotSaveDefaultApprovalFlowWhenUnchecked is the checkbox's negative
// case: an ordinary submission (every existing wizardForm caller) must create no template.
func TestSubmitDocumentWizard_doesNotSaveDefaultApprovalFlowWhenUnchecked(t *testing.T) {
	store, wsCtx, machines, cfg, submitter, _ := approvalFlowTemplateTestFixture(t, "No Default Flow", "no-default-flow-workspace", "no_default_flow@example.com")

	contentType, body := wizardForm(t, "Vendor Contract", "Tagihan", "sequential", testPDF(t), []stepInput{
		{name: "Finance Review", approverType: domain.ActorKindUser, assignee: submitter.ID},
	}, false)
	rec := postWizard(t, machines, store, cfg, submitter.ID, wsCtx, contentType, body)
	if rec.Code >= 400 {
		t.Fatalf("submit status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	templates, err := store.ListRecordsBy(wsCtx, action.TemplateMachineID, action.FieldTemplateDocumentType, "Tagihan")
	if err != nil {
		t.Fatalf("ListRecordsBy(templates): %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("templates for Tagihan = %d, want 0 -- the checkbox was not checked", len(templates))
	}
}

// TestSubmitDocumentWizard_resavingDefaultApprovalFlowReplacesSteps is find-or-create's own "not
// find-or-append" half: submitting the same Document Type a second time, with a different chain,
// replaces the saved steps rather than accumulating a second set under the same template.
func TestSubmitDocumentWizard_resavingDefaultApprovalFlowReplacesSteps(t *testing.T) {
	store, wsCtx, machines, cfg, submitter, group := approvalFlowTemplateTestFixture(t, "Resave Default Flow", "resave-default-flow-workspace", "resave_default_flow@example.com")

	first, firstBody := wizardForm(t, "First Contract", "Kontrak", "sequential", testPDF(t), []stepInput{
		{name: "Finance Review", approverType: domain.ActorKindUser, assignee: submitter.ID},
	}, true)
	rec := postWizard(t, machines, store, cfg, submitter.ID, wsCtx, first, firstBody)
	if rec.Code >= 400 {
		t.Fatalf("first submit status = %d; body=%s", rec.Code, rec.Body.String())
	}

	second, secondBody := wizardForm(t, "Second Contract", "Kontrak", "parallel", testPDF(t), []stepInput{
		{name: "Legal Review", approverType: domain.ActorKindGroup, approverGroup: group.ID},
		{name: "Finance Review", approverType: domain.ActorKindUser, assignee: submitter.ID},
	}, true)
	rec = postWizard(t, machines, store, cfg, submitter.ID, wsCtx, second, secondBody)
	if rec.Code >= 400 {
		t.Fatalf("second submit status = %d; body=%s", rec.Code, rec.Body.String())
	}

	templates, err := store.ListRecordsBy(wsCtx, action.TemplateMachineID, action.FieldTemplateDocumentType, "Kontrak")
	if err != nil {
		t.Fatalf("ListRecordsBy(templates): %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("templates for Kontrak = %d, want 1 -- find-or-create must reuse the existing one", len(templates))
	}
	if got := fmt.Sprint(templates[0].Values[action.FieldTemplateMode]); got != "parallel" {
		t.Errorf("template mode = %q, want %q -- the second submission's mode must win", got, "parallel")
	}

	steps, err := store.ListRecordsBy(wsCtx, action.TemplateStepMachineID, action.FieldTemplateStepTemplate, templates[0].ID)
	if err != nil {
		t.Fatalf("ListRecordsBy(template steps): %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("template steps = %d, want 2 -- the first submission's one step must have been replaced, not kept alongside the second's two", len(steps))
	}
}

// TestSubmitDocumentWizard_draftNeverSavesADefaultApprovalFlow: a Draft creates no Approval Steps
// at all (submitDocumentWizard's own isDraft branch returns before either), so even a checked box
// must not reach saveApprovalFlowTemplate -- there is no completed chain yet to save.
func TestSubmitDocumentWizard_draftNeverSavesADefaultApprovalFlow(t *testing.T) {
	store, wsCtx, machines, cfg, submitter, _ := approvalFlowTemplateTestFixture(t, "Draft No Default Flow", "draft-no-default-flow-workspace", "draft_no_default_flow@example.com")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	write := func(k, v string) {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%s): %v", k, err)
		}
	}
	write("fld_title", "Draft Contract")
	write("fld_document_type", "Kontrak")
	write(action.FieldDocumentMode, "sequential")
	write("save_as_default_flow", "1")
	write("intent", "draft")
	fw, err := mw.CreateFormFile("fld_file", "contract.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(testPDF(t)); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rec := postWizard(t, machines, store, cfg, submitter.ID, wsCtx, mw.FormDataContentType(), &buf)
	if rec.Code >= 400 {
		t.Fatalf("draft submit status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	templates, err := store.ListRecordsBy(wsCtx, action.TemplateMachineID, action.FieldTemplateDocumentType, "Kontrak")
	if err != nil {
		t.Fatalf("ListRecordsBy(templates): %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("templates for Kontrak = %d, want 0 -- a Draft has no completed chain to save", len(templates))
	}
}

// TestShowApprovalFlowTemplateRows_returnsSavedStepsAndMode is CAP-V28's own "find" half: the
// doc_type <select>'s own htmx fragment renders a saved template's steps back as prefilled
// ApproverRows, and its mode out-of-band.
func TestShowApprovalFlowTemplateRows_returnsSavedStepsAndMode(t *testing.T) {
	store, wsCtx, machines, cfg, submitter, group := approvalFlowTemplateTestFixture(t, "Load Default Flow", "load-default-flow-workspace", "load_default_flow@example.com")

	contentType, body := wizardForm(t, "Vendor Contract", "Kontrak", "parallel", testPDF(t), []stepInput{
		{name: "Legal Review", approverType: domain.ActorKindGroup, approverGroup: group.ID},
	}, true)
	rec := postWizard(t, machines, store, cfg, submitter.ID, wsCtx, contentType, body)
	if rec.Code >= 400 {
		t.Fatalf("submit status = %d; body=%s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/documents/new/approval-flow-template?fld_document_type=Kontrak", nil)
	req = req.WithContext(wsCtx)
	rec2 := httptest.NewRecorder()
	showApprovalFlowTemplateRows(store)(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec2.Code, rec2.Body.String())
	}
	html := rec2.Body.String()
	if !strings.Contains(html, "Legal Review") {
		t.Errorf("body missing the saved step's name: %s", html)
	}
	if !strings.Contains(html, `value="`+group.ID+`" selected`) {
		t.Errorf("body missing the saved step's selected group: %s", html)
	}
	if !strings.Contains(html, `value="parallel" checked`) {
		t.Errorf("body missing the saved template's selected mode: %s", html)
	}
	if !strings.Contains(html, `hx-swap-oob="true"`) {
		t.Errorf("body missing the mode fieldset's own out-of-band swap: %s", html)
	}
}

// TestShowApprovalFlowTemplateRows_blankWhenNoTemplateSaved is the fragment's own negative case:
// a Document Type with no saved template renders exactly a fresh wizard's own starting state.
func TestShowApprovalFlowTemplateRows_blankWhenNoTemplateSaved(t *testing.T) {
	store, wsCtx, _, _, _, _ := approvalFlowTemplateTestFixture(t, "No Saved Flow", "no-saved-flow-workspace", "no_saved_flow@example.com")

	req := httptest.NewRequest(http.MethodGet, "/documents/new/approval-flow-template?fld_document_type=Lain-lain", nil)
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	showApprovalFlowTemplateRows(store)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	html := rec.Body.String()
	if strings.Count(html, "approver-row") == 0 {
		t.Errorf("body has no blank approver row: %s", html)
	}
	if strings.Contains(html, "Legal Review") {
		t.Errorf("body leaked a step from a different Document Type's template: %s", html)
	}
}
