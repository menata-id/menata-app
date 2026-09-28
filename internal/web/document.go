package web

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/pdf"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showDocumentSubmit is Case 3's submission wizard (ROADMAP.md Phase 15 Step 6) -- Document
// details + Approval mode + a dynamic, flat approver picker, all on one screen.
//
// wizardOptions is everything both wizard renderings need beyond chrome: the two Fields whose
// declared options the form must offer (never hardcoded lists), and the pickers' own choices.
//
// It exists because the full page and the "+ Add approver" fragment must offer the *identical*
// approver row -- the drift that would otherwise appear as a new row silently missing a Group the
// first row had.
type wizardOptions struct {
	documentType domain.Field
	mode         domain.Field
	approvers    rendering.RelationOptions
	groups       rendering.GroupOptions
}

// readWizardOptions resolves them once.
//
// The approver list comes from composition.Loader.RelationOptions rather than a hand-built option
// list off mch_user records, which is what let documentsubmit.templ leave the projection ratchet:
// the Loader already resolves a reference Field's target records to {ID, Label} using the target's
// first Field, and for mch_user that first Field is fld_name -- exactly what the template used to
// read itself. The group list is keyed on mch_approval_step, not mch_document: GroupOptions
// short-circuits on a Machine declaring no group Field, and only the step Machine declares one.
func readWizardOptions(req *http.Request, machines map[string]*domain.Machine, store *data.Store) (wizardOptions, error) {
	stepMachine := machineForStep(req.Context())
	ld := composition.NewLoader(store, machines)
	approvers, err := ld.RelationOptions(req.Context(), stepMachine)
	if err != nil {
		return wizardOptions{}, err
	}
	// Narrowed to people who could actually decide a step (composition.ApproverOptions). Without
	// it a submitter can name anyone in the Workspace, including someone holding no role in this
	// Application at all -- producing a step assigned to a real person and decidable by nobody,
	// with no error anywhere.
	// Through the Loader, not store.ListMembers: GroupOptions below wants this Workspace's Groups
	// too, and reading them by both routes is what put this screen on the query-sweep ratchet.
	members, err := ld.Members(req.Context())
	if err != nil {
		return wizardOptions{}, err
	}
	approvers = composition.ApproverOptions(approvers, stepMachine, members)
	groups, err := ld.GroupOptions(req.Context(), stepMachine)
	if err != nil {
		return wizardOptions{}, err
	}
	docMachine := machineForDocument(req.Context())
	documentType, _ := docMachine.FieldByID("fld_document_type")
	mode, _ := docMachine.FieldByID(action.FieldDocumentMode)
	return wizardOptions{documentType: documentType, mode: mode, approvers: approvers, groups: groups}, nil
}

// showDocumentSubmit serves board 08 (ui-sample/case-03-flow1/08-submit-document.html).
//
// Every option list on this screen comes from metadata: fld_document_type's since the Field was
// added, and fld_mode's since Fase 6c-2 -- it was the last hardcoded pair in the wizard, two
// <input type="radio" value="sequential|parallel"> literals sitting two sections below a select
// that already did it correctly.
func showDocumentSubmit(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		machines := machinesFor(ctx)
		opts, err := readWizardOptions(req, machines, store)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)
		_, switchHref := viewerWorkspaceContext(ctx, store, actor.ID)
		render(ctx, w, rendering.DocumentSubmitPage(opts.documentType, opts.mode, opts.approvers, opts.groups,
			chrome.WorkspaceName, chrome.Viewer(), switchHref, rendering.DraftPrefill{}))
	}
}

// newApproverRow serves the wizard's own "+ Add approver" HTMX fragment -- a fresh
// rendering.ApproverRow with the same real options as the wizard's initial row, never fabricated
// data and never a different set.
func newApproverRow(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		opts, err := readWizardOptions(req, machines, store)
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.ApproverRow(opts.approvers, opts.groups, rendering.StepPrefill{}))
	}
}

