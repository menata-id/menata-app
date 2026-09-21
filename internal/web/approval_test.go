package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"menata.app/internal/pdf"
	"menata.app/internal/storage"
)

// approvalStepTestMachine mirrors metadata/approval_step.yaml's real shape closely enough for
// decideStep's own logic (fields it reads/writes, and prm_decide_own_step) without loading YAML,
// the same posture record_test.go's fileFieldMachine already takes.
func approvalStepTestMachine() *domain.Machine {
	return &domain.Machine{
		ID:   action.StepMachineID,
		Name: "Approval Step",
		Fields: []domain.Field{
			{ID: action.FieldStepDocument, Name: "Document", Type: domain.FieldTypeRelation, RelatedMachine: action.DocumentMachineID},
			{ID: action.FieldStepSequence, Name: "Sequence", Type: domain.FieldTypeNumber, Required: true},
			{ID: action.FieldStepAssignee, Name: "Assignee", Type: domain.FieldTypePerson, Required: true},
			{ID: action.FieldStepDecision, Name: "Decision", Type: domain.FieldTypeStatus, Required: true, Options: []string{action.DecisionPending, action.DecisionApproved, action.DecisionRejected}},
			{ID: action.FieldStepSignaturePage, Name: "Signature Page", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureX, Name: "Signature X", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureY, Name: "Signature Y", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureWidth, Name: "Signature Width", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureImage, Name: "Signature Image", Type: domain.FieldTypeFile},
		},
		// ApplicationID, the roles: arm and the transitions below all mirror
		// metadata/approval_step.yaml as of Fase 7, for the same reason the rollup Event does: a
		// fixture that omits them lets these tests pass against a Machine looser than the one that
		// actually runs. internal/conformance.TestApprovalStepPermissionsCarryRoles and
		// TestApprovalStepDeclaresItsTransitions are what keep the real manifest honest; this is
		// what keeps the fixture honest about the manifest.
		ApplicationID: "app_document_approval",
		Permissions: []domain.Permission{
			{ID: "prm_decide_own_step", Action: domain.ActionDecide, Roles: approverOrReviewer, ActorField: action.FieldStepAssignee},
			{ID: "prm_edit_own_step", Action: domain.ActionEdit, Roles: approverOrReviewer, ActorField: action.FieldStepAssignee},
			{ID: "prm_delete_own_step", Action: domain.ActionDelete, Roles: approverOrReviewer, ActorField: action.FieldStepAssignee},
		},
		Transitions: []domain.Transition{
			{ID: "trn_step_approve", Name: "Approve", Field: action.FieldStepDecision, From: action.DecisionPending, To: action.DecisionApproved, Action: domain.ActionDecide},
			{ID: "trn_step_reject", Name: "Reject", Field: action.FieldStepDecision, From: action.DecisionPending, To: action.DecisionRejected, Action: domain.ActionDecide},
		},
		// Mirrors metadata/approval_step.yaml's own evt_step_decision_rollup -- the Document's
		// status follows its steps by declaration now, not by a hardcoded recompute in the
		// handler, so these tests only exercise the real behaviour if the fixture declares it.
		// internal/conformance.TestApprovalStepDeclaresStatusRollup is what keeps the two in step.
		Events: []domain.Event{{
			ID: "evt_step_decision_rollup",
			On: action.FieldStepDecision,
			Then: domain.Service{Name: domain.ServiceRollupParentStatus, Rollup: &domain.Rollup{
				ParentField: action.FieldStepDocument,
				TargetField: action.FieldDocumentStatus,
				AnyValue:    action.DecisionRejected, AnySet: action.DocumentStatusRejected,
				AllValue: action.DecisionApproved, AllSet: action.DocumentStatusApproved,
				Default: action.DocumentStatusInReview,
			}},
		}},
	}
}

