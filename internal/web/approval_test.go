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
	"menata.app/internal/expression"
	"menata.app/internal/mail"
	"menata.app/internal/pdf"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// approvalStepTestMachine mirrors metadata/approval_step.yaml's real shape closely enough for
// decideStep's own logic (fields it reads/writes, and prm_decide_own_step) without loading YAML,
// the same posture record_test.go's fileFieldMachine already takes.
func approvalStepTestMachine(ids approvalIDs) *domain.Machine {
	return &domain.Machine{
		ID:   ids.step,
		Name: "Approval Step",
		Fields: []domain.Field{
			{ID: action.FieldStepDocument, Name: "Document", Type: domain.FieldTypeRelation, RelatedMachine: ids.document},
			{ID: action.FieldStepSequence, Name: "Sequence", Type: domain.FieldTypeNumber, Required: true},
			{ID: action.FieldStepAssignee, Name: "Assignee", Type: domain.FieldTypePerson, Required: true},
			{ID: ids.decision, Name: "Decision", Type: domain.FieldTypeStatus, Required: true, Options: []string{action.DecisionPending, action.DecisionApproved, action.DecisionRejected}},
			{ID: action.FieldStepSignaturePage, Name: "Signature Page", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureX, Name: "Signature X", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureY, Name: "Signature Y", Type: domain.FieldTypeNumber},
			{ID: action.FieldStepSignatureWidth, Name: "Signature Width", Type: domain.FieldTypeNumber},
			{ID: ids.signatureImage, Name: "Signature Image", Type: domain.FieldTypeFile},
			// The Field the declared decide effect writes. Absent until 2026-09-29, when running this
			// fixture through metadata.Validate said so: an actions: block naming a Field the Machine
			// does not declare is a Machine the runtime would refuse to load.
			{ID: ids.decidedBy, Name: "Decided By Name", Type: domain.FieldTypeText},
		},
		// Sequencing, the removal guard and the two Views all mirror metadata/approval_step.yaml as
		// well, and all three were missing until TestFixturesMirrorTheRealMachines was written below
		// (2026-09-28) -- which is the gate earning its place on its first run. Sequencing is the one
		// that mattered: decideStep reaches behavior.CanAct, so without it these tests never exercised
		// ordering at all, and a step locked behind an earlier one would have decided cleanly here.
		Sequencing: &domain.Sequencing{
			ParentField:     action.FieldStepDocument,
			ModeField:       action.FieldDocumentMode,
			SequentialValue: "sequential",
			OrderField:      action.FieldStepSequence,
			StateField:      ids.decision,
			OpenValue:       action.DecisionPending,
		},
		MemberRemovalBlocks: []domain.MemberRemovalBlock{{
			ID: "blk_step_pending", ActorField: action.FieldStepAssignee,
			Condition: expression.Comparison{Field: ids.decision, Op: expression.OpEquals, Value: action.DecisionPending},
			Reason:    "has a pending approval step",
		}},
		Views: []domain.View{
			{ID: "vw_approval_step_table", Name: "All Steps", Type: domain.ViewTable},
			{ID: "vw_step_progress", Name: "Approval Progress", Type: domain.ViewStepper},
		},
		// Mirrors metadata/approval_step.yaml's own signature_placement: block (Stage D). Without it
		// this fixture is a Machine that declares no signature shape at all, and the capture gate
		// correctly refuses to write an image into a Field nothing named -- which is what these tests
		// would otherwise be asserting.
		SignaturePlacement: &domain.SignaturePlacement{
			ImageField: ids.signatureImage,
			PageField:  action.FieldStepSignaturePage,
			XField:     action.FieldStepSignatureX,
			YField:     action.FieldStepSignatureY,
			WidthField: action.FieldStepSignatureWidth,
		},
		// ApplicationID, the workflow binding, the roles: arm and the transitions below all mirror
		// metadata/approval_step.yaml and its Application as of Fase 7, for the same reason the
		// rollup Event does: a fixture that omits them lets these tests pass against a Machine
		// looser than the one that actually runs.
		// internal/conformance.TestApprovalStepPermissionsCarryRoles and
		// TestApprovalStepDeclaresItsTransitions are what keep the real manifest honest; this is
		// what keeps the fixture honest about the manifest.
		//
		// WorkflowEngine/WorkflowRole are what the loader stamps from the Application's own
		// `workflow:` block (2026-09-28), and what action.IsStep reads. Omitting them is how this
		// fixture reads as "some Workspace's Machine called mch_approval_step" rather than as an
		// approval step, which is exactly the distinction the binding exists to make.
		ApplicationID:  "app_document_approval",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleStep,
		Permissions: []domain.Permission{
			{ID: "prm_decide_own_step", Action: domain.ActionDecide, Roles: approverOnly, ActorField: action.FieldStepAssignee},
			{ID: "prm_edit_own_step", Action: domain.ActionEdit, Roles: approverOnly, ActorField: action.FieldStepAssignee},
			{ID: "prm_delete_own_step", Action: domain.ActionDelete, Roles: approverOnly, ActorField: action.FieldStepAssignee},
		},
		// The declared effect, mirroring metadata/approval_step.yaml's own actions: block (Stage B): the
		// decider's name is a companion write, and the decision value itself comes from the Transition.
		// A fixture omitting it would leave these tests passing against a Machine that declares less than
		// the one that runs, which is the same reason the rollup Event and the roles: arm are here.
		ActionEffects: []domain.ActionEffect{{
			Action: domain.ActionDecide,
			Writes: []domain.FieldWrite{{Field: ids.decidedBy, From: domain.WriteFromActorName}},
		}},
		Transitions: []domain.Transition{
			{ID: "trn_step_approve", Name: "Approve", Field: ids.decision, From: action.DecisionPending, To: action.DecisionApproved, Action: domain.ActionDecide},
			{ID: "trn_step_reject", Name: "Reject", Field: ids.decision, From: action.DecisionPending, To: action.DecisionRejected, Action: domain.ActionDecide},
		},
		// Mirrors metadata/approval_step.yaml's own evt_step_decision_rollup -- the Document's
		// status follows its steps by declaration now, not by a hardcoded recompute in the
		// handler, so these tests only exercise the real behaviour if the fixture declares it.
		// internal/conformance.TestApprovalStepDeclaresStatusRollup is what keeps the two in step.
		Events: []domain.Event{{
			ID: "evt_step_signed_document",
			On: ids.decision, WhenEquals: action.DecisionApproved,
			Then: domain.Service{Name: domain.ServiceCompositeSignedDocument, Composite: &domain.Composite{
				ParentField: action.FieldStepDocument,
				SourceField: action.FieldDocumentFile,
				TargetField: action.FieldDocumentSignedFile,
			}},
		}, {
			ID: "evt_step_decision_rollup",
			On: ids.decision,
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

func documentTestMachine(ids approvalIDs) *domain.Machine {
	return &domain.Machine{
		ID:   ids.document,
		Name: "Document",
		// The workflow binding, for the same reason approvalStepTestMachine carries it: the loader
		// stamps these from the Application's own workflow: block, and every handler resolves its
		// Machines by role now (2026-09-28). A fixture declaring only the id is some Workspace's
		// Machine of that name, which is exactly what the engine must *not* act on.
		ApplicationID:  "app_document_approval",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleDocument,
		Fields: []domain.Field{
			{ID: action.FieldDocumentMode, Name: "Mode", Type: domain.FieldTypeStatus, Options: []string{"sequential", "parallel"}},
			{ID: action.FieldDocumentStatus, Name: "Status", Type: domain.FieldTypeStatus, Options: []string{action.DocumentStatusDraft, action.DocumentStatusInReview, action.DocumentStatusApproved, action.DocumentStatusRejected}},
			{ID: action.FieldDocumentFile, Name: "File", Type: domain.FieldTypeFile},
			{ID: action.FieldDocumentSignedFile, Name: "Signed File", Type: domain.FieldTypeFile},
			{ID: action.FieldDocumentSubmittedBy, Name: "Submitted By", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		// The rule blocks metadata/document.yaml declares, and every one of them was missing until the
		// first test of the revise route went looking (2026-09-29). The consequence was specific: with
		// no Transitions this Machine has no StatusField(), so the derivation reviseDocument and
		// continueDocumentWizard read their status Field through returned "" -- and the decide tests
		// still passed, because the rollup Event's own config names fld_status literally. A fixture can
		// be wrong in a way that only a *new* test notices, which is the argument for the fixture gate
		// rather than for trusting the next reader.
		Transitions: []domain.Transition{
			{ID: "trn_document_approved", Name: "Approved", Field: action.FieldDocumentStatus, From: action.DocumentStatusInReview, To: action.DocumentStatusApproved},
			{ID: "trn_document_rejected", Name: "Rejected", Field: action.FieldDocumentStatus, From: action.DocumentStatusInReview, To: action.DocumentStatusRejected},
			{ID: "trn_document_reopened_from_rejected", Name: "Reopened", Field: action.FieldDocumentStatus, From: action.DocumentStatusRejected, To: action.DocumentStatusInReview},
		},
		ActionEffects: []domain.ActionEffect{
			{Action: domain.ActionRevise, Writes: []domain.FieldWrite{{Field: action.FieldDocumentStatus, Value: action.DocumentStatusDraft}}},
			{Action: domain.ActionCreate, Writes: []domain.FieldWrite{{Field: action.FieldDocumentStatus, Value: action.DocumentStatusInReview}}},
		},
		Permissions: []domain.Permission{
			{ID: "prm_create_own_document", Action: domain.ActionCreate, Roles: []string{"approver", "submitter"}, ActorField: action.FieldDocumentSubmittedBy},
			{ID: "prm_edit_document_not_reviewer", Action: domain.ActionEdit, Roles: []string{"approver", "submitter"}},
			{ID: "prm_delete_document_not_reviewer", Action: domain.ActionDelete, Roles: []string{"approver", "submitter"}},
			{ID: "prm_revise_document", Action: domain.ActionRevise, Roles: []string{"approver", "submitter"}},
		},
		Events: []domain.Event{{
			ID: "evt_document_submitted", OnCreate: true,
			Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "\"{fld_title}\" submitted"},
		}},
	}
}

// signatureTestMachine is mch_signature, cast in the engine's optional `signature` role -- the store
// for an approver's own reusable signature image.
//
// The fixture needs it declared because the role is what decides whether "save this signature for
// next time" does anything at all (2026-09-28). A Workspace casting nobody in it keeps the one-time
// image on the step and stores no reusable copy, which is correct behaviour and not what these tests
// are about.
func signatureTestMachine(ids approvalIDs) *domain.Machine {
	return &domain.Machine{
		ID:             ids.signature,
		Name:           "Signature",
		ApplicationID:  "app_document_approval",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleSignature,
		Fields: []domain.Field{
			{ID: action.FieldSignatureOwner, Name: "Owner", Type: domain.FieldTypePerson},
			{ID: action.FieldSignatureImage, Name: "Image", Type: domain.FieldTypeFile},
		},
		// prm_create/edit/delete_own_signature, as metadata/signature.yaml declares them: only an
		// approver may keep a signature, and only their own. Missing until the fixture gate below was
		// written -- the capture path writes through internal/data directly so no test failed, but the
		// fixture was a store anyone could write anything into.
		Permissions: []domain.Permission{
			{ID: "prm_create_own_signature", Action: domain.ActionCreate, Roles: approverOnly, ActorField: action.FieldSignatureOwner},
			{ID: "prm_edit_own_signature", Action: domain.ActionEdit, Roles: approverOnly, ActorField: action.FieldSignatureOwner},
			{ID: "prm_delete_own_signature", Action: domain.ActionDelete, Roles: approverOnly, ActorField: action.FieldSignatureOwner},
		},
		// Mirrors metadata/signature.yaml's own signature_store: block (Stage D) -- which Fields say
		// whose signature this is and where the image lives. A store declaring neither is skipped
		// rather than queried under a guessed name, so a fixture omitting this would make every
		// "already has a saved signature" assertion pass for the wrong reason.
		SignatureStore: &domain.SignatureStore{
			OwnerField: action.FieldSignatureOwner,
			ImageField: action.FieldSignatureImage,
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
	store *data.Store
	files *storage.Store
	cfg   config.Config
	// machines and ids: the Machine set this fixture installed, and the ids it gave them. The ids are
	// a field rather than the action.* constants because approvalIDs below runs the identical flow
	// over *renamed* Machines -- which is the whole point of resolving a Machine by the role its
	// Application casts it in (2026-09-28).
	machines    map[string]*domain.Machine
	ids         approvalIDs
	ctx         context.Context
	workspaceID string
	documentID  string
	assignee    string
	stepID      string
	assignee2   string
	step2ID     string
}

// approvalIDs is what a Workspace happens to call the engine's Machines. templateLibraryIDs is what
// metadata/*.yaml ships; renamedIDs is what an install into a Workspace already using those names
// would have to produce, and nothing in a handler may care which it got.
type approvalIDs struct {
	document, step, signature string
	// decision and decidedBy are the two Fields `decide` writes. They are part of this set since Stage B
	// (2026-09-28): what an Action writes is declared by the Machine now, so a run over Fields named
	// something else is what proves the engine reads the declaration rather than action.Field* constants.
	decision, decidedBy string
	// signatureImage is the Field a captured signature lands in, and it joined this set the same day for
	// the same reason (Stage D): the Machine's own signature_placement: block declares it, so a run over
	// a Field called something else proves the capture reads that declaration.
	signatureImage string
}

func templateLibraryIDs() approvalIDs {
	return approvalIDs{
		document: action.DocumentMachineID, step: action.StepMachineID, signature: action.SignatureMachineID,
		decision: action.FieldStepDecision, decidedBy: action.FieldStepDecidedByName,
		signatureImage: action.FieldStepSignatureImage,
	}
}

// renamedIDs renames the Machines only. Its Fields stay the template's, because the *read* side of this
// engine still names them: action.decisionOf, CanDeleteApprovalStep and the signature compositing all
// look for fld_decision, and Stage B declared what an Action *writes*, not what the engine reads. That
// boundary is real and stated rather than papered over -- renamedFieldIDs below is what exercises the
// half that is declared.
func renamedIDs() approvalIDs {
	return approvalIDs{
		document: "mch_surat", step: "mch_langkah", signature: "mch_ttd",
		decision: action.FieldStepDecision, decidedBy: action.FieldStepDecidedByName,
		signatureImage: action.FieldStepSignatureImage,
	}
}

// renamedFieldIDs renames the two Fields `decide` writes, on top of the renamed Machines. Everything the
// decide route itself does is declared -- the Transition says which Field it moves and to what, the
// actions: block says the companion write -- so this is what Stage B made possible.
func renamedFieldIDs() approvalIDs {
	ids := renamedIDs()
	ids.decision, ids.decidedBy = "fld_putusan", "fld_diputus_oleh"
	// And the signature Field, declared since Stage D -- so this fixture now renames every Field the
	// decide route writes, not only the two Stage B reached.
	ids.signatureImage = "fld_gambar_ttd"
	return ids
}

func newDecideStepTestSetup(t *testing.T, testName string) decideStepTestSetup {
	t.Helper()
	return newDecideStepTestSetupWith(t, testName, templateLibraryIDs())
}

func newDecideStepTestSetupWith(t *testing.T, testName string, ids approvalIDs) decideStepTestSetup {
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
	docKey, err := files.Save(ids.document, action.FieldDocumentFile, "contract.pdf", bytes.NewReader(testDocumentPDFBytes(t)))
	if err != nil {
		t.Fatalf("Save(document PDF): %v", err)
	}

	// The status the Machine's own `create` effect declares, not a literal and not nothing. Left empty
	// until 2026-09-29, when the revise tests found it: a Document with no status has no *current*
	// state, so any declared transition refuses to move it and machine.StatusField() reads blank. In
	// production the wizard never makes one -- create's effect fills it -- so an empty status was a
	// state only this fixture could produce.
	docValues := map[string]any{
		action.FieldDocumentMode: "sequential",
		action.FieldDocumentFile: docKey,
	}
	if effect, ok := documentTestMachine(ids).EffectFor(domain.ActionCreate); ok {
		for _, wr := range effect.Writes {
			if wr.Value != "" {
				docValues[wr.Field] = wr.Value
			}
		}
	}
	document, err := store.CreateRecord(wsCtx, ids.document, docValues)
	if err != nil {
		t.Fatalf("CreateRecord(document): %v", err)
	}

	step, err := store.CreateRecord(wsCtx, ids.step, map[string]any{
		action.FieldStepDocument:       document.ID,
		action.FieldStepSequence:       float64(1),
		action.FieldStepAssignee:       assignee.ID,
		ids.decision:                   action.DecisionPending,
		action.FieldStepSignatureX:     float64(50),
		action.FieldStepSignatureY:     float64(50),
		action.FieldStepSignaturePage:  float64(1),
		action.FieldStepSignatureWidth: float64(20),
	})
	if err != nil {
		t.Fatalf("CreateRecord(step): %v", err)
	}
	step2, err := store.CreateRecord(wsCtx, ids.step, map[string]any{
		action.FieldStepDocument:       document.ID,
		action.FieldStepSequence:       float64(2),
		action.FieldStepAssignee:       assignee2.ID,
		ids.decision:                   action.DecisionPending,
		action.FieldStepSignatureX:     float64(70),
		action.FieldStepSignatureY:     float64(50),
		action.FieldStepSignaturePage:  float64(1),
		action.FieldStepSignatureWidth: float64(20),
	})
	if err != nil {
		t.Fatalf("CreateRecord(step2): %v", err)
	}

	machines := map[string]*domain.Machine{
		ids.document:  documentTestMachine(ids),
		ids.step:      approvalStepTestMachine(ids),
		ids.signature: signatureTestMachine(ids),
	}
	// The installed Workspace goes on the setup's own ctx too, not only on each request's: the
	// assertions call the same role-resolving helpers the handlers do (hasSavedSignature reads which
	// Machine this Application casts as its signature store, 2026-09-28), so a bare
	// WithWorkspaceScope ctx would make them answer "no such feature" rather than test it.
	setupCtx := rendering.WithCurrentWorkspace(wsCtx, testWorkspaceFor(machines), "Test Workspace", false)

	return decideStepTestSetup{
		store:       store,
		files:       files,
		cfg:         cfg,
		machines:    machines,
		ids:         ids,
		ctx:         setupCtx,
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
	req := httptest.NewRequest(http.MethodPost, "/machines/"+s.ids.step+"/records/"+stepID+"/decide", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), s.workspaceID), testWorkspaceFor(s.machines), "Test Workspace", false))
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, s.cfg, actorID, 0)})

	r := chi.NewRouter()
	r.Post("/machines/{machineID}/records/{id}/decide", decideStep(s.store, s.files, mail.LogMailer{}, s.cfg))
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

	step, err := s.store.GetRecord(s.ctx, s.ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[s.ids.decision]; got != action.DecisionPending {
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

	step, err := s.store.GetRecord(s.ctx, s.ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[s.ids.decision]; got != action.DecisionApproved {
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

	if _, err := s.store.CreateRecord(s.ctx, s.ids.signature, map[string]any{
		action.FieldSignatureOwner: s.assignee,
		action.FieldSignatureImage: "sig_test/fld_image/pre-existing.png",
	}); err != nil {
		t.Fatalf("CreateRecord(mch_signature): %v", err)
	}

	rec := postDecide(t, s, action.DecisionApproved, nil)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decideStep(approve, already saved) status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, s.ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[s.ids.decision]; got != action.DecisionApproved {
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

	document, err := s.store.GetRecord(s.ctx, s.ids.document, s.documentID)
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
	document2, err := s.store.GetRecord(s.ctx, s.ids.document, s.documentID)
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

// approverOnly mirrors the roles: arm on all three of mch_approval_step's Permissions. It was
// [approver, reviewer] until the owner settled the vocabulary (2026-09-21): a reviewer may only
// look. internal/conformance.TestApprovalStepPermissionsCarryRoles keeps the real manifest
// honest; this keeps the fixture honest about the manifest.
var approverOnly = []string{"approver"}

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

	// Holding a *different* real role is still not enough, which is the owner's decision of
	// 2026-09-21 seen from the route: a reviewer may only look.
	if err := s.store.SetMemberAppRole(s.ctx, s.workspaceID, s.assignee, "app_document_approval", "reviewer"); err != nil {
		t.Fatalf("grant reviewer: %v", err)
	}
	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("decideStep holding reviewer: status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// And granting the role that does carry it is all it takes -- which is what makes the
	// Authorization Matrix an administrative screen rather than a report.
	if err := s.store.SetMemberAppRole(s.ctx, s.workspaceID, s.assignee, "app_document_approval", "approver"); err != nil {
		t.Fatalf("grant approver: %v", err)
	}
	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code >= 400 {
		t.Fatalf("decideStep holding approver: status = %d, want success; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDecideStep_refusesRedecidingADecidedStep is the declared state model at the route, and it
// covers the hole neither existing guard did: sequencing only ever locked a step behind an
// *earlier* one, so on a parallel Document an already-approved step could be decided again and
// re-run the rollup onto its Document. No transition leaves `approved`, so this is 422 now.
func TestDecideStep_refusesRedecidingADecidedStep(t *testing.T) {
	s := newDecideStepTestSetup(t, "decide_step_redecide")

	step, err := s.store.GetRecord(s.ctx, s.ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	step.Values[s.ids.decision] = action.DecisionApproved
	if _, err := s.store.UpdateRecord(s.ctx, s.ids.step, s.stepID, step.Values); err != nil {
		t.Fatalf("UpdateRecord(step): %v", err)
	}

	if rec := postDecide(t, s, action.DecisionRejected, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("decideStep re-deciding an approved step: status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
	after, err := s.store.GetRecord(s.ctx, s.ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step) after: %v", err)
	}
	if got := after.Values[s.ids.decision]; got != action.DecisionApproved {
		t.Errorf("step decision = %v, want still approved -- a decision is final", got)
	}
}

// TestDecideStep_worksOverRenamedMachines is the assertion the whole "resolve by role" slice exists
// for, and the one no test could make before it (2026-09-28).
//
// Every Machine here has a name this repo has never used -- mch_surat, mch_langkah, mch_ttd -- and
// the flow is the real one, driven through the real handler: a sequential two-step approval where
// step 2 is locked behind step 1, approve step 1, watch the Document's own status follow through the
// declared rollup, and confirm the redirect names the Workspace's *own* document Machine.
//
// It matters because an install into a Workspace that already uses `mch_document` has to rename, and
// until this slice every handler asked for the old id by name: the copy would load, bind, engage --
// and then read nothing. `machines[action.StepMachineID]` is legal Go against any Workspace, which is
// exactly why only a test running the flow under different names can tell the difference.
//
// Sequencing is deliberately not asserted here: this fixture's step Machine declares none (the
// sequencing rule has its own tests against the real manifest), and adding it would change what every
// other test in this file exercises.
func TestDecideStep_worksOverRenamedMachines(t *testing.T) {
	ids := renamedIDs()
	s := newDecideStepTestSetupWith(t, "decide_step_renamed_machines", ids)

	rec := postDecide(t, s, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decide over renamed machines: status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}
	// The redirect is built from the role, so it names this Workspace's own document Machine. A
	// handler still holding action.DocumentMachineID would send the browser to /machines/mch_document.
	if got := rec.Header().Get("Location"); got != "/machines/"+ids.document+"/records/"+s.documentID {
		t.Errorf("redirect = %q, want it to name this Workspace's own document machine %q", got, ids.document)
	}

	step, err := s.store.GetRecord(s.ctx, ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[s.ids.decision]; got != action.DecisionApproved {
		t.Errorf("step fld_decision = %v, want approved -- the engine wrote to the renamed Machine", got)
	}
	if name, _ := step.Values[s.ids.decidedBy].(string); name == "" {
		t.Error("step fld_decided_by_name is empty, want the decider's name snapshotted")
	}

	// The declared rollup fired too: one of two steps approved leaves the Document in review, which
	// is mch_approval_step's own evt_step_decision_rollup acting on a parent it reached by relation,
	// not by name.
	document, err := s.store.GetRecord(s.ctx, ids.document, s.documentID)
	if err != nil {
		t.Fatalf("GetRecord(document): %v", err)
	}
	if got := document.Values[action.FieldDocumentStatus]; got != action.DocumentStatusInReview {
		t.Errorf("document fld_status = %v, want in_review after the first of two approvals", got)
	}

	// And the signed PDF was composited onto the renamed Document's own record.
	if key, _ := document.Values[action.FieldDocumentSignedFile].(string); key == "" {
		t.Error("document fld_signed_file is empty, want the composited PDF -- signDocument resolves both Machines by role")
	}
}

// TestDecideStep_writesTheFieldsTheMachineDeclares is Stage B's own proof, and the assertion no test
// could make before 2026-09-28: the decide route writes a step whose decision Field is called
// fld_putusan and whose decider-name Field is called fld_diputus_oleh, because the Machine says so.
//
// Both were literals in internal/web until this change -- `step.Values[action.FieldStepDecision] =
// decision` -- so an Application binding the approval engine had to name its Fields exactly as Document
// Approval does or the engine wrote nothing it could read. What is declared here is the whole effect:
// the Transition says which Field `decide` moves and which values are legal (so the route no longer
// compares against the literals "approved"/"rejected" either), and the actions: block says the companion.
//
// **What this deliberately does not assert** is the signed PDF. The compositing side still reads
// fld_decision by id (action.decisionOf, StampFor) -- Stage B declared what an Action writes, not what
// the engine reads, and claiming more here would make this test say something the change did not do.
func TestDecideStep_writesTheFieldsTheMachineDeclares(t *testing.T) {
	ids := renamedFieldIDs()
	s := newDecideStepTestSetupWith(t, "decide_step_renamed_fields", ids)

	rec := postDecide(t, s, action.DecisionApproved, map[string]string{
		"signature_image": testSignaturePNGDataURL(t),
	})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("decide over renamed fields: status = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}

	step, err := s.store.GetRecord(s.ctx, ids.step, s.stepID)
	if err != nil {
		t.Fatalf("GetRecord(step): %v", err)
	}
	if got := step.Values[ids.decision]; got != action.DecisionApproved {
		t.Errorf("%s = %v, want approved -- the Field the declared Transition names", ids.decision, got)
	}
	if name, _ := step.Values[ids.decidedBy].(string); name == "" {
		t.Errorf("%s is empty, want the decider's name -- the companion write the actions: block declares", ids.decidedBy)
	}
	// The captured signature lands in the Field signature_placement: names (Stage D) -- the third
	// Field this route writes, and the one Stage B could not reach because it is not an Action effect
	// but a capture the handler performs before the decision is applied.
	if key, _ := step.Values[ids.signatureImage].(string); key == "" {
		t.Errorf("%s is empty, want the captured image's storage key -- the Field signature_placement: declares", ids.signatureImage)
	}
	// And nothing was written under the template's own Field ids, which is what would happen if any of
	// this were still naming them.
	for _, id := range []string{action.FieldStepDecision, action.FieldStepSignatureImage} {
		if _, set := step.Values[id]; set {
			t.Errorf("something wrote %s on a Machine that does not declare it", id)
		}
	}
}

// TestFixturesMirrorTheRealMachines closes a hole that has now been found by hand twice and by a gate
// never: a test fixture that declares *less* than the Machine it mirrors does not fail. It passes,
// against a Machine looser than the one that runs, and the test goes on reading as though it covered
// the rule.
//
// The first occurrence is recorded in composition's own stepMachineForTest comment -- its `decide`
// Permission was missing for a whole phase and "nothing noticed", because the screen under test ANDed
// an equivalent check in Go. Stage D (2026-09-28) found four more in one afternoon, including this
// file's two: neither declared signature_placement:/signature_store:, so every signature assertion
// here was about to start passing for the wrong reason.
//
// It compares **presence, not equality** (domain.Machine.MissingBlocksFrom). These fixtures are
// deliberately smaller than the real Machines, and deliberately renamed in the tests that prove this
// runtime does not depend on Document Approval's own names -- what must not differ is which blocks
// exist at all. Checked against templateLibraryIDs, since a renamed fixture is the same shape under
// other names.
func TestFixturesMirrorTheRealMachines(t *testing.T) {
	real, _ := loadRealMachines(t)
	ids := templateLibraryIDs()

	for _, c := range []struct {
		name    string
		id      string
		fixture *domain.Machine
	}{
		{"approvalStepTestMachine", ids.step, approvalStepTestMachine(ids)},
		{"documentTestMachine", ids.document, documentTestMachine(ids)},
		{"signatureTestMachine", ids.signature, signatureTestMachine(ids)},
	} {
		want, ok := real[c.id]
		if !ok {
			t.Fatalf("%s mirrors %s, which the default Workspace no longer installs -- the fixture is describing a Machine that is gone", c.name, c.id)
		}
		if missing := c.fixture.MissingBlocksFrom(want); len(missing) > 0 {
			t.Errorf("%s declares no %v, which the real %s does -- every test using this fixture is currently passing against a Machine looser than the one that runs. Add the block (mirroring metadata/workspaces/default/), do not delete this check",
				c.name, missing, c.id)
		}
	}
}