// submitDocumentWizard is the wizard's own single POST: creates one mch_document and its own
// mch_approval_step records together. Hardcoded to mch_document/mch_approval_step, same posture
// as internal/action and every other Case 3-specific screen -- not a generic multi-Machine
// composite-create mechanism, since no second case needs one yet.
//
// Two intents share this one handler (Flow 2 gap study Tahap 4, 2026-09-25): the wizard's own
// "intent" form field is "submit" (the pre-existing behaviour, default when the field is missing
// so an old cached page keeps working) or "draft". A draft skips hasApprover entirely -- it hasn't
// gone anywhere yet, so it may have zero approvers -- and creates no Approval Steps, which is what
// makes reviewHref/SplitDrafts able to tell a Draft apart from the pre-existing "zero-step
// Document the generic form can make" edge case: a Draft is the *only* status guaranteed to have
// zero steps by construction.
func submitDocumentWizard(store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		docMachine, ok := requireMachineForDocument(w, req.Context())
		if !ok {
			return
		}

		values, _, ok := submittedValues(w, req, docMachine, files)
		if !ok {
			return
		}
		rows, problem := parseStepInputs(req)
		if problem != "" {
			http.Error(w, problem, http.StatusUnprocessableEntity)
			return
		}
		isDraft := req.FormValue("intent") == "draft"
		if !isDraft && !hasApprover(rows) {
			// Without this, createApprovalSteps below silently skips every empty slot and
			// returns success -- a Document would be created with zero Approval Steps, and
			// nothing could ever decide it. Checked before CreateRecord so a rejected submission
			// never creates an orphaned Document that would then need cleaning up.
			http.Error(w, "at least one approver is required", http.StatusUnprocessableEntity)
			return
		}
		actor := currentActor(req, store, cfg)
		if !applySubmissionEffect(w, docMachine, values, actor.ID, isDraft) {
			return
		}
		if !allowsRecordCreate(w, docMachine, values, actor) {
			return
		}
		if !validRecord(w, req, store, docMachine, values) {
			return
		}

		document, err := store.CreateRecord(req.Context(), docMachine.ID, values)
		if err != nil {
			serverError(w, err)
			return
		}

		if isDraft {
			logActivity(req.Context(), store, docMachine.ID, document.ID, actor.ID,
				fmt.Sprintf("%q saved as draft", toDisplayString(document.Values["fld_title"])))
			redirectTo(w, req, navRouteByID(req.Context(), "nav_my_documents"))
			return
		}

		if !createApprovalSteps(w, req, store, machineForStep(req.Context()), document.ID, rows) {
			return
		}
		if req.FormValue("save_as_default_flow") != "" {
			saveDefaultApprovalFlow(req.Context(), store, values, rows)
		}

		logActivity(req.Context(), store, docMachine.ID, document.ID, actor.ID,
			fmt.Sprintf("%q submitted", toDisplayString(document.Values["fld_title"])))

		redirectTo(w, req, fmt.Sprintf("/machines/%s/records/%s/signature-placement", docMachine.ID, document.ID))
	}
}

// applySubmissionEffect writes what a submission sets beyond the form's own values: the status it
// starts in and who submitted it, both declared by mch_document's own actions: block (Stage B,
// 2026-09-28) rather than typed here. Submitted-by is stamped from the session rather than accepted
// from the form (2026-09-21), which is what prm_create_own_document checks.
//
// "Save as draft" is the one thing the declaration cannot say: one Action has one effect, and which of
// the two outcomes this submission is, is what the button chose -- so it overrides afterwards, the same
// way an INSERT's explicit column value outranks a SQL DEFAULT. continueDocumentWizard carries the same
// shape a second time, for the same reason.
//
// Extracted from submitDocumentWizard rather than inlined: that handler sits at the 70-line budget
// (internal/conformance.TestHandlersStaySmall), and the remedy that gate asks for is moving a
// derivation out, never raising the number.
func applySubmissionEffect(w http.ResponseWriter, docMachine *domain.Machine, values map[string]any, actorID string, isDraft bool) bool {
	if _, err := action.ApplyEffect(docMachine, domain.ActionCreate, values, action.EffectInput{ActorID: actorID}); err != nil {
		serverError(w, err)
		return false
	}
	if isDraft {
		values[action.FieldDocumentStatus] = action.DocumentStatusDraft
	}
	return true
}

// saveDefaultApprovalFlow is CAP-V28's own call site shared by submitDocumentWizard and
// continueDocumentWizard (ROADMAP.md, 2026-09-27): both build an identical values map (this
// wizard's own four Fields) and an identical rows slice before this point, so one helper reads the
// Document Type/Mode the submission just used and hands them to saveApprovalFlowTemplate. Its own
// error is logged, never surfaced -- see saveApprovalFlowTemplate's doc comment for why.
func saveDefaultApprovalFlow(ctx context.Context, store *data.Store, values map[string]any, rows []stepInput) {
	// flow_template/flow_template_step are optional roles (domain.WorkflowEngineSpec): an approval
	// Application may cast neither, and then there is nowhere to save a default flow. This guard is
	// also a real bug fix -- saveApprovalFlowTemplate dereferences both Machines, so before the roles
	// existed a Workspace that installed Document Approval without them panicked on submit.
	template, templateStep := approvalMachine(ctx, domain.WorkflowRoleFlowTemplate), approvalMachine(ctx, domain.WorkflowRoleFlowTemplateStep)
	if template == nil || templateStep == nil {
		return
	}
	documentType := toDisplayString(values[action.FieldTemplateDocumentType])
	mode := toDisplayString(values[action.FieldDocumentMode])
	if err := saveApprovalFlowTemplate(ctx, store, template, templateStep, documentType, mode, rows); err != nil {
		log.Printf("save default approval flow for document type %q: %v", documentType, err)
	}
}