func documentTestMachine() *domain.Machine {
	return &domain.Machine{
		ID:   action.DocumentMachineID,
		Name: "Document",
		Fields: []domain.Field{
			{ID: action.FieldDocumentMode, Name: "Mode", Type: domain.FieldTypeStatus, Options: []string{"sequential", "parallel"}},
			{ID: action.FieldDocumentStatus, Name: "Status", Type: domain.FieldTypeStatus, Options: []string{action.DocumentStatusInReview, action.DocumentStatusApproved, action.DocumentStatusRejected}},
			{ID: action.FieldDocumentFile, Name: "File", Type: domain.FieldTypeFile},
		},
	}
}

// testSignaturePNGDataURL builds a real, tiny PNG and returns it as the same
// `data:image/png;base64,...` shape decideButtons' own canvas modal (rendering/detail.templ)
// produces via canvas.toDataURL("image/png").
func testSignaturePNGDataURL(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 10, G: 10, B: 10, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// testDocumentPDFBytes reuses internal/action's own tiny real (pdfcpu-parseable) test PDF rather
// than duplicating a fixture -- a document needs a real file on disk for signDocument's own
// loadDocumentPDF/api.PageDims to succeed, unlike document_test.go's upload-sniffing tests, which
// only need bytes that *look* like a PDF to http.DetectContentType.
func testDocumentPDFBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "action", "testdata", "blank.pdf"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return data
}

// decideStepTestSetup is the shared fixture every test below needs: a Workspace, a real
// (pdfcpu-parseable) Document PDF, a two-step sequential Approval flow, and the two distinct
// assignee identities for those steps.
type decideStepTestSetup struct {
	store       *data.Store
	files       *storage.Store
	cfg         config.Config
	machines    map[string]*domain.Machine
	ctx         context.Context
	workspaceID string
	documentID  string
	assignee    string
	stepID      string
	assignee2   string
	step2ID     string
}

