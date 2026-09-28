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

// TestApprovalFlowTemplate_overMachinesThatNameTheirFieldsDifferently is Stage E2's own property
// (2026-09-29), and it is the assertion the stage exists to make possible: a saved approval flow works
// over template Machines whose Fields are called anything, because both halves read the Machines' own
// flow_template:/flow_template_step: declarations.
//
// Before those blocks, internal/web named fld_document_type, fld_mode, fld_template, fld_sequence,
// fld_step_name, fld_assignee, fld_approver_type and fld_approver_group itself -- so Stage A's promise
// (cast any Machine in the role, under any name) was only half true: the roles resolved and the writes
// went to Fields the Machine might not have.
//
// Exercised through saveApprovalFlowTemplate and findApprovalFlowTemplate directly rather than the
// wizard, because those two are the whole read/write pair and the wizard adds a Document it does not
// need here.
func TestApprovalFlowTemplate_overMachinesThatNameTheirFieldsDifferently(t *testing.T) {
	store, wsCtx, _, _, submitter, group := approvalFlowTemplateTestFixture(t,
		"Renamed Flow Template", "renamed-flow-template-workspace", "renamed_flow_template@example.com")

	templateM, stepM := renamedFlowTemplateMachines()
	ctx := rendering.WithCurrentWorkspace(wsCtx, workspaceCasting(templateM, stepM), "Renamed Flow Template", false)

	rows := []stepInput{
		{name: "Legal", assignee: submitter.ID},
		{name: "Finance", approverType: domain.ActorKindGroup, approverGroup: group.ID},
	}
	if err := saveApprovalFlowTemplate(ctx, store, templateM, stepM, "Kontrak", "sequential", rows); err != nil {
		t.Fatalf("saveApprovalFlowTemplate over renamed Machines: %v", err)
	}

	template, steps, err := findApprovalFlowTemplate(ctx, store, "Kontrak")
	if err != nil {
		t.Fatalf("findApprovalFlowTemplate: %v", err)
	}
	if template == nil {
		t.Fatal("no saved flow was found -- the key Field this Machine declares was not the one written")
	}
	if got := toDisplayString(template.Values["fld_cara"]); got != "sequential" {
		t.Errorf("fld_cara = %q, want sequential -- the mode Field this Machine declares", got)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d saved rows, want 2", len(steps))
	}
	// In declared order, by the Field this Machine calls its order -- not fld_sequence.
	if got := toDisplayString(steps[0].Values["fld_nama"]); got != "Legal" {
		t.Errorf("first row fld_nama = %q, want Legal", got)
	}
	if got := toDisplayString(steps[0].Values["fld_petugas"]); got != submitter.ID {
		t.Errorf("first row fld_petugas = %q, want the submitter", got)
	}
	if got := toDisplayString(steps[1].Values["fld_grup"]); got != group.ID {
		t.Errorf("second row fld_grup = %q, want the Group", got)
	}
	// And nothing was written under the template library's own ids, which is what would happen if any
	// of this were still naming them.
	for _, id := range []string{action.FieldTemplateDocumentType, action.FieldTemplateMode} {
		if _, set := template.Values[id]; set {
			t.Errorf("something wrote %s on a template Machine that does not declare it", id)
		}
	}
	for _, id := range []string{action.FieldTemplateStepSequence, action.FieldTemplateStepAssignee, action.FieldStepName} {
		if _, set := steps[0].Values[id]; set {
			t.Errorf("something wrote %s on a template step Machine that does not declare it", id)
		}
	}
}