// navRouteByID mirrors rendering.routeByID's own lookup (internal/web cannot call it directly --
// unexported, and it reads ctx through a different package's own accessor) for the one place a
// handler needs to redirect to a declared nav route instead of retyping it
// (internal/conformance.TestHandlersHaveNoHardcodedApplicationRoute; CLAUDE.md "Where a
// metadata-derived value belongs"). Panics on an unknown id, matching routeByID's own posture -- a
// typo here is a programmer error, cheaper to find at first use than as a silently broken redirect.
func navRouteByID(ctx context.Context, id string) string {
	for _, app := range rendering.CurrentWorkspace(ctx).Applications {
		for _, item := range app.AllNavigation {
			if item.ID == id {
				return item.Route
			}
		}
	}
	panic("web: no navigation item with id " + id)
}

// showDocumentContinue reopens the submit wizard for a Draft (Flow 2 gap study Tahap 4,
// 2026-09-25): "Continue" on My Documents' own Drafts section. Prefilled from the draft's own
// stored values; approver rows start empty exactly as a fresh wizard's do, because a Draft is
// guaranteed to carry none (submitDocumentWizard's own draft branch creates no Approval Steps, and
// reviseDocument deletes any that existed before moving a Document back to Draft).
func showDocumentContinue(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsDocument(machine) {
			http.Error(w, "this machine has no continue-submit screen", http.StatusNotFound)
			return
		}
		ctx := req.Context()
		id := chi.URLParam(req, "id")
		document, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}
		if ok, reason := action.CanContinueDraft(toDisplayString(document.Values[action.FieldDocumentStatus])); !ok {
			http.Error(w, reason, http.StatusUnprocessableEntity)
			return
		}

		opts, err := readWizardOptions(req, machines, store)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)
		_, switchHref := viewerWorkspaceContext(ctx, store, actor.ID)
		draft := rendering.DraftPrefill{
			ID:           document.ID,
			Title:        toDisplayString(document.Values["fld_title"]),
			DocumentType: toDisplayString(document.Values["fld_document_type"]),
			Mode:         toDisplayString(document.Values[action.FieldDocumentMode]),
			FileName:     storage.DisplayName(toDisplayString(document.Values["fld_file"])),
		}
		render(ctx, w, rendering.DocumentSubmitPage(opts.documentType, opts.mode, opts.approvers, opts.groups,
			chrome.WorkspaceName, chrome.Viewer(), switchHref, draft))
	}
}

// continueDocumentWizard is showDocumentContinue's own POST: finishes a Draft into review, reusing
// this wizard's own approver-row handling (parseStepInputs/createApprovalSteps) exactly as
// submitDocumentWizard's create path does, but as an UpdateRecord on the existing draft rather than
// a CreateRecord.
//
// One fetch (existing, below) feeds both recordEditAllowed's permission check and the Field
// carry-forward, rather than reusing record.go's allowsRecordEdit/carryForwardFiles as separate
// calls that would each fetch the record again on their own: this bespoke wizard form only carries
// four of mch_document's eight Fields (title/type/file/mode), so whatever it doesn't mention --
// fld_due_date, fld_submitted_by, and fld_signed_file -- has to be carried forward from that one
// fetch or a plain UpdateRecord (a whole-record replace, not a merge) would silently erase it. This
// is exactly the "generic route rewrites a whole record" trap signatureplacement's own
// carry-forward history already found once (ROADMAP.md) -- the loop below is
// carryForwardExistingFiles' own reasoning generalized to every Field, not just file ones, and
// Machine-agnostic for the same reason that function is: a Field added to mch_document later is
// carried forward automatically rather than needing this list remembered a second time.
//
// draft -> in_review is not behavior.CheckTransitions either, and deliberately so -- see
// action.CanContinueDraft's own doc comment for why mch_document's fld_status may declare no
// person-performed Transition at all.
//
// Scope, stated once rather than left to be discovered: continuing a draft only ever finalizes it
// into in_review here. There is no "save this edit, stay draft" loop on this form -- revisiting a
// draft without submitting it is just leaving the page.
func continueDocumentWizard(store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsDocument(machine) {
			http.Error(w, "this machine has no continue-submit screen", http.StatusNotFound)
			return
		}
		ctx := req.Context()
		id := chi.URLParam(req, "id")
		existing, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)
		if !recordEditAllowed(machine, existing.Values, actor) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		// Not behavior.CheckTransitions: internal/conformance.TestDocumentStatusIsDerivedNotSettable
		// holds that no mch_document transition on fld_status may declare an Action (draft <->
		// in_review included), so this move is gated by a plain business-state check instead
		// (action.CanContinueDraft), the same posture action.CanDeleteDocument already takes for
		// delete.
		if ok, reason := action.CanContinueDraft(toDisplayString(existing.Values[action.FieldDocumentStatus])); !ok {
			http.Error(w, reason, http.StatusUnprocessableEntity)
			return
		}

		rows, problem := parseStepInputs(req)
		if problem != "" {
			http.Error(w, problem, http.StatusUnprocessableEntity)
			return
		}
		if !hasApprover(rows) {
			http.Error(w, "at least one approver is required", http.StatusUnprocessableEntity)
			return
		}

		values, _, ok := submittedValues(w, req, machine, files)
		if !ok {
			return
		}
		carryForwardMissingFields(machine, values, existing.Values)
		// Still a literal, deliberately: "submit this draft" shares the `edit` Action with every
		// ordinary field change, so declaring it as edit's effect would set in_review on every edit.
		// See mch_document's own actions: block, and ROADMAP.md's Stage B note on what it leaves.
		values[action.FieldDocumentStatus] = action.DocumentStatusInReview
		if !passesWriteGuards(w, req, store, machines, machine, id, values) {
			return
		}

		document, err := store.UpdateRecord(ctx, machine.ID, id, values)
		if err != nil {
			recordError(w, err)
			return
		}
		if !createApprovalSteps(w, req, store, machineForStep(req.Context()), document.ID, rows) {
			return
		}
		if req.FormValue("save_as_default_flow") != "" {
			saveDefaultApprovalFlow(ctx, store, values, rows)
		}

		logActivity(ctx, store, machine.ID, document.ID, actor.ID,
			fmt.Sprintf("%q submitted", toDisplayString(document.Values["fld_title"])))

		redirectTo(w, req, fmt.Sprintf("/machines/%s/records/%s/signature-placement", machine.ID, document.ID))
	}
}