func newDecideStepTestSetup(t *testing.T, testName string) decideStepTestSetup {
	t.Helper()
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "decide-step-test-secret", SecureCookies: false}
	ctx := context.Background()
	email := testName + "@example.com"

	ws, err := store.CreateWorkspace(ctx, testName, strings.ReplaceAll(testName, "_", "-"))
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	assignee, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_name": "Rina Nur", "fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(assignee): %v", err)
	}
	// The Application role prm_decide_own_step now also requires (CAP-P01, Fase 7). Granted
	// directly here; the Group-granted path is covered in signatureplacement_test.go, so between
	// them both arms of data.EffectiveRoles reach a real request.
	if err := grantApproverRole(t, store, ctx, ws.ID, assignee.ID); err != nil {
		t.Fatalf("SetMemberAppRole(assignee): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, assignee.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	email2 := "second_" + email
	assignee2, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_name": "Budi Santoso", "fld_email": email2})
	if err != nil {
		t.Fatalf("CreateRecord(assignee2): %v", err)
	}
	if err := grantApproverRole(t, store, ctx, ws.ID, assignee2.ID); err != nil {
		t.Fatalf("SetMemberAppRole(assignee2): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, assignee2.ID, email2, "member", ""); err != nil {
		t.Fatalf("AddMember(assignee2): %v", err)
	}

	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("storage.NewStore: %v", err)
	}
	docKey, err := files.Save(action.DocumentMachineID, action.FieldDocumentFile, "contract.pdf", bytes.NewReader(testDocumentPDFBytes(t)))
	if err != nil {
		t.Fatalf("Save(document PDF): %v", err)
	}

	document, err := store.CreateRecord(wsCtx, action.DocumentMachineID, map[string]any{
		action.FieldDocumentMode: "sequential",
		action.FieldDocumentFile: docKey,
	})
	if err != nil {
		t.Fatalf("CreateRecord(document): %v", err)
	}

	step, err := store.CreateRecord(wsCtx, action.StepMachineID, map[string]any{
		action.FieldStepDocument:       document.ID,
		action.FieldStepSequence:       float64(1),
		action.FieldStepAssignee:       assignee.ID,
		action.FieldStepDecision:       action.DecisionPending,
		action.FieldStepSignatureX:     float64(50),
		action.FieldStepSignatureY:     float64(50),
		action.FieldStepSignaturePage:  float64(1),
		action.FieldStepSignatureWidth: float64(20),
	})
	if err != nil {
		t.Fatalf("CreateRecord(step): %v", err)
	}
	step2, err := store.CreateRecord(wsCtx, action.StepMachineID, map[string]any{
		action.FieldStepDocument:       document.ID,
		action.FieldStepSequence:       float64(2),
		action.FieldStepAssignee:       assignee2.ID,
		action.FieldStepDecision:       action.DecisionPending,
		action.FieldStepSignatureX:     float64(70),
		action.FieldStepSignatureY:     float64(50),
		action.FieldStepSignaturePage:  float64(1),
		action.FieldStepSignatureWidth: float64(20),
	})
	if err != nil {
		t.Fatalf("CreateRecord(step2): %v", err)
	}

	return decideStepTestSetup{
		store: store,
		files: files,
		cfg:   cfg,
		machines: map[string]*domain.Machine{
			action.DocumentMachineID: documentTestMachine(),
			action.StepMachineID:     approvalStepTestMachine(),
		},
		ctx:         wsCtx,
		workspaceID: ws.ID,
		documentID:  document.ID,
		assignee:    assignee.ID,
		stepID:      step.ID,
		assignee2:   assignee2.ID,
		step2ID:     step2.ID,
	}
}

// postDecide POSTs decision (+ any extra form values, e.g. signature_image/save_signature) to
// .../decide for setup's own first step, as its assignee -- the shape every gate test below
// needs. postDecideAs is the general form for tests that decide a specific step as a specific
// actor (the progressive-signing tests, which have two steps and two assignees).
func postDecide(t *testing.T, s decideStepTestSetup, decision string, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return postDecideAs(t, s, s.stepID, s.assignee, decision, extra)
}

// postDecideAs is postDecide's general form: which step, decided as which actor. Same
// session-cookie-only auth pattern switchworkspace_test.go's sessionCookieValueForTest already
// established for this package's handler-level tests -- no CSRF middleware in play, since this
// mounts decideStep directly rather than the full internal/web.Routes() chain.
func postDecideAs(t *testing.T, s decideStepTestSetup, stepID, actorID, decision string, extra map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"decision": {decision}}
	for k, v := range extra {
		form.Set(k, v)
	}
	req := httptest.NewRequest(http.MethodPost, "/machines/"+action.StepMachineID+"/records/"+stepID+"/decide", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(data.WithWorkspaceScope(req.Context(), s.workspaceID))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Post("/machines/{machineID}/records/{id}/decide", decideStep(s.machines, s.store, s.files, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestDecideStep_approveRequiresSignatureWhenNoneOnFile is the write-time half of the
// signature-capture gate (owner request, 2026-09-19): approving with no signature_image and no
// saved mch_signature must be refused, not silently approved with no stamp -- this is what stops
// a client that bypasses decideButtons' own canvas modal (or has JS disabled).
func TestDecideStep_approveRequiresSignatureWhenNoneOnFile(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_requires_signature")

	rec := postDecide(t, s, action.DecisionApproved, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("decideStep(approve, no signature) status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[action.FieldStepDecision]; got != action.DecisionPending {
		t.Errorf("step decision = %v, want still pending after a refused approve", got)
	}
}

// TestDecideStep_approveCapturesSignatureWithoutSavingByDefault covers the owner's own default
// (checkbox unchecked): a captured signature is stamped onto this one step (fld_signature_image)
// but never becomes a reusable mch_signature -- so the same identity's next document asks again.
func TestDecideStep_approveCapturesSignatureWithoutSavingByDefault(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_capture_no_save")

	rec := postDecide(t, s, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decideStep(approve, drawn signature) status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[action.FieldStepDecision]; got != action.DecisionApproved {
		t.Errorf("step decision = %v, want approved", got)
	}
	if key, _ := step.Values[action.FieldStepSignatureImage].(string); key == "" {
		t.Error("step fld_signature_image is empty, want the captured image's storage key")
	}

	saved, err := hasSavedSignature(s.ctx, s.store, s.assignee)
	if err != nil {
		t.Fatalf("hasSavedSignature: %v", err)
	}
	if saved {
		t.Error("hasSavedSignature = true, want false: save_signature was not submitted, so nothing should have been persisted to mch_signature")
	}
}

// TestDecideStep_approveSavesSignatureWhenRequested covers save_signature=on: the same capture
// as above, but this time a reusable mch_signature is created too, so a later document's Approve
// skips the modal (TestDecideStep_approveSkipsGateWhenAlreadySaved below).
func TestDecideStep_approveSavesSignatureWhenRequested(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_capture_and_save")

	rec := postDecide(t, s, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
		"save_signature":  "on",
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decideStep(approve, save signature) status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	saved, err := hasSavedSignature(s.ctx, s.store, s.assignee)
	if err != nil {
		t.Fatalf("hasSavedSignature: %v", err)
	}
	if !saved {
		t.Error("hasSavedSignature = false, want true: save_signature=on should have created a reusable mch_signature")
	}
}

// TestDecideStep_approveSkipsGateWhenAlreadySaved is the other side of the gate: an identity that
// already has a reusable mch_signature on file approves directly, no signature_image required.
func TestDecideStep_approveSkipsGateWhenAlreadySaved(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_skips_gate")

	if _, err := s.store.CreateRecord(s.ctx, action.SignatureMachineID, map[string]any{
		action.FieldSignatureOwner: s.assignee,
		action.FieldSignatureImage: "sig_test/fld_image/pre-existing.png",
	}); err != nil {
		t.Fatalf("CreateRecord(mch_signature): %v", err)
	}

	rec := postDecide(t, s, action.DecisionApproved, nil)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decideStep(approve, already saved) status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[action.FieldStepDecision]; got != action.DecisionApproved {
		t.Errorf("step decision = %v, want approved", got)
	}
}

// TestDecideStep_signsDocumentAfterEveryApproval is the owner's own end-to-end request
// (2026-09-19): "setiap step, image tandatangan langsung masuk di pdf" -- the Document's own
// fld_signed_file must exist right after the *first* of two sequential steps approves, not only
// once the whole Document is fully approved, and the top-of-page-1 status banner must already
// show that one step by name.
func TestDecideStep_signsDocumentAfterEveryApproval(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_signs_progressively")

	rec := postDecideAs(t, s, s.stepID, s.assignee, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decideStep(step 1 approve) status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	document, err := s.store.GetRecord(s.ctx, action.DocumentMachineID, s.documentID)
	if err != nil {
		t.Fatalf("GetRecord(document): %v", err)
	}
	if got := document.Values[action.FieldDocumentStatus]; got != action.DocumentStatusInReview {
		t.Fatalf("document status = %v, want still in_review (step 2 of 2 is still pending)", got)
	}
	signedKey, _ := document.Values[action.FieldDocumentSignedFile].(string)
	if signedKey == "" {
		t.Fatal("document fld_signed_file is empty after step 1 of 2 approved, want it populated immediately")
	}

	signedPath, err := s.files.Path(signedKey)
	if err != nil {
		t.Fatalf("files.Path(signed): %v", err)
	}
	signedBytes, err := os.ReadFile(signedPath)
	if err != nil {
		t.Fatalf("read signed PDF: %v", err)
	}
	rendered, err := pdf.RenderPagePNG(signedBytes, 0, 900, 900)
	if err != nil {
		t.Fatalf("render signed page: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(rendered))
	if err != nil {
		t.Fatalf("decode rendered png: %v", err)
	}
	r, g, b, _ := img.At(3, 3).RGBA()
	r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
	if r8 <= g8+3 || b8 <= g8+3 {
		t.Errorf("top-left pixel of the signed page = rgb(%d,%d,%d), want purple-tinted (the status banner should already be there after just step 1)", r8, g8, b8)
	}

	// Now the second (and last) step approves -- the document completes, and the banner should
	// grow to name both approvers, not just replace the first one.
	rec2 := postDecideAs(t, s, s.step2ID, s.assignee2, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
	})
	if rec2.Code != http.StatusSeeOther && rec2.Code != http.StatusFound {
		t.Fatalf("decideStep(step 2 approve) status = %d, want a redirect; body=%s", rec2.Code, rec2.Body.String())
	}
	document2, err := s.store.GetRecord(s.ctx, action.DocumentMachineID, s.documentID)
	if err != nil {
		t.Fatalf("GetRecord(document) after step 2: %v", err)
	}
	if got := document2.Values[action.FieldDocumentStatus]; got != action.DocumentStatusApproved {
		t.Errorf("document status = %v, want approved once both steps are decided", got)
	}
	if key, _ := document2.Values[action.FieldDocumentSignedFile].(string); key == "" {
		t.Error("document fld_signed_file is empty after both steps approved")
	}
}

// approverOrReviewer mirrors the roles: arm on all three of mch_approval_step's Permissions.
var approverOrReviewer = []string{"approver", "reviewer"}

// grantApproverRole gives a member the Document Approval role its Permissions require, so these
// handler tests exercise the real gate rather than the un-roled path the fixture used to take.
func grantApproverRole(t *testing.T, store *data.Store, ctx context.Context, workspaceID, userRecordID string) error {
	t.Helper()
	return store.SetMemberAppRole(ctx, workspaceID, userRecordID, "app_document_approval", "approver")
}

// TestDecideStep_refusesAnActorWithoutTheRole is CAP-P01 at the route, and the assertion it makes
// is the behaviour change Fase 7 actually shipped: being the person a step names is no longer
// enough. The same identity, the same step, the same POST -- only the role row differs.
func TestDecideStep_refusesAnActorWithoutTheRole(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_role_gate")

	if err := s.store.SetMemberAppRole(s.ctx, s.workspaceID, s.assignee, "app_document_approval", ""); err != nil {
		t.Fatalf("clear app role: %v", err)
	}
	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("decideStep without the role: status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// And granting it back is all it takes -- which is what makes the Approval Role Matrix an
	// administrative screen rather than a report.
	if err := s.store.SetMemberAppRole(s.ctx, s.workspaceID, s.assignee, "app_document_approval", "reviewer"); err != nil {
		t.Fatalf("grant reviewer: %v", err)
	}
	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code >= 400 {
		t.Fatalf("decideStep holding reviewer: status = %d, want success; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDecideStep_refusesRedecidingADecidedStep is the declared state model at the route, and it
// covers the hole neither existing guard did: sequencing only ever locked a step behind an
// *earlier* one, so on a parallel Document an already-approved step could be decided again and
// re-run the rollup onto its Document. No transition leaves `approved`, so this is 422 now.
func TestDecideStep_refusesRedecidingADecidedStep(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_redecide")

	step, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	step.Values[action.FieldStepDecision] = action.DecisionApproved
	if _, err := s.store.UpdateRecord(s.ctx, action.StepMachineID, s.stepID, step.Values); err != nil {
		t.Fatalf("UpdateRecord(step): %v", err)
	}

	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("decideStep re-deciding an approved step: status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
	after, err := s.store.GetRecord(s.ctx, action.StepMachineID, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step) after: %v", err)
	}
	if got := after.Values[action.FieldStepDecision]; got != action.DecisionApproved {
		t.Errorf("step decision = %v, want still approved -- a decision is final", got)
	}
}
