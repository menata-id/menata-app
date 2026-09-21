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
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/storage"
)

// wizardForm builds the multipart body the submission wizard posts: one Document plus N approver
// rows, each row contributing one value to each of the four parallel field slices.
//
// It writes every row's every field, exactly as the rendered form does -- which is the property
// under test. A helper that skipped empty values would hide the bug it exists to catch.
func wizardForm(t *testing.T, title, docType, mode string, pdf []byte, rows []stepInput) (string, *bytes.Buffer) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	write := func(k, v string) {
		t.Helper()
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%s): %v", k, err)
		}
	}
	write("fld_title", title)
	write("fld_document_type", docType)
	write(action.FieldDocumentMode, mode)
	for _, r := range rows {
		write(action.FieldStepName, r.name)
		write(action.FieldStepApproverType, r.approverType)
		write(action.FieldStepAssignee, r.assignee)
		write(action.FieldStepApproverGroup, r.approverGroup)
	}
	fw, err := mw.CreateFormFile("fld_file", "contract.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(pdf); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return mw.FormDataContentType(), &body
}

func testPDF(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "pdf", "testdata", "blank.pdf"))
	if err != nil {
		t.Fatalf("read testdata/blank.pdf: %v", err)
	}
	return b
}

// TestSubmitDocumentWizard_writesBothApproverKinds is what makes CAP-F24 real rather than merely
// built: the gate shipped in Fase 6c-1 and nothing wrote its Fields until this wizard did.
//
// The Group row sits in the MIDDLE deliberately. Row order is fld_sequence -- there is no hidden
// sequence input -- so the four per-row form values arrive as parallel slices, and a row whose
// controls submit a different number of values than its neighbours would attach approvers to the
// wrong steps. A Group row between two User rows is the arrangement that catches that; a Group row
// last would pass even with the slices misaligned.
func TestSubmitDocumentWizard_writesBothApproverKinds(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Wizard Approver Kinds", "wizard-approver-kinds-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "wizard_approver_kinds@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	group, err := store.CreateGroup(wsCtx, ws.ID, "Legal Group")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	rina, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Rina Nur", "fld_email": "rina@example.com"})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}
	maya, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Maya Puspita", "fld_email": "maya@example.com"})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	machines := loadRealMachines(t)

	contentType, body := wizardForm(t, "Vendor Contract Q3", "Kontrak", "sequential", testPDF(t), []stepInput{
		{name: "Finance Review", approverType: domain.ActorKindUser, assignee: rina.ID},
		{name: "Legal Review", approverType: domain.ActorKindGroup, approverGroup: group.ID},
		{name: "Director", approverType: domain.ActorKindUser, assignee: maya.ID},
	})

	// A real session, which this test did not carry before Fase 7's follow-up: mch_document now
	// declares prm_create_own_document, and an unidentified caller satisfies no Permission at
	// all (authorization.AllowsAction: "an unidentified caller is not an actor"). In the running
	// app this route is inside requireAuth so a session is always present; mounting the handler
	// directly is what let the test post without one.
	// Submitting is now a role-gated action (owner decision, 2026-09-21: a reviewer may only
	// look), so the identity posting this has to hold one that grants it.
	if err := store.SetMemberAppRole(wsCtx, ws.ID, rina.ID, "app_document_approval", "submitter"); err != nil {
		t.Fatalf("SetMemberAppRole: %v", err)
	}
	cfg := config.Config{SessionSecret: "test-secret-for-document-wizard"}
	r := chi.NewRouter()
	r.Post("/documents", submitDocumentWizard(machines, store, files, cfg))
	req := httptest.NewRequest(http.MethodPost, "/documents", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, rina.ID, 0)})
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code >= 400 {
		t.Fatalf("submit status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	steps, err := store.ListRecords(wsCtx, action.StepMachineID)
	if err != nil {
		t.Fatalf("ListRecords(steps): %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("created %d Approval Steps, want 3", len(steps))
	}
	bySeq := map[float64]*data.Record{}
	for _, s := range steps {
		seq, _ := s.Values[action.FieldStepSequence].(float64)
		bySeq[seq] = s
	}

	// Step 2 is the Group row: the group is named, and fld_assignee is empty -- which is only
	// storable because fld_assignee stopped being required in this same phase.
	two := bySeq[2]
	if two == nil {
		t.Fatal("no step at sequence 2")
	}
	if got := fmt.Sprint(two.Values[action.FieldStepApproverType]); got != domain.ActorKindGroup {
		t.Errorf("step 2 approver type = %q, want %q", got, domain.ActorKindGroup)
	}
	if got := fmt.Sprint(two.Values[action.FieldStepApproverGroup]); got != group.ID {
		t.Errorf("step 2 approver group = %q, want %q", got, group.ID)
	}
	if got := fmt.Sprint(two.Values[action.FieldStepAssignee]); got != "" && got != "<nil>" {
		t.Errorf("step 2 assignee = %q, want empty -- a Group-held step names no person", got)
	}
	if got := fmt.Sprint(two.Values[action.FieldStepName]); got != "Legal Review" {
		t.Errorf("step 2 name = %q, want %q -- fld_step_name was declared in 6b and had no writer until now", got, "Legal Review")
	}

	// The User rows either side must still be theirs. This is the alignment assertion.
	if got := fmt.Sprint(bySeq[1].Values[action.FieldStepAssignee]); got != rina.ID {
		t.Errorf("step 1 assignee = %q, want Rina (%q) -- parallel row slices are misaligned", got, rina.ID)
	}
	if got := fmt.Sprint(bySeq[3].Values[action.FieldStepAssignee]); got != maya.ID {
		t.Errorf("step 3 assignee = %q, want Maya (%q) -- parallel row slices are misaligned", got, maya.ID)
	}
	// A User row stores no approver type at all, so authorization's own fallback stays the live
	// path rather than becoming dead code nobody exercises.
	if got := fmt.Sprint(bySeq[1].Values[action.FieldStepApproverType]); got != "" && got != "<nil>" {
		t.Errorf("step 1 approver type = %q, want unset so the actor_field fallback still applies", got)
	}
}

// A row claiming Group must name one. Without this the step would be created with a Group arm
// pointing nowhere, which authorization refuses -- so it would be a step nobody could ever decide,
// created silently at submission time.
func TestSubmitDocumentWizard_rejectsAGroupRowWithNoGroup(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Wizard Group Row Guard", "wizard-group-row-guard-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "wizard_group_row_guard@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	contentType, body := wizardForm(t, "Bad Submission", "Kontrak", "sequential", testPDF(t), []stepInput{
		{name: "Legal Review", approverType: domain.ActorKindGroup},
	})

	r := chi.NewRouter()
	r.Post("/documents", submitDocumentWizard(loadRealMachines(t), store, files, config.Config{}))
	req := httptest.NewRequest(http.MethodPost, "/documents", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must name a group") {
		t.Errorf("body = %q, want it to say which row is wrong", rec.Body.String())
	}
	// And nothing was written: the guard runs before CreateRecord, so a rejected submission
	// leaves no orphaned Document behind.
	docs, err := store.ListRecords(wsCtx, action.DocumentMachineID)
	if err != nil {
		t.Fatalf("ListRecords(documents): %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("created %d Documents on a rejected submission, want 0", len(docs))
	}
}