// carryForwardMissingFields fills every one of machine's own Fields that values doesn't mention
// with existing's already-stored value -- continueDocumentWizard's own carry-forward step,
// factored out so its handler stays under internal/conformance's TestHandlersStaySmall budget.
// See continueDocumentWizard's doc comment for why this exists at all: its bespoke wizard form
// only carries four of mch_document's eight Fields, so whatever it doesn't mention would otherwise
// be silently erased by UpdateRecord's whole-record replace.
func carryForwardMissingFields(machine *domain.Machine, values, existing map[string]any) {
	for _, f := range machine.Fields {
		if _, present := values[f.ID]; present {
			continue
		}
		if v, ok := existing[f.ID]; ok {
			values[f.ID] = v
		}
	}
}

// stepInput is one approver row as the wizard submitted it, already paired up.
//
// One struct rather than four parallel []string arguments, because the pairing is the fragile
// part: row order *is* fld_sequence (there is no hidden sequence input -- see below), so four
// slices that drift out of alignment by one would silently attach every approver to the wrong
// step. parseStepInputs is the single place that alignment is established, and the reason the
// wizard's type picker is a <select> rather than the board's segmented buttons: an unchecked radio
// submits nothing at all, which is exactly how a slice loses an element.
type stepInput struct {
	name          string
	assignee      string
	approverType  string
	approverGroup string
}

// parseStepInputs reads the approver rows out of the submitted form and rejects a row that names
// no approver of the kind it claims.
//
// That pairing check lives here rather than in metadata because metadata cannot state it: what it
// wants to say is "fld_assignee is required when fld_approver_type is User", and this runtime's
// Constraint has one shape only -- block a field transition while a *related Machine* has a
// matching record -- which cannot condition on a sibling Field of the same record. So fld_assignee
// is declared optional (see metadata/approval_step.yaml's own comment) and this is the enforcement.
// Forward pointer: ROADMAP.md's "Conditional required" deferral row; when that lands, this check
// becomes two declarations and this function loses its reason to validate anything.
//
// An entirely blank row is skipped rather than rejected: the wizard renders one empty row to start
// with, and "+ Add approver" can leave a spare.
func parseStepInputs(req *http.Request) ([]stepInput, string) {
	names := req.Form[action.FieldStepName]
	assignees := req.Form[action.FieldStepAssignee]
	types := req.Form[action.FieldStepApproverType]
	groups := req.Form[action.FieldStepApproverGroup]

	rows := make([]stepInput, 0, len(assignees))
	for i := range assignees {
		row := stepInput{
			name:          at(names, i),
			assignee:      assignees[i],
			approverType:  at(types, i),
			approverGroup: at(groups, i),
		}
		switch {
		case row.approverType == domain.ActorKindGroup && row.approverGroup == "":
			return nil, "a step set to Group must name a group"
		case row.approverType != domain.ActorKindGroup && row.assignee == "" && row.approverGroup == "":
			row = stepInput{} // an untouched spare row
		case row.approverType != domain.ActorKindGroup && row.assignee == "":
			return nil, "a step set to User must name a person"
		}
		rows = append(rows, row)
	}
	return rows, ""
}