// TestApprovalFlowTemplate_undeclaredShapeIsRefusedNotGuessed holds the other half of the "empty means
// undeclared, never assume the usual name" contract: a Machine cast in the role but declaring no shape
// must refuse, not fall back to internal/action's constants.
//
// It is the assertion that would have caught Stage E1's near-miss before a probe did -- passing the
// derived (and entirely empty) EngineFields into the row writer would have written four values under
// the empty key and saved a flow with no approvers.
func TestApprovalFlowTemplate_undeclaredShapeIsRefusedNotGuessed(t *testing.T) {
	store, wsCtx, _, _, submitter, _ := approvalFlowTemplateTestFixture(t,
		"Undeclared Flow Template", "undeclared-flow-template-workspace", "undeclared_flow_template@example.com")

	templateM, stepM := renamedFlowTemplateMachines()
	templateM.FlowTemplate, stepM.FlowTemplateStep = nil, nil
	ctx := rendering.WithCurrentWorkspace(wsCtx, workspaceCasting(templateM, stepM), "Undeclared", false)

	err := saveApprovalFlowTemplate(ctx, store, templateM, stepM, "Kontrak", "sequential",
		[]stepInput{{name: "Legal", assignee: submitter.ID}})
	if err == nil {
		t.Error("saving a flow over Machines that declare no shape succeeded -- it must refuse rather than guess Field ids")
	}

	// And nothing was created: the refusal happens before any write, so there is no half-saved flow.
	records, err := store.ListRecords(ctx, templateM.ID)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d template record(s) after a refused save, want 0", len(records))
	}
}

// renamedFlowTemplateMachines is the saved-flow pair under names this repo has never used, declaring
// its shape through the Stage E2 blocks. The ids are Indonesian for the same reason
// TestDecideStep_worksOverRenamedMachines' are: a name this codebase cannot have hardcoded anywhere.
func renamedFlowTemplateMachines() (*domain.Machine, *domain.Machine) {
	template := &domain.Machine{
		ID:             "mch_pola_persetujuan",
		Name:           "Pola Persetujuan",
		ApplicationID:  "app_document_approval",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleFlowTemplate,
		Fields: []domain.Field{
			{ID: "fld_jenis", Name: "Jenis", Type: domain.FieldTypeStatus, Required: true, Options: []string{"Kontrak", "Tagihan"}},
			{ID: "fld_cara", Name: "Cara", Type: domain.FieldTypeStatus, Required: true, Options: []string{"sequential", "parallel"}},
		},
		FlowTemplate: &domain.FlowTemplate{KeyField: "fld_jenis", ModeField: "fld_cara"},
	}
	step := &domain.Machine{
		ID:             "mch_baris_pola",
		Name:           "Baris Pola",
		ApplicationID:  "app_document_approval",
		WorkflowEngine: domain.WorkflowEngineDocumentApproval,
		WorkflowRole:   domain.WorkflowRoleFlowTemplateStep,
		Fields: []domain.Field{
			{ID: "fld_induk", Name: "Induk", Type: domain.FieldTypeRelation, RelatedMachine: "mch_pola_persetujuan"},
			{ID: "fld_urutan", Name: "Urutan", Type: domain.FieldTypeNumber, Required: true},
			{ID: "fld_nama", Name: "Nama", Type: domain.FieldTypeText},
			{ID: "fld_jenis_petugas", Name: "Jenis Petugas", Type: domain.FieldTypeStatus, Options: []string{"User", "Group"}},
			{ID: "fld_petugas", Name: "Petugas", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
			{ID: "fld_grup", Name: "Grup", Type: domain.FieldTypeGroup},
		},
		FlowTemplateStep: &domain.FlowTemplateStep{
			TemplateField:   "fld_induk",
			OrderField:      "fld_urutan",
			NameField:       "fld_nama",
			ActorField:      "fld_petugas",
			ActorTypeField:  "fld_jenis_petugas",
			ActorGroupField: "fld_grup",
		},
	}
	return template, step
}

// workspaceCasting is a Workspace installing exactly these Machines, so approvalMachine(ctx, role)
// resolves them the way a real request's does.
func workspaceCasting(machines ...*domain.Machine) domain.Workspace {
	ws := domain.Workspace{Slug: "renamed"}
	for _, m := range machines {
		ws.MachineIDs = append(ws.MachineIDs, m.ID)
		ws.Machines = append(ws.Machines, m)
	}
	return ws
}