// at is a bounds-safe index into a parallel form slice. A row whose control was never rendered --
// an older cached page, a hand-built POST -- reads as empty rather than panicking the handler.
func at(values []string, i int) string {
	if i < len(values) {
		return values[i]
	}
	return ""
}

func hasApprover(rows []stepInput) bool {
	for _, r := range rows {
		if r.assignee != "" || r.approverGroup != "" {
			return true
		}
	}
	return false
}

// stepRowValues is one approver row's Field values, keyed by whichever "which Document/Template
// do I belong to" field name the caller passes -- shared by createApprovalSteps (fld_document) and
// saveApprovalFlowTemplate (fld_template, CAP-V28, ROADMAP.md 2026-09-27), the second real caller
// that is this split's own trigger (CLAUDE.md's decomposition-on-second-case rule).
//
// Sequence comes from the submitted order rather than a hidden input: a browser submits repeated
// field names in DOM order, which is exactly the order the wizard's own reordering leaves the
// <select>s in.
//
// fld_approver_type is written only when the row actually chose one. A row left on the default
// stores nothing there, which is deliberate: an empty type is what makes authorization's own
// fallback take over, so a wizard that stamped "User" on every row would opt every step into the
// dynamic gate for no reason and make the fallback path untested in practice.
func stepRowValues(parentField, parentID string, i int, row stepInput) map[string]any {
	values := map[string]any{
		parentField:              parentID,
		action.FieldStepSequence: float64(i + 1),
	}
	if row.name != "" {
		values[action.FieldStepName] = row.name
	}
	if row.approverType == domain.ActorKindGroup {
		values[action.FieldStepApproverType] = domain.ActorKindGroup
		values[action.FieldStepApproverGroup] = row.approverGroup
	} else {
		values[action.FieldStepAssignee] = row.assignee
	}
	return values
}

// createApprovalSteps writes one pending Approval Step per approver row.
//
// It is the position in the submitted list, so an empty slot leaves a gap -- an untouched row
// before a filled one makes the filled one step 2, not step 1. That is the existing behaviour and
// it is harmless, because action.CanDecide compares sequences relatively rather than expecting
// 1..n. Left as it was: Phase 19 moved this code, it did not quietly renumber approvals, and
// neither does Fase 6c-2.
func createApprovalSteps(w http.ResponseWriter, req *http.Request, store *data.Store, stepMachine *domain.Machine, documentID string, rows []stepInput) bool {
	for i, row := range rows {
		if row.assignee == "" && row.approverGroup == "" {
			continue
		}
		values := stepRowValues(action.FieldStepDocument, documentID, i, row)
		values[action.FieldStepDecision] = action.DecisionPending
		data.ApplyDefaults(stepMachine, values)
		if !validRecord(w, req, store, stepMachine, values) {
			return false
		}
		if _, err := store.CreateRecord(req.Context(), stepMachine.ID, values); err != nil {
			serverError(w, err)
			return false
		}
	}
	return true
}

// findApprovalFlowTemplate is CAP-V28's "find" half (ROADMAP.md, 2026-09-27): the saved default
// approval flow for documentType, if one exists, and its own ordered steps. Returns
// (nil, nil, nil) when none is saved yet -- not an error, the ordinary state for a Document Type
// nobody has saved a flow for.
//
// At most one template exists per Document Type by construction (saveApprovalFlowTemplate always
// finds-before-creating); this runtime has no uniqueness primitive to declare that at the metadata
// level, so a duplicate created by hand through the generic create route would simply have its
// first match (ListRecordsBy's own order) picked here and its rest ignored.
func findApprovalFlowTemplate(ctx context.Context, store *data.Store, documentType string) (template *data.Record, steps []*data.Record, err error) {
	// Uncast optional roles mean this Application saves no default flows at all, so there is nothing
	// to find -- and querying an empty Machine id would read nothing while looking like a real read.
	templateMachine, stepMachine := approvalMachine(ctx, domain.WorkflowRoleFlowTemplate), approvalMachine(ctx, domain.WorkflowRoleFlowTemplateStep)
	if templateMachine == nil || stepMachine == nil {
		return nil, nil, nil
	}
	templates, err := store.ListRecordsBy(ctx, templateMachine.ID, action.FieldTemplateDocumentType, documentType)
	if err != nil {
		return nil, nil, err
	}
	if len(templates) == 0 {
		return nil, nil, nil
	}
	template = templates[0]
	steps, err = store.ListRecordsBy(ctx, stepMachine.ID, action.FieldTemplateStepTemplate, template.ID)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(steps, func(i, j int) bool {
		return toFloat(steps[i].Values[action.FieldTemplateStepSequence]) < toFloat(steps[j].Values[action.FieldTemplateStepSequence])
	})
	return template, steps, nil
}

// toFloat reads a stored fld_sequence value (always a float64 off JSONB) for findApprovalFlowTemplate's
// own sort -- 0 for a record somehow missing it, sorting it first rather than panicking.
func toFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

// saveApprovalFlowTemplate is CAP-V28's "save" half: find-or-create the Document Type's own
// template, then replace its steps wholesale with rows -- never append, so resubmitting the same
// Document Type with a different chain replaces the old default rather than accumulating two.
//
// Called after the real Document and its real Approval Steps are already committed
// (submitDocumentWizard/continueDocumentWizard), and its own error is logged rather than failing
// the request: at that point the document submission itself already succeeded, and surfacing a
// template-save failure as "your submission failed" would be a worse lie than a quiet log line --
// the same posture the logActivity calls immediately beside both callers already take.
func saveApprovalFlowTemplate(ctx context.Context, store *data.Store, templateMachine, templateStepMachine *domain.Machine, documentType, mode string, rows []stepInput) error {
	existing, steps, err := findApprovalFlowTemplate(ctx, store, documentType)
	if err != nil {
		return err
	}
	templateValues := map[string]any{
		action.FieldTemplateDocumentType: documentType,
		action.FieldTemplateMode:         mode,
	}
	var templateID string
	if existing != nil {
		templateID = existing.ID
		if _, err := store.UpdateRecord(ctx, templateMachine.ID, templateID, templateValues); err != nil {
			return err
		}
		for _, step := range steps {
			if err := store.DeleteRecord(ctx, templateStepMachine.ID, step.ID); err != nil {
				return err
			}
		}
	} else {
		data.ApplyDefaults(templateMachine, templateValues)
		created, err := store.CreateRecord(ctx, templateMachine.ID, templateValues)
		if err != nil {
			return err
		}
		templateID = created.ID
	}
	for i, row := range rows {
		if row.assignee == "" && row.approverGroup == "" {
			continue
		}
		values := stepRowValues(action.FieldTemplateStepTemplate, templateID, i, row)
		data.ApplyDefaults(templateStepMachine, values)
		if _, err := store.CreateRecord(ctx, templateStepMachine.ID, values); err != nil {
			return err
		}
	}
	return nil
}

// showApprovalFlowTemplateRows serves the wizard's own "pick a Document Type" htmx fragment
// (CAP-V28's "find" half rendered): GET /documents/new/approval-flow-template?fld_document_type=.
// A saved template's steps become prefilled ApproverRows plus an out-of-band update to the mode
// fieldset; no saved template renders exactly a fresh wizard's own starting state (one blank row,
// the Field's first declared mode) -- switching *away* from a type with a saved flow resets rather
// than leaves stale rows from whatever was picked before.
func showApprovalFlowTemplateRows(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		opts, err := readWizardOptions(req, machines, store)
		if err != nil {
			serverError(w, err)
			return
		}
		documentType := req.URL.Query().Get(action.FieldTemplateDocumentType)
		template, steps, err := findApprovalFlowTemplate(req.Context(), store, documentType)
		if err != nil {
			serverError(w, err)
			return
		}
		prefills := make([]rendering.StepPrefill, 0, len(steps))
		for _, step := range steps {
			prefills = append(prefills, rendering.StepPrefill{
				Name:         toDisplayString(step.Values[action.FieldTemplateStepName]),
				ApproverType: toDisplayString(step.Values[action.FieldTemplateStepApproverType]),
				AssigneeID:   toDisplayString(step.Values[action.FieldTemplateStepAssignee]),
				GroupID:      toDisplayString(step.Values[action.FieldTemplateStepApproverGroup]),
			})
		}
		// "" is a deliberate valid value here, not an unhandled case: modeFieldset (documentsubmit.
		// templ) treats an empty selected the same way DocumentSubmitPage's own fresh-wizard render
		// already does -- the Field's first declared option checked, exactly the reset this
		// fragment's own doc comment above promises for a Document Type with no saved template.
		var selectedMode string
		if template != nil {
			selectedMode = toDisplayString(template.Values[action.FieldTemplateMode])
		}
		render(req.Context(), w, rendering.ApprovalFlowTemplateRows(opts.approvers, opts.groups, opts.mode, prefills, selectedMode))
	}
}

// showSignaturePlacement is Case 3's signature-coordinate placement screen (ROADMAP.md Phase 15
// Step 4) -- a real rendered page of the Document's own PDF (Step 3's internal/pdf), one
// draggable marker per Approval Step. Hardcoded to mch_document, same posture as decideStep: this
// is Case 3's own screen, not a generic per-Machine feature.
func showSignaturePlacement(store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsDocument(machine) {
			http.Error(w, "signature placement only applies to a Document", http.StatusNotFound)
			return
		}
		ctx := req.Context()
		documentID := chi.URLParam(req, "id")
		document, steps, relations, totalPages, err := loadSignaturePlacementData(ctx, store, files, machines, documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		page := pageFromQuery(req, totalPages)
		actor := currentActor(req, store, cfg)
		view, err := composition.SignaturePlacement(ctx, composition.NewLoader(store, machines),
			machineForStep(ctx), machine, document, steps, relations, page, totalPages, actor)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		_, switchHref := viewerWorkspaceContext(ctx, store, actor.ID)
		render(ctx, w, rendering.SignaturePlacementPage(view, chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}

// loadSignaturePlacementData centralizes what both the dedicated Signature Placement page and
// Document's own detail page need: the Document record, its PDF's total page count, its own
// Approval Steps, and their relation options -- the exact logic that used to live inline in
// showSignaturePlacement, extracted so documentSignaturePlacementView below doesn't duplicate it.
// It does not take a page number: totalPages is an output (the caller decides which page to show,
// or -- for the inline embed -- always shows page 1), not an input this loading step needs.
func loadSignaturePlacementData(ctx context.Context, store *data.Store, files *storage.Store, machines map[string]*domain.Machine, documentID string) (*data.Record, []*data.Record, rendering.RelationOptions, int, error) {
	document, fileData, err := loadDocumentPDF(ctx, store, files, documentID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	totalPages, err := pdf.PageCount(fileData)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	// This guard predates the roles and was written for the failure they remove: a Workspace holding
	// its own, unrelated Machine named mch_document reached here and panicked on a nil
	// *domain.Machine inside composition.Loader.RelationOptions (found 2026-09-27). Asking which
	// Machine this Application cast as its step answers that by construction -- an Application that
	// casts none has no placement screen -- and the guard stays because "no approval Application
	// here" is still a reachable state. documentSignaturePlacementView's own doc comment promises
	// "fails open, not closed" for this whole function, and that is what an error does.
	stepMachine := machineForStep(ctx)
	if stepMachine == nil {
		return nil, nil, nil, 0, fmt.Errorf("this workspace has no approval step machine")
	}
	steps, err := store.ListRecordsBy(ctx, stepMachine.ID, action.FieldStepDocument, documentID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	ld := composition.NewLoader(store, machines)
	relations, err := ld.RelationOptions(ctx, stepMachine)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	return document, steps, relations, totalPages, nil
}

// documentSignaturePlacementView is Fase 0 of the composable-runtime kajian: the same
// signaturePlacementBlock Component the dedicated Signature Placement page renders, now also
// available to Document's own generic detail view (record.go). Returns nil immediately for every
// Machine other than mch_document (zero cost -- no PDF read, no extra query). Fails open, not
// closed: Document's detail page has never depended on its PDF being readable, so a loading error
// here is logged and treated as "nothing to show inline", not a reason to 500 the whole page --
// the same "best-effort, logged, never blocks the primary flow" posture already established for
// PDF signature compositing (capabilities.md).
func documentSignaturePlacementView(ctx context.Context, store *data.Store, files *storage.Store, machines map[string]*domain.Machine, machine *domain.Machine, recordID string, actor domain.Actor) *rendering.DocumentSignaturePlacement {
	if !action.IsDocument(machine) {
		return nil
	}
	document, steps, relations, totalPages, err := loadSignaturePlacementData(ctx, store, files, machines, recordID)
	if err != nil {
		log.Printf("signature placement inline view for document %s: %v", recordID, err)
		return nil
	}
	// Page 1, because this embed has no pager of its own -- a step placed on page 6 has its card
	// rendered here but not its marker. Named rather than fixed: the dedicated screen is where
	// placement happens, and giving the embed a pager would duplicate that screen inside a page
	// that is already the generic record view.
	view, err := composition.SignaturePlacement(ctx, composition.NewLoader(store, machines),
		machineForStep(ctx), machine, document, steps, relations, 1, totalPages, actor)
	if err != nil {
		log.Printf("signature placement inline view for document %s: %v", recordID, err)
		return nil
	}
	return &rendering.DocumentSignaturePlacement{View: view}
}

// servePDFPreview rasterizes one page of a Document's own PDF to PNG (Step 3's internal/pdf),
// for showSignaturePlacement's own <img> -- not exposed for any other Machine or file field, same
// hardcoded scope as the rest of Case 3's Action/screen code.
func servePDFPreview(store *data.Store, files *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if !action.IsDocument(machine) {
			http.Error(w, "PDF preview only applies to a Document", http.StatusNotFound)
			return
		}
		_, fileData, err := loadDocumentPDF(req.Context(), store, files, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		totalPages, err := pdf.PageCount(fileData)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to read document PDF: %v", err), http.StatusUnprocessableEntity)
			return
		}
		page := pageFromQuery(req, totalPages)

		png, err := pdf.RenderPagePNG(fileData, page-1, 900, 1200)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to render page: %v", err), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(png); err != nil {
			log.Printf("write PNG preview failed: %v", err)
		}
	}
}

// loadDocumentPDF fetches document's own fld_file upload and reads it off local disk, for both
// showSignaturePlacement and servePDFPreview -- the one place either handler needs the raw PDF
// bytes.
func loadDocumentPDF(ctx context.Context, store *data.Store, files *storage.Store, documentID string) (*data.Record, []byte, error) {
	document, err := store.GetRecord(ctx, approvalMachineID(ctx, domain.WorkflowRoleDocument), documentID)
	if err != nil {
		return nil, nil, err
	}
	key := toDisplayString(document.Values[action.FieldDocumentFile])
	if key == "" {
		return document, nil, fmt.Errorf("document has no file uploaded")
	}
	path, err := files.Path(key)
	if err != nil {
		return document, nil, err
	}
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return document, nil, err
	}
	return document, fileBytes, nil
}

// serveUpload streams a previously uploaded file back. Gated by requireAuth like every other
// route in its group, plus an ownership check: the storage key names the Machine/Field it belongs
// to (recordOwnsUpload below), so serveUpload confirms some record in the caller's own Workspace
// actually carries this exact key in that field before serving it -- closing security audit
// 2026-09-19's M1 (an authenticated member of any Workspace could previously read any other
// Workspace's uploaded file just by knowing or guessing its key). A miss 404s rather than 403, so
// a guessed key can't be used to distinguish "wrong workspace" from "never existed".
//
// Content-Type and Content-Disposition are decided from the file's own sniffed bytes, never from
// the stored key's extension (security audit 2026-09-19, H2) -- an attacker who got a disguised
// payload past handleFileUploads' own validation (record.go), or whose file was stored before that
// validation existed, still can't get the browser to render it inline as something other than what
// it actually is. Content-Type is set explicitly before http.ServeFile so it takes precedence over
// ServeFile's own extension-based guess.
func serveUpload(store *data.Store, files *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key := chi.URLParam(req, "*")
		owns, err := recordOwnsUpload(req.Context(), store, key)
		if err != nil {
			serverError(w, err)
			return
		}
		if !owns {
			http.NotFound(w, req)
			return
		}
		path, err := files.Path(key)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		contentType, disposition, err := sniffUploadForServing(path)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, disposition, storage.DisplayName(key)))
		http.ServeFile(w, req, path)
	}
}

// recordOwnsUpload reports whether some record of the Machine/Field the key itself names
// (storage.Store.Save's own key shape: "<machineID>/<fieldID>/<random>__<filename>") carries this
// exact key as that field's value, in the caller's own Workspace -- store.ListRecordsBy is already
// workspace-scoped from context the same way every other Store read is, so this doesn't special-
// case any one Machine (mch_document, mch_task, or any future Field Type: file Machine alike).
func recordOwnsUpload(ctx context.Context, store *data.Store, key string) (bool, error) {
	// filepath.Separator (a rune constant), not a "/" string literal -- matches storage.Save's own
	// filepath.Join for this same key, and keeps this line out of
	// TestHandlersHaveNoHardcodedApplicationRoute's string-literal scan, which doesn't distinguish
	// a path separator from a route by argument position, only by literal value.
	parts := strings.SplitN(key, string(filepath.Separator), 3)
	if len(parts) != 3 {
		return false, nil
	}
	machineID, fieldID := parts[0], parts[1]
	records, err := store.ListRecordsBy(ctx, machineID, fieldID, key)
	if err != nil {
		return false, err
	}
	return len(records) > 0, nil
}

// inlineSafeUploadContentTypes is the small allowlist of content this app deliberately opens
// in-browser (PDF/image previews and attachments) rather than downloading -- everything else is
// forced to `attachment`, regardless of what extension the original upload carried. Narrower than,
// and independent of, handleFileUploads' own denylist (record.go): validation happens once at
// upload time, disposition is decided fresh on every serve, which also covers any file stored
// before that upload-time validation existed.
var inlineSafeUploadContentTypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
}

// sniffUploadForServing reads path's own first bytes (http.DetectContentType, the same mechanism
// a browser's MIME-sniffing would use) to decide what Content-Type to declare and whether it's
// safe to show inline.
func sniffUploadForServing(path string) (contentType, disposition string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", "", err
	}
	contentType = http.DetectContentType(buf[:n])

	base := contentType
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		base = strings.TrimSpace(contentType[:idx])
	}
	disposition = "attachment"
	if inlineSafeUploadContentTypes[base] {
		disposition = "inline"
	}
	return contentType, disposition, nil
}
